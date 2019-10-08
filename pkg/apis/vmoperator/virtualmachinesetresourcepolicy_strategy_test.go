// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func aResourcePoolSpec(reservations, limits VirtualMachineResourceSpec) ResourcePoolSpec {
	return ResourcePoolSpec{
		Reservations: reservations,
		Limits:       limits,
	}
}

func aFolderSpec(folderName string) FolderSpec {
	return FolderSpec{
		Name: folderName,
	}
}

func aVirtualMachineSetResourcePolicy(rpSpec ResourcePoolSpec, folderSpec FolderSpec) VirtualMachineSetResourcePolicy {
	return VirtualMachineSetResourcePolicy{
		Spec: VirtualMachineSetResourcePolicySpec{
			ResourcePool: rpSpec,
			Folder:       folderSpec,
		},
	}
}

func aVirtualMachineResourceSpec(cpu, memory string) VirtualMachineResourceSpec {
	return VirtualMachineResourceSpec{
		Cpu:    resource.MustParse(cpu),
		Memory: resource.MustParse(memory),
	}
}

var _ = Describe("VirtualMachineSetResourcePolicy Validation", func() {

	Context("with valid resource pool and folder spec", func() {
		Specify("should validate without errors", func() {
			res := aVirtualMachineResourceSpec("1Gi", "1Gi")
			limit := aVirtualMachineResourceSpec("1Gi", "1Gi")

			rpSpec := aResourcePoolSpec(res, limit)
			folderSpec := aFolderSpec("some-name")
			resourcePolicy := aVirtualMachineSetResourcePolicy(rpSpec, folderSpec)

			st := VirtualMachineSetResourcePolicyStrategy{}
			result := st.Validate(context.TODO(), &resourcePolicy)
			Expect(result).Should(BeEmpty())
		})
	})

	Context("when memory reservation is more than limit", func() {
		Specify("should fail", func() {
			res := aVirtualMachineResourceSpec("1Gi", "2Gi")
			limit := aVirtualMachineResourceSpec("1Gi", "1Gi")

			rpSpec := aResourcePoolSpec(res, limit)
			folderSpec := aFolderSpec("some-name")
			resourcePolicy := aVirtualMachineSetResourcePolicy(rpSpec, folderSpec)
			st := VirtualMachineSetResourcePolicyStrategy{}
			result := st.Validate(context.TODO(), &resourcePolicy)

			Expect(result).Should(HaveLen(1))
			expectedErr := field.Invalid(
				field.NewPath("spec", "resourcepool", "reservations", "memory"),
				res.Memory.Value(), "Memory reservation should not be larger than Memory limit")
			Expect(result[0]).Should(Equal(expectedErr))
		})
	})

	Context("when CPU reservation is more than limit", func() {
		Specify("should fail", func() {
			res := aVirtualMachineResourceSpec("2Gi", "1Gi")
			limit := aVirtualMachineResourceSpec("1Gi", "1Gi")

			rpSpec := aResourcePoolSpec(res, limit)
			folderSpec := aFolderSpec("some-name")
			resourcePolicy := aVirtualMachineSetResourcePolicy(rpSpec, folderSpec)
			st := VirtualMachineSetResourcePolicyStrategy{}
			result := st.Validate(context.TODO(), &resourcePolicy)
			Expect(result).To(HaveLen(1))
			expectedErr := field.Invalid(
				field.NewPath("spec", "resourcepool", "reservations", "cpu"),
				res.Cpu.Value(), "CPU reservation should not be larger than CPU limit")
			Expect(result[0]).Should(Equal(expectedErr))
		})
	})
})
