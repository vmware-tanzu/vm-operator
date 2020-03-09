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

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestInfraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Infra Cluster Provider Suite", []Reporter{envtest.NewlineReporter{}})
}

var (
	cfg       *rest.Config
	vcSim     *integration.VcSimInstance
	testEnv   *envtest.Environment
	clientSet *kubernetes.Clientset
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _ = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(cfg)
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
