// +build integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	vmopclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	vmopsession "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	restConfig    *rest.Config
	vcSim         *integration.VcSimInstance
	testEnv       *envtest.Environment
	vSphereConfig *config.VSphereVmProviderConfig
	vmClient      *vmopclient.Client
	vmProvider    vmprovider.VirtualMachineProviderInterface
	clientSet     *kubernetes.Clientset
	k8sClient     client.Client
	session       *vmopsession.Session

	err error
	ctx context.Context
	c   *vmopclient.Client
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "vSphere Provider Suite", []Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testEnv, vSphereConfig, restConfig, vcSim, vmClient, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
	clientSet = kubernetes.NewForConfigOrDie(restConfig)

	k8sClient, err = integration.GetCtrlRuntimeClient(restConfig)
	Expect(err).NotTo(HaveOccurred())

	ctx = context.Background()

	c, err = vmProvider.(vsphere.VSphereVmProviderGetSessionHack).GetClient(ctx)
	Expect(c).ToNot(BeNil())
	Expect(err).NotTo(HaveOccurred())

	session, err = vmopsession.NewSessionAndConfigure(ctx, c, vSphereConfig, k8sClient, nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(session).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
