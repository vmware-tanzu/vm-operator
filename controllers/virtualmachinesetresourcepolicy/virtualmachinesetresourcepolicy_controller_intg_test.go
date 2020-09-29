// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachinesetresourcepolicy

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestVirtualMachineSetResourcePolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineSetResourcePolicy Suite", []Reporter{printer.NewlineReporter{}})
}

var (
	cfg        *rest.Config
	vcSim      *integration.VcSimInstance
	testEnv    *envtest.Environment
	vmProvider vmprovider.VirtualMachineProviderInterface
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
