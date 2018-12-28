/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package fake

import (
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
	internalversion "vmware.com/kubevsphere/pkg/client/clientset_generated/internalclientset/typed/vmoperator/internalversion"
)

type FakeVmoperator struct {
	*testing.Fake
}

func (c *FakeVmoperator) VirtualMachines(namespace string) internalversion.VirtualMachineInterface {
	return &FakeVirtualMachines{c, namespace}
}

func (c *FakeVmoperator) VirtualMachineImages(namespace string) internalversion.VirtualMachineImageInterface {
	return &FakeVirtualMachineImages{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeVmoperator) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
