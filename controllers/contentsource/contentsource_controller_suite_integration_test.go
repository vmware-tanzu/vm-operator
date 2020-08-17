// +build integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	cfg        *rest.Config
	testEnv    *envtest.Environment
	vcSim      *integration.VcSimInstance
	ctx        = context.Background()
	vmProvider vmprovider.VirtualMachineProviderInterface
)

func TestContentSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "ContentSource Suite", []Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _, vmProvider = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})
