/*
Copyright The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	imagecontrollerv1alpha1 "github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	versioned "github.com/discordianfish/k8s-image-controller/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/discordianfish/k8s-image-controller/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/discordianfish/k8s-image-controller/pkg/generated/listers/imagecontroller/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ImageBuilderInformer provides access to a shared informer and lister for
// ImageBuilders.
type ImageBuilderInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ImageBuilderLister
}

type imageBuilderInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewImageBuilderInformer constructs a new informer for ImageBuilder type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewImageBuilderInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredImageBuilderInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredImageBuilderInformer constructs a new informer for ImageBuilder type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredImageBuilderInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ImagecontrollerV1alpha1().ImageBuilders(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ImagecontrollerV1alpha1().ImageBuilders(namespace).Watch(context.TODO(), options)
			},
		},
		&imagecontrollerv1alpha1.ImageBuilder{},
		resyncPeriod,
		indexers,
	)
}

func (f *imageBuilderInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredImageBuilderInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *imageBuilderInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&imagecontrollerv1alpha1.ImageBuilder{}, f.defaultInformer)
}

func (f *imageBuilderInformer) Lister() v1alpha1.ImageBuilderLister {
	return v1alpha1.NewImageBuilderLister(f.Informer().GetIndexer())
}
