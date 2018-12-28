/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package internalversion

import (
	rest "k8s.io/client-go/rest"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/internalclientset/scheme"
)

type VmoperatorInterface interface {
	RESTClient() rest.Interface
	VirtualMachinesGetter
	VirtualMachineImagesGetter
}

// VmoperatorClient is used to interact with features provided by the vmoperator.vmware.com group.
type VmoperatorClient struct {
	restClient rest.Interface
}

func (c *VmoperatorClient) VirtualMachines(namespace string) VirtualMachineInterface {
	return newVirtualMachines(c, namespace)
}

func (c *VmoperatorClient) VirtualMachineImages(namespace string) VirtualMachineImageInterface {
	return newVirtualMachineImages(c, namespace)
}

// NewForConfig creates a new VmoperatorClient for the given config.
func NewForConfig(c *rest.Config) (*VmoperatorClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &VmoperatorClient{client}, nil
}

// NewForConfigOrDie creates a new VmoperatorClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *VmoperatorClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new VmoperatorClient for the given RESTClient.
func New(c rest.Interface) *VmoperatorClient {
	return &VmoperatorClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	g, err := scheme.Registry.Group("vmoperator.vmware.com")
	if err != nil {
		return err
	}

	config.APIPath = "/apis"
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	if config.GroupVersion == nil || config.GroupVersion.Group != g.GroupVersion.Group {
		gv := g.GroupVersion
		config.GroupVersion = &gv
	}
	config.NegotiatedSerializer = scheme.Codecs

	if config.QPS == 0 {
		config.QPS = 5
	}
	if config.Burst == 0 {
		config.Burst = 10
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *VmoperatorClient) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
