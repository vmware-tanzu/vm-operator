// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
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

		DescribeTable("should reject network interface without name",
			func(vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateNetworkType(vm)).ShouldNot(BeEmpty())

			},
			Entry("nsx-t network type", []VirtualMachineNetworkInterface{{NetworkType: "nsx-t"}}),
			Entry("empty network type", []VirtualMachineNetworkInterface{{NetworkType: ""}}),
		)

		DescribeTable("should reject network interface with invalid type",
			func(vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateNetworkType(vm)).ShouldNot(BeEmpty())
			},
			Entry("invalid network type", []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "invalid-type"}}),
		)

		DescribeTable("should accept valid network interface",
			func(vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateNetworkType(vm)).Should(BeEmpty())
			},
			Entry("nsx-t network type", []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "nsx-t"}}),
			Entry("empty network type", []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: ""}}),
			Entry("no network interface", nil),
		)
	})

	Describe("validate volumes type", func() {

		DescribeTable("should accept valid volumes",
			func(fieldErrors field.ErrorList, volumes []VirtualMachineVolumes) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						Volumes: volumes,
					},
				}
				Expect(validateVolumes(vm)).Should(Equal(fieldErrors))
			},
			Entry("volume with valid pvc", field.ErrorList{}, []VirtualMachineVolumes{
				{
					Name: "test-volume-1",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "dummy-pvc"},
				},
				{
					Name: "test-volume-2",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "dummy-pvc-2"},
				},
			}),
			Entry("no volumes", field.ErrorList{}, nil),
		)

		DescribeTable("should reject invalid volumes",
			func(fieldErrors field.ErrorList, volumes []VirtualMachineVolumes) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						Volumes: volumes,
					},
				}
				Expect(validateVolumes(vm)).Should(Equal(fieldErrors))
			},

			Entry("volume with no pvc", field.ErrorList{
				{
					Type:     field.ErrorTypeRequired,
					Field:    "spec.volumes[0].persistentVolumeClaim.claimName",
					BadValue: "",
				},
			}, []VirtualMachineVolumes{
				{
					Name: "test-volume-0",
				},
			}),

			Entry("volume with no name", field.ErrorList{
				{
					Type:     field.ErrorTypeRequired,
					Field:    "spec.volumes[0].name",
					BadValue: "",
				},
				{
					Type:     field.ErrorTypeRequired,
					Field:    "spec.volumes[0].persistentVolumeClaim.claimName",
					BadValue: "",
				},
			}, []VirtualMachineVolumes{
				{
					Name: "",
				},
			}),

			Entry("volume with duplicate names", field.ErrorList{
				{
					Type:     field.ErrorTypeDuplicate,
					Field:    "spec.volumes[1].name",
					BadValue: "test-volume-1",
				},
			}, []VirtualMachineVolumes{
				{
					Name: "test-volume-1",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "dummy-pvc-1"},
				},
				{
					Name: "test-volume-1",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "dummy-pvc-2"},
				},
			}),

			Entry("volume with no pvc name", field.ErrorList{
				{
					Type:     field.ErrorTypeRequired,
					Field:    "spec.volumes[0].persistentVolumeClaim.claimName",
					BadValue: "",
				},
			}, []VirtualMachineVolumes{
				{
					Name: "test-volume-3",
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: ""},
				},
			}),
		)
	})

	Describe("Validate", func() {

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
