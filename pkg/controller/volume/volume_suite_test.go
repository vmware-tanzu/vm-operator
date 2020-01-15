// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package volume

import (
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestVirtualMachine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Volume Suite", []Reporter{test.NewlineReporter{}})
}

var (
	cfg           *rest.Config
	vcSim         *integration.VcSimInstance
	testEnv       *suite.Environment
	err           error
	vSphereConfig *vsphere.VSphereVmProviderConfig
	session       *vsphere.Session
)

var _ = BeforeSuite(func() {
	testEnv, vSphereConfig, cfg, vcSim, session = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
