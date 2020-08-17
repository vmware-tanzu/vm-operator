// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestInfraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Infra Cluster Provider Suite", []Reporter{printer.NewlineReporter{}})
}

var (
	cfg        *rest.Config
	vcSim      *integration.VcSimInstance
	testEnv    *envtest.Environment
	clientSet  *kubernetes.Clientset
	vmProvider vmprovider.VirtualMachineProviderInterface
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(cfg)
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
