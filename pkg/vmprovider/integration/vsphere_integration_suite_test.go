// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package integration

import (
	"context"
	"testing"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	restConfig        *rest.Config
	vcSim             *integration.VcSimInstance
	testEnv           *envtest.Environment
	vSphereConfig     *vsphere.VSphereVmProviderConfig
	session           *vsphere.Session
	vmProvider        vmprovider.VirtualMachineProviderInterface
	clientSet         *kubernetes.Clientset
	ctrlruntimeClient client.Client

	err error
	ctx context.Context
	c   *vsphere.Client
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "vSphere Provider Suite", []Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testEnv, vSphereConfig, restConfig, vcSim, session, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(restConfig)

	ctrlruntimeClient, err = integration.GetVmopClient(restConfig)
	Expect(err).NotTo(HaveOccurred())

	ctx = context.Background()

	c, err = vsphere.NewClient(ctx, vSphereConfig)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
