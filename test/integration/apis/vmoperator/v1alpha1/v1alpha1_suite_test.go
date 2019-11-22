// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmrest "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/rest"
	"github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	testenv *test.TestEnvironment
	config  *rest.Config
	cs      *clientset.Clientset
	vcsim   *integration.VcSimInstance
	log     = logf.Log.WithName("vmoperatorv1alpha1")
)

func TestV1alpha1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "v1 Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	vcsim = integration.NewVcSimInstance()
	address, port := vcsim.Start()

	vSphereProvider, err := vsphere.NewVSphereMachineProviderFromConfig(integration.DefaultNamespace, integration.NewIntegrationVmOperatorConfig(address, port, ""))
	if err != nil {
		log.Error(err, "Failed to create vSphere provider")
		os.Exit(255)
	}

	vmProvider := vSphereProvider.(vmprovider.VirtualMachineProviderInterface)
	vmprovider.GetService().RegisterVmProvider(vmProvider)

	if err := vmoperator.RegisterRestProvider(vmrest.NewVirtualMachineImagesREST(vmProvider)); err != nil {
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
