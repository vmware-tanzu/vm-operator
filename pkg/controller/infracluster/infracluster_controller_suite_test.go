// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestInfraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Infra Cluster Provider Suite", []Reporter{test.NewlineReporter{}})
}

var (
	cfg       *rest.Config
	vcSim     *integration.VcSimInstance
	testEnv   *suite.Environment
	clientSet *kubernetes.Clientset
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _ = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(cfg)
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
