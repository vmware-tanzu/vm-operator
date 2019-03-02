/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice_test

import (
	"github.com/golang/glog"
	"testing"
	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/openapi"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"
	"vmware.com/kubevsphere/test/integration"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	vmrest "vmware.com/kubevsphere/pkg/apis/vmoperator/rest"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	"vmware.com/kubevsphere/pkg/controller/virtualmachineservice"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var shutdown chan struct{}
var controller *virtualmachineservice.VirtualMachineServiceController
var si *sharedinformers.SharedInformers
var vcsim *integration.VcSimInstance
var execute = false

func TestVirtualMachineService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineService Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	// TODO: Just return for now
	if !execute {
		return
	}
	vcsim = integration.NewVcSimInstance()

	address, port := vcsim.Start()

	vsphere.InitProviderWithConfig(integration.NewIntegrationVmOperatorConfig(address, port))

	vmprovider, err := vmprovider.NewVmProvider()
	if err != nil {
		glog.Fatalf("Failed to acquire vm provider: %s", err)
	}

	if err := v1alpha1.RegisterRestProvider(vmrest.NewVirtualMachineImagesREST(vmprovider)); err != nil {
		glog.Fatalf("Failed to register REST provider: %s", err)
	}

	testenv = test.NewTestEnvironment()
	config = testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs = clientset.NewForConfigOrDie(config)

	shutdown = make(chan struct{})
	si = sharedinformers.NewSharedInformers(config, shutdown)

	controller = virtualmachineservice.NewVirtualMachineServiceController(config, si)
	controller.Run(shutdown)
})

var _ = AfterSuite(func() {
	if !execute {
		return
	}
	close(shutdown)
	testenv.Stop()
	vcsim.Stop()
})
