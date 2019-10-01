// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Because of a hard coded path in buildAggregatedAPIServer, all integration tests that are using
// suite.InstallLocalTestingAPIAggregationEnvironment() must be at this level of the directory hierarchy.  Do not try
// and move this test suite until this hard coded path is resolved in apiserver-builder.
package integration

import (
	"testing"

<<<<<<< HEAD
=======
	"k8s.io/client-go/kubernetes"

>>>>>>> Manage VirtualMachineImages asynchronously
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
<<<<<<< HEAD
	"k8s.io/client-go/kubernetes"
=======
>>>>>>> Manage VirtualMachineImages asynchronously
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
<<<<<<< HEAD
	restConfig    *rest.Config
	vcSim         *integration.VcSimInstance
	testEnv       *suite.Environment
	vSphereConfig *vsphere.VSphereVmProviderConfig
	session       *vsphere.Session
=======
	vcSim         *integration.VcSimInstance
	testEnv       *suite.Environment
	vSphereConfig *vsphere.VSphereVmProviderConfig
	restConfig    *rest.Config
>>>>>>> Manage VirtualMachineImages asynchronously
	clientSet     *kubernetes.Clientset
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "vSphere Provider Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
<<<<<<< HEAD
	testEnv, vSphereConfig, restConfig, vcSim, session = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
=======
	testEnv, vSphereConfig, restConfig, vcSim, _ = integration.SetupIntegrationEnv()
>>>>>>> Manage VirtualMachineImages asynchronously
	clientSet = kubernetes.NewForConfigOrDie(restConfig)
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
