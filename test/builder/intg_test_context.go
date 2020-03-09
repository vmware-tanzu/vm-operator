// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"

	uuid "github.com/satori/go.uuid"
	//nolint
	. "github.com/onsi/ginkgo"
	//nolint
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	//"github.com/vmware-tanzu/vm-operator/test/testutil"
)

// IntegrationTestContext is used for integration testing VmOperatorControllers.
type IntegrationTestContext struct {
	context.Context
	Client          client.Client
	Namespace       string
	envTest         *envtest.Environment
	suite           *TestSuite
	StopManager     func()
	StartNewManager func(*IntegrationTestContext)
}

// AfterEach should be invoked by ginkgo.AfterEach to stop the VM Operator API server.
func (ctx *IntegrationTestContext) AfterEach() {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ctx.Namespace,
		},
	}
	By("Destroying integration test namespace")
	Expect(ctx.suite.integrationTestClient.Delete(ctx, namespace)).To(Succeed())

	if ctx.envTest != nil {
		By("Shutting down vm operator control plane")
		Expect(ctx.envTest.Stop()).To(Succeed())
	}
}

// NewIntegrationTestContext should be invoked by ginkgo.BeforeEach
//
// This function returns a TestSuite context
// The resources created by this function may be cleaned up by calling AfterEach
// with the IntegrationTestContext returned by this function
func (s *TestSuite) NewIntegrationTestContext() *IntegrationTestContext {
	ctx := &IntegrationTestContext{
		Context:         context.Background(),
		Client:          s.integrationTestClient,
		suite:           s,
		StopManager:     s.stopManager,
		StartNewManager: s.startNewManager,
	}

	By("Creating a temporary namespace", func() {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewV4().String(),
			},
		}
		Expect(ctx.Client.Create(s, namespace)).To(Succeed())

		ctx.Namespace = namespace.Name
	})

	return ctx
}
