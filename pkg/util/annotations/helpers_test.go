// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package annotations_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/util/annotations"
)

var _ = DescribeTable(
	"HasPaused",
	func(in map[string]string, out bool) {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: in,
			},
		}
		actual := annotations.HasPaused(vm)
		Expect(actual).To(Equal(out))
	},
	Entry("nil", nil, false),
	Entry("not present", map[string]string{"foo": "bar"}, false),
	Entry("present but empty ", map[string]string{vmopv1.PauseAnnotation: ""}, true),
	Entry("present and not empty ", map[string]string{vmopv1.PauseAnnotation: "true"}, true),
)

var _ = DescribeTable(
	"HasForceEnableBackup",
	func(in map[string]string, out bool) {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: in,
			},
		}
		actual := annotations.HasForceEnableBackup(vm)
		Expect(actual).To(Equal(out))
	},
	Entry("nil", nil, false),
	Entry("not present", map[string]string{"foo": "bar"}, false),
	Entry("present but empty ", map[string]string{vmopv1.ForceEnableBackupAnnotation: ""}, true),
	Entry("present and not empty ", map[string]string{vmopv1.ForceEnableBackupAnnotation: "true"}, true),
)

var _ = DescribeTable(
	"HasRestoredVM",
	func(in map[string]string, out bool) {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: in,
			},
		}
		actual := annotations.HasRestoredVM(vm)
		Expect(actual).To(Equal(out))
	},
	Entry("nil", nil, false),
	Entry("not present", map[string]string{"foo": "bar"}, false),
	Entry("present but empty ", map[string]string{vmopv1.RestoredVMAnnotation: ""}, true),
	Entry("present and not empty ", map[string]string{vmopv1.RestoredVMAnnotation: "true"}, true),
)

var _ = DescribeTable(
	"HasImportVM",
	func(in map[string]string, out bool) {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: in,
			},
		}
		actual := annotations.HasImportVM(vm)
		Expect(actual).To(Equal(out))
	},
	Entry("nil", nil, false),
	Entry("not present", map[string]string{"foo": "bar"}, false),
	Entry("present but empty ", map[string]string{vmopv1.ImportedVMAnnotation: ""}, true),
	Entry("present and not empty ", map[string]string{vmopv1.ImportedVMAnnotation: "true"}, true),
)

var _ = DescribeTable(
	"HasFailOverVM",
	func(in map[string]string, out bool) {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: in,
			},
		}
		actual := annotations.HasFailOverVM(vm)
		Expect(actual).To(Equal(out))
	},
	Entry("nil", nil, false),
	Entry("not present", map[string]string{"foo": "bar"}, false),
	Entry("present but empty ", map[string]string{vmopv1.FailedOverVMAnnotation: ""}, true),
	Entry("present and not empty ", map[string]string{vmopv1.FailedOverVMAnnotation: "true"}, true),
)
