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

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestVirtualMachineSetResourcePolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineSetResourcePolicy Suite", []Reporter{envtest.NewlineReporter{}})
}

var (
	cfg     *rest.Config
	vcSim   *integration.VcSimInstance
	testEnv *envtest.Environment
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _ = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
