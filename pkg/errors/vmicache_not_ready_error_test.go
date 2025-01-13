// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
)

var _ = Describe("VMICacheNotReadyError", func() {

	Context("Error", func() {
		It("should return \"cache not ready\"", func() {
			Expect(pkgerr.VMICacheNotReadyError{}.Error()).To(Equal("cache not ready"))
		})
	})

	DescribeTable("WatchVMICacheIfNotReady",
		func(e error, obj metav1.Object, expResult bool, expName string) {
			ok := pkgerr.WatchVMICacheIfNotReady(e, obj)
			Expect(ok).To(Equal(expResult))
			if obj != nil {
				labels := obj.GetLabels()
				if ok {
					Expect(labels).To(HaveKeyWithValue(
						pkgconst.VMICacheLabelKey, expName))
				} else {
					Expect(labels).ToNot(HaveKey(pkgconst.VMICacheLabelKey))
				}
			}
		},

		Entry(
			"error is nil",
			(error)(nil),
			&corev1.ConfigMap{},
			false,
			"",
		),

		Entry(
			"object is nil",
			errors.New("hi"),
			(metav1.Object)(nil),
			false,
			"",
		),

		Entry(
			"error is not VMICacheNotReadyError",
			errors.New("hi"),
			&corev1.ConfigMap{},
			false,
			"",
		),

		Entry(
			"error is VMICacheNotReadyError",
			pkgerr.VMICacheNotReadyError{Name: "vmi-123"},
			&corev1.ConfigMap{},
			true,
			"vmi-123",
		),

		Entry(
			"err is wrapped VMICacheNotReadyError",
			fmt.Errorf("hi: %w", pkgerr.VMICacheNotReadyError{Name: "vmi-123"}),
			&corev1.ConfigMap{},
			true,
			"vmi-123",
		),

		Entry(
			"err is VMICacheNotReadyError wrapped with multiple errors",
			fmt.Errorf(
				"hi: %w",
				fmt.Errorf("there: %w, %w",
					errors.New("hello"),
					pkgerr.VMICacheNotReadyError{Name: "vmi-123"},
				),
			),
			&corev1.ConfigMap{},
			true,
			"vmi-123",
		),
	)
})
