/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package v1beta1

import (
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
	v1beta1 "vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/scheme"
)

type VmoperatorV1beta1Interface interface {
	RESTClient() rest.Interface
	VirtualMachinesGetter
	VirtualMachineImagesGetter
}

// VmoperatorV1beta1Client is used to interact with features provided by the vmoperator.vmware.com group.
type VmoperatorV1beta1Client struct {
	restClient rest.Interface
}

func (c *VmoperatorV1beta1Client) VirtualMachines(namespace string) VirtualMachineInterface {
	return newVirtualMachines(c, namespace)
}

func (c *VmoperatorV1beta1Client) VirtualMachineImages(namespace string) VirtualMachineImageInterface {
	return newVirtualMachineImages(c, namespace)
}

// NewForConfig creates a new VmoperatorV1beta1Client for the given config.
func NewForConfig(c *rest.Config) (*VmoperatorV1beta1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &VmoperatorV1beta1Client{client}, nil
}

// NewForConfigOrDie creates a new VmoperatorV1beta1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *VmoperatorV1beta1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new VmoperatorV1beta1Client for the given RESTClient.
func New(c rest.Interface) *VmoperatorV1beta1Client {
	return &VmoperatorV1beta1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1beta1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *VmoperatorV1beta1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
