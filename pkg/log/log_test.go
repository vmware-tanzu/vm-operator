// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package log_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

var _ = Describe("FromContextOrDefault", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	When("there is a logger in the context", func() {
		BeforeEach(func() {
			ctx = logr.NewContext(ctx, logr.Discard())
		})
		It("should return a logger", func() {
			Expect(pkglog.FromContextOrDefault(ctx)).ToNot(BeNil())
		})
	})
	When("there is not a logger in the context", func() {
		It("should return a logger", func() {
			Expect(pkglog.FromContextOrDefault(ctx)).ToNot(BeNil())
		})
	})
})

var _ = Describe("ControllerLogConstructor", func() {
	var (
		scheme *runtime.Scheme
		obj    runtime.Object
		req    *reconcile.Request
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		obj = &unstructured.Unstructured{}
		req = &reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace-1",
				Name:      "my-name-1",
			},
		}
	})
	When("scheme has gvk", func() {
		BeforeEach(func() {
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			obj = &corev1.Pod{}
		})
		It("should work", func() {
			fn := pkglog.ControllerLogConstructor("", obj, scheme)
			Expect(fn).ToNot(BeNil())
			Expect(fn(req)).ToNot(BeNil())
		})
	})
	When("scheme does not have gvk", func() {
		It("should work", func() {
			fn := pkglog.ControllerLogConstructor("", obj, scheme)
			Expect(fn).ToNot(BeNil())
			Expect(fn(req)).ToNot(BeNil())
		})
	})
})
