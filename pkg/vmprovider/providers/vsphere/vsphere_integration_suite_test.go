// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	stdlog "log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	vcSim  *integration.VcSimInstance
	config *vsphere.VSphereVmProviderConfig
	err    error
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}

var _ = BeforeSuite(func() {
	stdlog.Print("setting up integration test env..")
	vcSim = integration.NewVcSimInstance()
	address, port := vcSim.Start()
	config = integration.NewIntegrationVmOperatorConfig(address, port)
})

var _ = AfterSuite(func() {
	integration.CleanupEnv(vcSim)
})
