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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/discordianfish/k8s-image-controller/pkg/apis/imagecontroller/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeImageBuilders implements ImageBuilderInterface
type FakeImageBuilders struct {
	Fake *FakeImagecontrollerV1alpha1
	ns   string
}

var imagebuildersResource = schema.GroupVersionResource{Group: "imagecontroller.5pi.de", Version: "v1alpha1", Resource: "imagebuilders"}

var imagebuildersKind = schema.GroupVersionKind{Group: "imagecontroller.5pi.de", Version: "v1alpha1", Kind: "ImageBuilder"}

// Get takes name of the imageBuilder, and returns the corresponding imageBuilder object, and an error if there is any.
func (c *FakeImageBuilders) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ImageBuilder, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(imagebuildersResource, c.ns, name), &v1alpha1.ImageBuilder{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ImageBuilder), err
}

// List takes label and field selectors, and returns the list of ImageBuilders that match those selectors.
func (c *FakeImageBuilders) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ImageBuilderList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(imagebuildersResource, imagebuildersKind, c.ns, opts), &v1alpha1.ImageBuilderList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ImageBuilderList{ListMeta: obj.(*v1alpha1.ImageBuilderList).ListMeta}
	for _, item := range obj.(*v1alpha1.ImageBuilderList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested imageBuilders.
func (c *FakeImageBuilders) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(imagebuildersResource, c.ns, opts))

}

// Create takes the representation of a imageBuilder and creates it.  Returns the server's representation of the imageBuilder, and an error, if there is any.
func (c *FakeImageBuilders) Create(ctx context.Context, imageBuilder *v1alpha1.ImageBuilder, opts v1.CreateOptions) (result *v1alpha1.ImageBuilder, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(imagebuildersResource, c.ns, imageBuilder), &v1alpha1.ImageBuilder{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ImageBuilder), err
}

// Update takes the representation of a imageBuilder and updates it. Returns the server's representation of the imageBuilder, and an error, if there is any.
func (c *FakeImageBuilders) Update(ctx context.Context, imageBuilder *v1alpha1.ImageBuilder, opts v1.UpdateOptions) (result *v1alpha1.ImageBuilder, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(imagebuildersResource, c.ns, imageBuilder), &v1alpha1.ImageBuilder{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ImageBuilder), err
}

// Delete takes name of the imageBuilder and deletes it. Returns an error if one occurs.
func (c *FakeImageBuilders) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(imagebuildersResource, c.ns, name, opts), &v1alpha1.ImageBuilder{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeImageBuilders) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(imagebuildersResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ImageBuilderList{})
	return err
}

// Patch applies the patch and returns the patched imageBuilder.
func (c *FakeImageBuilders) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ImageBuilder, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(imagebuildersResource, c.ns, name, pt, data, subresources...), &v1alpha1.ImageBuilder{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ImageBuilder), err
}