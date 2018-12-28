/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package internalclientset

import (
	glog "github.com/golang/glog"
	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
	vmoperatorinternalversion "vmware.com/kubevsphere/pkg/client/clientset_generated/internalclientset/typed/vmoperator/internalversion"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	Vmoperator() vmoperatorinternalversion.VmoperatorInterface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	vmoperator *vmoperatorinternalversion.VmoperatorClient
}

// Vmoperator retrieves the VmoperatorClient
func (c *Clientset) Vmoperator() vmoperatorinternalversion.VmoperatorInterface {
	return c.vmoperator
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.vmoperator, err = vmoperatorinternalversion.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(&configShallowCopy)
	if err != nil {
		glog.Errorf("failed to create the DiscoveryClient: %v", err)
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.vmoperator = vmoperatorinternalversion.NewForConfigOrDie(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClientForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.vmoperator = vmoperatorinternalversion.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
