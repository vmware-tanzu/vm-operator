/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"os"
	"testing"

	"k8s.io/klog/klogr"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"

	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	vmrest "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/rest"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var vcsim *integration.VcSimInstance
var log = klogr.New()

func TestV1alpha1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "v1 Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	vcsim = integration.NewVcSimInstance()
	address, port := vcsim.Start()

	provider, err := vsphere.NewVSphereVmProviderFromConfig(integration.DefaultNamespace, integration.NewIntegrationVmOperatorConfig(address, port))
	if err != nil {
		log.Error(err, "Failed to create vSphere provider")
		os.Exit(255)
	}

	vmprovider.RegisterVmProvider(provider)

	if err := vmoperator.RegisterRestProvider(vmrest.NewVirtualMachineImagesREST(provider)); err != nil {
		log.Error(err, "Failed to register REST provider")
		os.Exit(255)
	}

	testenv = test.NewTestEnvironment()
	config = testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs = clientset.NewForConfigOrDie(config)
})

var _ = AfterSuite(func() {
	testenv.Stop()
	vcsim.Stop()
})
