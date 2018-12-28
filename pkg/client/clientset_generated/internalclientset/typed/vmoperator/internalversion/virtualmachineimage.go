/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package internalversion

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	vmoperator "vmware.com/kubevsphere/pkg/apis/vmoperator"
	scheme "vmware.com/kubevsphere/pkg/client/clientset_generated/internalclientset/scheme"
)

// VirtualMachineImagesGetter has a method to return a VirtualMachineImageInterface.
// A group's client should implement this interface.
type VirtualMachineImagesGetter interface {
	VirtualMachineImages(namespace string) VirtualMachineImageInterface
}

// VirtualMachineImageInterface has methods to work with VirtualMachineImage resources.
type VirtualMachineImageInterface interface {
	Create(*vmoperator.VirtualMachineImage) (*vmoperator.VirtualMachineImage, error)
	Update(*vmoperator.VirtualMachineImage) (*vmoperator.VirtualMachineImage, error)
	UpdateStatus(*vmoperator.VirtualMachineImage) (*vmoperator.VirtualMachineImage, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*vmoperator.VirtualMachineImage, error)
	List(opts v1.ListOptions) (*vmoperator.VirtualMachineImageList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *vmoperator.VirtualMachineImage, err error)
	VirtualMachineImageExpansion
}

// virtualMachineImages implements VirtualMachineImageInterface
type virtualMachineImages struct {
	client rest.Interface
	ns     string
}

// newVirtualMachineImages returns a VirtualMachineImages
func newVirtualMachineImages(c *VmoperatorClient, namespace string) *virtualMachineImages {
	return &virtualMachineImages{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the virtualMachineImage, and returns the corresponding virtualMachineImage object, and an error if there is any.
func (c *virtualMachineImages) Get(name string, options v1.GetOptions) (result *vmoperator.VirtualMachineImage, err error) {
	result = &vmoperator.VirtualMachineImage{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of VirtualMachineImages that match those selectors.
func (c *virtualMachineImages) List(opts v1.ListOptions) (result *vmoperator.VirtualMachineImageList, err error) {
	result = &vmoperator.VirtualMachineImageList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested virtualMachineImages.
func (c *virtualMachineImages) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a virtualMachineImage and creates it.  Returns the server's representation of the virtualMachineImage, and an error, if there is any.
func (c *virtualMachineImages) Create(virtualMachineImage *vmoperator.VirtualMachineImage) (result *vmoperator.VirtualMachineImage, err error) {
	result = &vmoperator.VirtualMachineImage{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		Body(virtualMachineImage).
		Do().
		Into(result)
	return
}

// Update takes the representation of a virtualMachineImage and updates it. Returns the server's representation of the virtualMachineImage, and an error, if there is any.
func (c *virtualMachineImages) Update(virtualMachineImage *vmoperator.VirtualMachineImage) (result *vmoperator.VirtualMachineImage, err error) {
	result = &vmoperator.VirtualMachineImage{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		Name(virtualMachineImage.Name).
		Body(virtualMachineImage).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *virtualMachineImages) UpdateStatus(virtualMachineImage *vmoperator.VirtualMachineImage) (result *vmoperator.VirtualMachineImage, err error) {
	result = &vmoperator.VirtualMachineImage{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		Name(virtualMachineImage.Name).
		SubResource("status").
		Body(virtualMachineImage).
		Do().
		Into(result)
	return
}

// Delete takes name of the virtualMachineImage and deletes it. Returns an error if one occurs.
func (c *virtualMachineImages) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *virtualMachineImages) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("virtualmachineimages").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched virtualMachineImage.
func (c *virtualMachineImages) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *vmoperator.VirtualMachineImage, err error) {
	result = &vmoperator.VirtualMachineImage{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("virtualmachineimages").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
