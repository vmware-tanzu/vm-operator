// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Because of a hard coded path in buildAggregatedAPIServer, all integration tests that are using
// suite.InstallLocalTestingAPIAggregationEnvironment() must be at this level of the directory hierarchy.  Do not try
// and move this test suite until this hard coded path is resolved in apiserver-builder.
package integration

import (
	"context"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	restConfig    *rest.Config
	vcSim         *integration.VcSimInstance
	testEnv       *suite.Environment
	vSphereConfig *vsphere.VSphereVmProviderConfig
	session       *vsphere.Session
	clientSet     *kubernetes.Clientset

	err error
	ctx context.Context
	c   *vsphere.Client
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "vSphere Provider Suite", []Reporter{test.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testEnv, vSphereConfig, restConfig, vcSim, session = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(restConfig)

	ctx = context.Background()

	c, err = vsphere.NewClient(ctx, vSphereConfig)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
