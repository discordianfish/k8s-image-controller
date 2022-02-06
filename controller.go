/*
Copyright 2017 The Kubernetes Authors.
Copyright 2022 Johannes 'fish' Ziemke.

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
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/alessio/shellescape"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	batchinformers "k8s.io/client-go/informers/batch/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	imagev1alpha1 "github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	clientset "github.com/discordianfish/k8s-image-controller/pkg/generated/clientset/versioned"
	imagescheme "github.com/discordianfish/k8s-image-controller/pkg/generated/clientset/versioned/scheme"
	informers "github.com/discordianfish/k8s-image-controller/pkg/generated/informers/externalversions/imagecontroller/v1alpha1"
	listers "github.com/discordianfish/k8s-image-controller/pkg/generated/listers/imagecontroller/v1alpha1"
)

const controllerAgentName = "image-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Image is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Image fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Image"
	// MessageResourceSynced is the message used for an Event fired when a Image
	// is synced successfully
	MessageResourceSynced = "Image synced successfully"
)

// Controller is the controller implementation for Image resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// imageclientset is a clientset for our own API group
	imageclientset clientset.Interface

	jobsLister          batchlisters.JobLister
	jobsSynced          cache.InformerSynced
	imagesLister        listers.ImageLister
	imagesSynced        cache.InformerSynced
	imageBuildersLister listers.ImageBuilderLister
	imageBuildersSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	client clientset.Interface,
	jobInformer batchinformers.JobInformer,
	imageInformer informers.ImageInformer,
	imageBuilderInformer informers.ImageBuilderInformer) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(imagescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:       kubeclientset,
		imageclientset:      client,
		jobsLister:          jobInformer.Lister(),
		jobsSynced:          jobInformer.Informer().HasSynced,
		imagesLister:        imageInformer.Lister(),
		imagesSynced:        imageInformer.Informer().HasSynced,
		imageBuildersLister: imageBuilderInformer.Lister(),
		imageBuildersSynced: imageBuilderInformer.Informer().HasSynced,

		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Images"),
		recorder:  recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Image resources change
	imageInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueImage,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueImage(new)
		},
	})
	// Set up an event handler for when Job resources change. This
	// handler will lookup the owner of the given Job, and if it is
	// owned by a Image resource then the handler will enqueue that Image resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Job resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	jobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newJob := new.(*batchv1.Job)
			oldJob := old.(*batchv1.Job)
			if newJob.ResourceVersion == oldJob.ResourceVersion {
				// Periodic resync will send update events for all known Jobs.
				// Two different versions of the same Jobs will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Image controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.jobsSynced, c.imagesSynced, c.imageBuildersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Image resources
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Image resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Image resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Image resource with this namespace/name
	image, err := c.imagesLister.Images(namespace).Get(name)
	if err != nil {
		// The Image resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("image '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	// FIXME: Find a cheaper way to do this
	jobs, err := c.jobsLister.Jobs(image.Namespace).List(labels.Everything())

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}
	now := metav1.NewTime(time.Now())
	condition := imagev1alpha1.ImageCondition{
		Type:               "Unknown",
		Status:             v1.ConditionTrue,
		LastTransitionTime: &now,
	}
	newJob, err := c.newBuildJob(image)
	if err != nil {
		return err
	}
	job := findJob(jobs, newJob, image)
	if job == nil {
		klog.Info("Creating new job: %v", newJob)
		job, err = c.kubeclientset.BatchV1().Jobs(image.Namespace).Create(context.TODO(), newJob, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		condition.Type = "Pending"
	}

	for _, c := range job.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Complete":
			condition.Type = "Ready"
			condition.Reason = "Ready"
			condition.Message = "Build job finished successfully"
		case "Failed":
			condition.Type = "Failed"
			condition.Reason = "Failed"
			condition.Message = "Build job failed"
		}
	}

	// Finally, we update the status block of the Image resource to reflect the
	// current state of the world
	err = c.updateImageStatus(image, condition)
	if err != nil {
		return err
	}

	c.recorder.Event(image, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func splitNamespace(fqname string) (namespace, name string) {
	parts := strings.SplitN(fqname, "/", 2)
	if len(parts) != 2 {
		return "", fqname
	}
	return parts[0], parts[1]
}

func jobsEqual(a *batchv1.Job, b *batchv1.Job) bool {
	var (
		as = a.Spec.Template.Spec
		bs = b.Spec.Template.Spec
	)

	return reflect.DeepEqual(as.Containers[0].Env, bs.Containers[0].Env)
}

// findJob returns the first job in jobs that run the same commands and are also managed by the imagecontroller.
// This job could have been created in response to any image
func findJob(jobs []*batchv1.Job, job *batchv1.Job, image *imagev1alpha1.Image) *batchv1.Job {
	for _, j := range jobs {
		// why are there so many nil jobs?
		if j == nil {
			continue
		}
		if !metav1.IsControlledBy(j, image) {
			continue
		}
		if jobsEqual(j, job) {
			return j
		}
	}
	return nil
}
func contains(a []string, s string) bool {
	for _, e := range a {
		if e == s {
			return true
		}
	}
	return false
}

// FIXME: This doesn't seem to work properly. Pending never happens
func updateConditions(image *imagev1alpha1.Image, condition imagev1alpha1.ImageCondition) {
	for i, cond := range image.Status.Conditions {
		if cond.Type != condition.Type {
			continue
		}

		if cond.Status == condition.Status {
			return // No change
		}

		image.Status.Conditions[i] = condition
		return
	}
	image.Status.Conditions = append(image.Status.Conditions, condition)
}
func (c *Controller) updateImageStatus(image *imagev1alpha1.Image, condition imagev1alpha1.ImageCondition) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance

	imageCopy := image.DeepCopy()
	updateConditions(image, condition)
	klog.Info("setting following conditions", image.Status.Conditions)
	//imageCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Image resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.imageclientset.ImagecontrollerV1alpha1().Images(image.Namespace).UpdateStatus(context.TODO(), imageCopy, metav1.UpdateOptions{})
	return err
}

// enqueueImage takes a Image resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Image.
func (c *Controller) enqueueImage(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Image resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Image resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Image, we should not do anything more
		// with it.
		if ownerRef.Kind != "Image" {
			return
		}

		image, err := c.imagesLister.Images(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned object '%s' of image '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueImage(image)
		return
	}
}

func nameFromImage(image *imagev1alpha1.Image) string {
	return fmt.Sprintf("%s/%s:%s", image.Spec.Registry, image.Spec.Repository, image.Spec.Tag)
}

func formatBuildArgs(image *imagev1alpha1.Image) string {
	args := make([]string, len(image.Spec.BuildArgs)*2)
	for i, arg := range image.Spec.BuildArgs {
		args[i] = shellescape.Quote(arg.Name + "=" + arg.Value)
	}
	return strings.Join(args, " ")
}

// newBuildJob creates a new Job for a Image resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Image resource that 'owns' it.
func (c *Controller) newBuildJob(image *imagev1alpha1.Image) (*batchv1.Job, error) {
	builders, err := c.imageBuildersLister.List(labels.Everything())
	//builder, err := c.imageBuildersLister.ImageBuilders("").Get(image.Spec.BuilderName)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get ImageBuilder: %w", err)
	}
	builder := &v1alpha1.ImageBuilder{}
	for _, b := range builders {
		if b.ObjectMeta.Name == image.Spec.BuilderName {
			builder = b
			break
		}
	}

	staticFields := 2 // 3 Fields we add statically
	envs := make([]corev1.EnvVar, len(image.Spec.BuildArgs)+staticFields)
	envs = []corev1.EnvVar{
		corev1.EnvVar{Name: "CONTAINERFILE", Value: image.Spec.Containerfile},
		corev1.EnvVar{Name: "IMAGE", Value: nameFromImage(image)},
	}
	for i, arg := range image.Spec.BuildArgs {
		envs[staticFields+i] = corev1.EnvVar{Name: "BUILD_ARG_" + arg.Name, Value: arg.Value}
	}

	template := builder.Template.DeepCopy()
	template.Spec.Containers[0].Env = append(
		template.Spec.Containers[0].Env,
		envs...,
	)
	template.Spec.Containers[0].Env = envs
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: image.Name,
			Namespace:    image.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(image, imagev1alpha1.SchemeGroupVersion.WithKind("Image")),
			},
		},
		Spec: batchv1.JobSpec{
			Template: *template,
		},
	}, nil
}
