// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("SyncGVKToObject", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
	})

	When("the provided object is not registered with the provided scheme", func() {
		It("should return an error", func() {
			obj := &corev1.Pod{}
			Expect(kubeutil.SyncGVKToObject(obj, scheme)).ToNot(Succeed())
			Expect(obj.APIVersion).To(BeEmpty())
			Expect(obj.Kind).To(BeEmpty())
		})
	})

	When("the provided object is registered with the provided scheme", func() {
		BeforeEach(func() {
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
		})
		It("should return the correct gvk", func() {
			obj := &corev1.Pod{}
			Expect(kubeutil.SyncGVKToObject(obj, scheme)).To(Succeed())
			Expect(obj.APIVersion).To(Equal("v1"))
			Expect(obj.Kind).To(Equal("Pod"))
		})
	})
})
