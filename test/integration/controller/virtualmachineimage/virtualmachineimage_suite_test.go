/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage_test

import (
	"testing"

	"github.com/golang/glog"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis"
	vmrest "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/rest"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/clientset_generated/clientset"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/controller/sharedinformers"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/controller/virtualmachineimage"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/openapi"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/vmprovider"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/vmprovider/providers/vsphere"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/test/integration"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var shutdown chan struct{}
var controller *virtualmachineimage.VirtualMachineImageController
var si *sharedinformers.SharedInformers
var vcsim *integration.VcSimInstance
var execute = false

func TestVirtualMachineImage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineImage Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	// TODO: Just return for now
	if !execute {
		return
	}
	vcsim = integration.NewVcSimInstance()
	address, port := vcsim.Start()

	provider, err := vsphere.NewVSphereVmProviderFromConfig(integration.DefaultNamespace, integration.NewIntegrationVmOperatorConfig(address, port))
	if err != nil {
		glog.Fatalf("Failed to create vSphere provider: %v", err)
	}

	vmprovider.RegisterVmProvider(provider)

	if err := v1alpha1.RegisterRestProvider(vmrest.NewVirtualMachineImagesREST(provider)); err != nil {
		glog.Fatalf("Failed to register REST provider: %s", err)
	}

	testenv = test.NewTestEnvironment()
	config = testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs = clientset.NewForConfigOrDie(config)

	shutdown = make(chan struct{})
	si = sharedinformers.NewSharedInformers(config, shutdown)

	controller = virtualmachineimage.NewVirtualMachineImageController(config, si)
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
