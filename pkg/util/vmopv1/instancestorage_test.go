// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("IsInstanceStoragePresent", func() {
	var vm *vmopv1.VirtualMachine

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
	})

	When("VM does not have instance storage", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
				Name: "my-vol",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
						InstanceVolumeClaim: nil,
					},
				},
			})
		})

		It("returns false", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeFalse())
		})
	})

	When("VM has instance storage", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: "my-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-pvc",
							},
						},
					},
				},
				vmopv1.VirtualMachineVolume{
					Name: "my-is-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-is-pvc",
							},
							InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
								StorageClass: "foo",
							},
						},
					},
				})
		})

		It("returns true", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
		})
	})
})

var _ = Describe("FilterInstanceStorageVolumes", func() {
	var vm *vmopv1.VirtualMachine

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
	})

	When("VM has no volumes", func() {
		It("returns empty list", func() {
			Expect(vmopv1util.FilterInstanceStorageVolumes(vm)).To(BeEmpty())
		})
	})

	When("VM has volumes", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: "my-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-pvc",
							},
						},
					},
				},
				vmopv1.VirtualMachineVolume{
					Name: "my-is-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-is-pvc",
							},
							InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
								StorageClass: "foo",
							},
						},
					},
				})
		})

		It("returns instance storage volumes", func() {
			volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0]).To(Equal(vm.Spec.Volumes[1]))
		})
	})
})

var _ = Describe("IsInsufficientQuota", func() {

	It("returns true", func() {
		err := apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("insufficient quota for creating PVC"))
		Expect(vmopv1util.IsInsufficientQuota(err)).To(BeTrue())
	})
})
