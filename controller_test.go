/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	imagecontroller "github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	imagev1alpha1 "github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	"github.com/discordianfish/k8s-image-controller/pkg/generated/clientset/versioned/fake"
	informers "github.com/discordianfish/k8s-image-controller/pkg/generated/informers/externalversions"
	"github.com/google/go-cmp/cmp"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	imageLister []*imagecontroller.Image
	jobsLister  []*batchv1.Job
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func genUID() types.UID {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 32)
	rand.Read(b)
	return types.UID(fmt.Sprintf("%x", b)[:32])
}

func newImage(name string, containerfile string) *imagecontroller.Image {
	return &imagecontroller.Image{
		TypeMeta: metav1.TypeMeta{APIVersion: imagecontroller.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			UID:       genUID(),
		},
		Spec: imagecontroller.ImageSpec{
			Registry:      "example.com",
			Repository:    "some-image",
			Tag:           "latest",
			Containerfile: containerfile,
		},
	}
}

func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := NewController(f.kubeclient, f.client,
		k8sI.Batch().V1().Jobs(),
		i.Imagecontroller().V1alpha1().Images(),
		k8sI.Apps().V1().Deployments(),
	)

	c.imagesSynced = alwaysReady
	c.deploymentsSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	for _, f := range f.imageLister {
		i.Imagecontroller().V1alpha1().Images().Informer().GetIndexer().Add(f)
	}

	for _, d := range f.jobsLister {
		k8sI.Batch().V1().Jobs().Informer().GetIndexer().Add(d)
	}

	return c, i, k8sI
}

func (f *fixture) run(imageName string) {
	f.runController(imageName, true, false)
}

func (f *fixture) runExpectError(imageName string) {
	f.runController(imageName, true, true)
}

func (f *fixture) runController(imageName string, startInformers bool, expectError bool) {
	c, i, k8sI := f.newController()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		i.Start(stopCh)
		k8sI.Start(stopCh)
	}

	err := c.syncHandler(imageName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing image: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing image, got nil")
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "images") ||
				action.Matches("watch", "images") ||
				action.Matches("list", "deployments") ||
				action.Matches("watch", "deployments")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateImageStatusAction(image *imagecontroller.Image) {
	action := core.NewUpdateSubresourceAction(schema.GroupVersionResource{Resource: "images"}, "status", image.Namespace, image)
	f.actions = append(f.actions, action)
}

func getKey(image *imagecontroller.Image, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(image)
	if err != nil {
		t.Errorf("Unexpected error getting key for image %v: %v", image.Name, err)
		return ""
	}
	return key
}

/*
func TestCreatesDeployment(t *testing.T) {
	f := newFixture(t)
	image := newImage("test", int32Ptr(1))

	f.imageLister = append(f.imageLister, image)
	f.objects = append(f.objects, image)

	expDeployment := newDeployment(image)
	f.expectCreateDeploymentAction(expDeployment)
	f.expectUpdateImageStatusAction(image)

	f.run(getKey(image, t))
}

func TestDoNothing(t *testing.T) {
	f := newFixture(t)
	image := newImage("test", int32Ptr(1))
	d := newDeployment(image)

	f.imageLister = append(f.imageLister, image)
	f.objects = append(f.objects, image)
	f.jobsLister = append(f.jobsLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateImageStatusAction(image)
	f.run(getKey(image, t))
}

func TestUpdateDeployment(t *testing.T) {
	f := newFixture(t)
	image := newImage("test", int32Ptr(1))
	d := newDeployment(image)

	// Update replicas
	image.Spec.Replicas = int32Ptr(2)
	expDeployment := newDeployment(image)

	f.imageLister = append(f.imageLister, image)
	f.objects = append(f.objects, image)
	f.jobsLister = append(f.jobsLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateImageStatusAction(image)
	f.expectUpdateDeploymentAction(expDeployment)
	f.run(getKey(image, t))
}

func TestNotControlledByUs(t *testing.T) {
	f := newFixture(t)
	image := newImage("test", int32Ptr(1))
	d := newDeployment(image)

	d.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}

	f.imageLister = append(f.imageLister, image)
	f.objects = append(f.objects, image)
	f.jobsLister = append(f.jobsLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.runExpectError(getKey(image, t))
}
*/

func TestFindJob(t *testing.T) {
	var (
		someImage  = newImage("test", "FROM busybox")
		otherImage = newImage("test", "FROM debian")
		someJob    = newBuildJob(someImage)
		otherJob   = newBuildJob(otherImage)
	)
	for _, tc := range []struct {
		name     string
		jobs     []*batchv1.Job
		job      *batchv1.Job
		image    *imagev1alpha1.Image
		expected *batchv1.Job
	}{
		{
			"job is found",
			[]*batchv1.Job{otherJob, someJob, otherJob},
			someJob,
			someImage,
			someJob,
		},
		{
			"job is not found",
			[]*batchv1.Job{otherJob, otherJob, otherJob},
			someJob,
			someImage,
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := findJob(tc.jobs, newBuildJob(tc.image), tc.image)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Fatal("(-want +got)\n", diff)
			}
		})
	}
}
