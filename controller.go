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
	"context"
	"fmt"
	"time"

	"github.com/alessio/shellescape"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

	jobsLister   batchlisters.JobLister
	jobsSynced   cache.InformerSynced
	imagesLister listers.ImageLister
	imagesSynced cache.InformerSynced

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
	imageInformer informers.ImageInformer) *Controller {

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
		kubeclientset:  kubeclientset,
		imageclientset: client,
		jobsLister:     jobInformer.Lister(),
		jobsSynced:     jobInformer.Informer().HasSynced,
		imagesLister:   imageInformer.Lister(),
		imagesSynced:   imageInformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Images"),
		recorder:       recorder,
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
	if ok := cache.WaitForCacheSync(stopCh, c.jobsSynced, c.imagesSynced); !ok {
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

	var job *batchv1.Job
	for _, j := range jobs {
		// why are there so many nil jobs?
		if j == nil {
			continue
		}
		if metav1.IsControlledBy(j, image) {
			klog.Info("Found job for image:", j)
			job = j
			break
		}
	}
	if job == nil {
		job, err = c.kubeclientset.BatchV1().Jobs(image.Namespace).Create(context.TODO(), newBuildJob(image), metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}

	// Finally, we update the status block of the Image resource to reflect the
	// current state of the world
	err = c.updateImageStatus(image)
	if err != nil {
		return err
	}

	c.recorder.Event(image, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateImageStatus(image *imagev1alpha1.Image) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance

	imageCopy := image.DeepCopy()
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

// newBuildJob creates a new Job for a Image resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Image resource that 'owns' it.
func newBuildJob(image *imagev1alpha1.Image) *batchv1.Job {
	uid := int64(1000)
	// FIXME FIXME: Triple check this is safe
	script := fmt.Sprintf(`
mkdir /tmp/context
echo %s > /tmp/context/Containerfile
IMAGE="%s/%s:%s"
podman build --isolation chroot -t "$IMAGE" /tmp/context
podman push "$IMAGE"`, shellescape.Quote(image.Spec.Containerfile), image.Spec.Registry, image.Spec.Repository, image.Spec.Tag)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: image.Name,
			Namespace:    image.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(image, imagev1alpha1.SchemeGroupVersion.WithKind("Image")),
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"container.apparmor.security.beta.kubernetes.io/builder": "unconfined",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: "OnFailure",
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "filer",
					},
					Containers: []corev1.Container{
						{
							Name:       "builder",
							Image:      "quay.io/podman/stable",
							WorkingDir: "/usr/src",
							Args:       []string{"/bin/bash", "-xeuo", "pipefail", "-c", script},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &uid,
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"github.com/fuse": resource.MustParse("1"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "image-push-secret",
									MountPath: "/home/podman/.docker",
								}, {
									Name:      "podman-local",
									MountPath: "/home/podman/.local/share/containers",
								}, {
									Name:      "tls-certs",
									MountPath: "/etc/ssl/certs",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "image-push-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "image-pull-secret",
									Items: []corev1.KeyToPath{
										{
											Key:  ".dockerconfigjson",
											Path: "config.json",
										},
									},
								},
							},
						},
						{
							Name: "podman-local",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/tmp/podman-containers",
								},
							},
						}, {
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/ssl/certs",
								},
							},
						},
					},
				},
			},
		},
	}
}
