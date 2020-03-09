// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman_test

import (
	"context"
	"testing"

	//nolint
	. "github.com/onsi/ginkgo"
	//nolint
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var cfg *rest.Config
var suiteCtx = context.Background()
var k8sClient client.Client
var testEnv *envtest.Environment

func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klogr.New())
}

func TestCertMan(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t,
		"Certificate manager test suite",
		[]Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
