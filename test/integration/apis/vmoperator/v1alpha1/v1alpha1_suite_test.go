/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"github.com/golang/glog"
	"testing"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"
	"vmware.com/kubevsphere/test/integration"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"vmware.com/kubevsphere/pkg/apis"
	vmrest "vmware.com/kubevsphere/pkg/apis/vmoperator/rest"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	"vmware.com/kubevsphere/pkg/openapi"
)

var testenv *test.TestEnvironment
var config *rest.Config
var cs *clientset.Clientset
var vcsim *integration.VcSimInstance

func TestV1alpha1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "v1 Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
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
})

var _ = AfterSuite(func() {
	testenv.Stop()
	vcsim.Stop()
})
