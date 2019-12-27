// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"context"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/test/suite"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

func TestVirtualMachineService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "VirtualMachineService Suite", []Reporter{test.NewlineReporter{}})
}

var (
	cfg     *rest.Config
	vcSim   *integration.VcSimInstance
	testEnv *suite.Environment
)

var _ = BeforeSuite(func() {
	testEnv, _, cfg, vcSim, _ = integration.SetupIntegrationEnv([]string{integration.DefaultNamespace})
})

var _ = AfterSuite(func() {
	integration.TeardownIntegrationEnv(testEnv, vcSim)
})

func createObjects(ctx context.Context, ctrlClient client.Client, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Create(ctx, obj)).To(Succeed())
	}
}

func updateObjectsStatus(ctx context.Context, ctrlClient client.StatusClient, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Status().Update(ctx, obj)).To(Succeed())
	}
}

func deleteObjects(ctx context.Context, ctrlClient client.Client, runtimeObjects []runtime.Object) {
	for _, obj := range runtimeObjects {
		Expect(ctrlClient.Delete(ctx, obj)).To(Succeed())
	}
}

func assertEventuallyExistsInNamespace(ctx context.Context, ctrlClient client.Client, namespace, name string, obj runtime.Object) {
	EventuallyWithOffset(2, func() error {
		key := client.ObjectKey{Namespace: namespace, Name: name}
		return ctrlClient.Get(ctx, key, obj)
	}, timeout).Should(Succeed())
}

func assertService(ctx context.Context, ctrlClient client.Client, namespace, name string) {
	service := &corev1.Service{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, service)
}

func assertServiceWithNoEndpointSubsets(ctx context.Context, ctrlClient client.Client, namespace, name string) {
	assertService(ctx, ctrlClient, namespace, name)
	endpoints := &corev1.Endpoints{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, endpoints)
	Expect(endpoints.Subsets).To(BeEmpty())
}

func assertServiceWithEndpointSubsets(ctx context.Context, ctrlClient client.Client, namespace, name string) []corev1.EndpointSubset {
	assertService(ctx, ctrlClient, namespace, name)
	endpoints := &corev1.Endpoints{}
	assertEventuallyExistsInNamespace(ctx, ctrlClient, namespace, name, endpoints)
	Expect(endpoints.Subsets).NotTo(BeEmpty())
	return endpoints.Subsets
}
