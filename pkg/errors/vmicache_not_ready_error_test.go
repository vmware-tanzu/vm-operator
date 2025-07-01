// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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
		It("should return \"disks not ready\"", func() {
			Expect(pkgerr.VMICacheNotReadyError{
				Message: "disks not ready"}.Error()).To(Equal("disks not ready"))
		})
	})

	DescribeTable("WatchVMICacheIfNotReady",
		func(
			e error,
			obj metav1.Object,
			expResult bool,
			expName,
			expLocation string) {

			Expect(pkgerr.WatchVMICacheIfNotReady(e, obj)).To(Equal(expResult))

			assertMapKeyVal := func(
				m map[string]string,
				k, v string) {

				if v != "" {
					ExpectWithOffset(1, m).To(HaveKeyWithValue(k, v))
				} else {
					ExpectWithOffset(1, m).ToNot(HaveKey(k))
				}

			}
			if obj != nil {
				assertMapKeyVal(
					obj.GetLabels(),
					pkgconst.VMICacheLabelKey,
					expName)
				assertMapKeyVal(
					obj.GetAnnotations(),
					pkgconst.VMICacheLocationAnnotationKey,
					expLocation)
			}
		},

		Entry(
			"error is nil",
			(error)(nil),
			&corev1.ConfigMap{},
			false,
			"",
			"",
		),

		Entry(
			"object is nil",
			errors.New("hi"),
			(metav1.Object)(nil),
			false,
			"",
			"",
		),

		Entry(
			"error is not VMICacheNotReadyError",
			errors.New("hi"),
			&corev1.ConfigMap{},
			false,
			"",
			"",
		),

		Entry(
			"error is VMICacheNotReadyError",
			pkgerr.VMICacheNotReadyError{
				Name: "vmi-123",
			},
			&corev1.ConfigMap{},
			true,
			"vmi-123",
			"",
		),

		Entry(
			"err is wrapped VMICacheNotReadyError",
			fmt.Errorf("hi: %w", pkgerr.VMICacheNotReadyError{
				Name:         "vmi-123",
				DatacenterID: "dc-123",
				DatastoreID:  "ds-123",
			}),
			&corev1.ConfigMap{},
			true,
			"vmi-123",
			"dc-123,ds-123",
		),

		Entry(
			"err is VMICacheNotReadyError wrapped with multiple errors",
			fmt.Errorf(
				"hi: %w",
				fmt.Errorf("there: %w, %w",
					errors.New("hello"),
					pkgerr.VMICacheNotReadyError{
						Name:         "vmi-123",
						DatacenterID: "dc-123",
						DatastoreID:  "ds-123",
					},
				),
			),
			&corev1.ConfigMap{},
			true,
			"vmi-123",
			"dc-123,ds-123",
		),
	)
})
