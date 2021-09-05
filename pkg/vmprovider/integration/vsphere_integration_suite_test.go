// +build integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	vcsession "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	vcSim         *integration.VcSimInstance
	testEnv       *envtest.Environment
	vSphereConfig *config.VSphereVMProviderConfig
	vcClient      *vcclient.Client
	vmProvider    vmprovider.VirtualMachineProviderInterface
	k8sClient     client.Client
	session       *vcsession.Session

	err error
	ctx context.Context
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "vSphere Provider Suite", []Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	ctx = context.Background()

	testEnv, vSphereConfig, k8sClient, vcSim, vcClient, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})

	session, err = vcsession.NewSessionAndConfigure(ctx, vcClient, vSphereConfig, k8sClient)
	Expect(err).NotTo(HaveOccurred())
	Expect(session).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
