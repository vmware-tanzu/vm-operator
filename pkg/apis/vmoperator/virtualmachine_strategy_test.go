/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ = Describe("VirtualMachine Validation", func() {

	Describe("PrepareForCreate", func() {

		It("Sets finalizer", func() {
			v := VirtualMachineStrategy{}
			ctx := context.Background()
			vm := &VirtualMachine{Spec: VirtualMachineSpec{ImageName: "fooImage", ClassName: "fooClass"}}
			v.PrepareForCreate(ctx, vm)
			Expect(vm.Finalizers).Should(HaveLen(1))
			Expect(vm.Finalizers[0]).Should(Equal("virtualmachine.vmoperator.vmware.com"))
		})
	})

	Describe("validate network type", func() {
		type ValidateFunc func(*VirtualMachine) field.ErrorList

		DescribeTable("should reject network interface without name",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).ShouldNot(BeEmpty())

			},
			Entry("nsx-t network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkType: "nsx-t"}}),
			Entry("empty network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkType: ""}}),
		)

		DescribeTable("should reject network interface with invalid type",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).ShouldNot(BeEmpty())
			},
			Entry("invalid network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "invalid-type"}}),
		)

		DescribeTable("should accept valid network interface",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).Should(BeEmpty())
			},
			Entry("nsx-t network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "nsx-t"}}),
			Entry("empty network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: ""}}),
			Entry("no network interface", validateNetworkType, nil),
		)
	})

	Describe("Validate", func() {
		type ValidateObjFunc func(ctx context.Context, obj runtime.Object) field.ErrorList

		DescribeTable("should accept valid vm",
			func(vm *VirtualMachine) {
				v := VirtualMachineStrategy{}
				ctx := context.Background()
				Expect(v.Validate(ctx, vm)).Should(BeEmpty())
			},
			Entry("valid vm", &VirtualMachine{Spec: VirtualMachineSpec{ImageName: "fooImage", ClassName: "fooClass"}}),
		)

		DescribeTable("should reject invalid vm",
			func(vm *VirtualMachine, expectedErr *field.Error) {
				v := VirtualMachineStrategy{}
				ctx := context.Background()

				result := v.Validate(ctx, vm)

				Expect(result).Should(HaveLen(1))
				Expect(result[0]).Should(Equal(expectedErr))
			},
			Entry("invalid image", &VirtualMachine{Spec: VirtualMachineSpec{ClassName: "foo"}},
				field.Required(field.NewPath("spec", "imageName"), "imageName must be provided")),
			Entry("invalid class", &VirtualMachine{Spec: VirtualMachineSpec{ImageName: "foo"}},
				field.Required(field.NewPath("spec", "className"), "className must be provided")),
		)
	})
})
