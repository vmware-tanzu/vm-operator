/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass_test

import (
	"testing"

	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachineclass"

	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	vmrest "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/rest"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/sharedinformers"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var shutdown chan struct{}
var controller *virtualmachineclass.VirtualMachineClassController
var si *sharedinformers.SharedInformers
var vcsim *integration.VcSimInstance

func TestVirtualMachineClass(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineClass Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	vcsim = integration.NewVcSimInstance()
	address, port := vcsim.Start()

	provider, err := vsphere.NewVSphereVmProviderFromConfig(integration.DefaultNamespace, integration.NewIntegrationVmOperatorConfig(address, port), integration.NewIntegrationVmOperatorCredentials())
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
	controller = virtualmachineclass.NewVirtualMachineClassController(config, si)
	controller.Run(shutdown)
})

var _ = AfterSuite(func() {
	close(shutdown)
	testenv.Stop()
	vcsim.Stop()
})
