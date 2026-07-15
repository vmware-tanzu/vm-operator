// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("VMSpecVolumesPVCsIndexerFunc", func() {
	When("the object is not a VirtualMachine", func() {
		It("should return nil", func() {
			obj := &corev1.Pod{}
			Expect(kubeutil.VMSpecVolumesPVCsIndexerFunc(obj)).To(BeNil())
		})
	})

	When("the object is a VirtualMachine", func() {
		var vm *vmopv1.VirtualMachine

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
		})

		When("there are no volumes", func() {
			It("should return an empty slice", func() {
				Expect(kubeutil.VMSpecVolumesPVCsIndexerFunc(vm)).To(BeEmpty())
			})
		})

		When("there are non-PVC volumes", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name:                       "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{},
					},
				}
			})
			It("should return an empty slice", func() {
				Expect(kubeutil.VMSpecVolumesPVCsIndexerFunc(vm)).To(BeEmpty())
			})
		})

		When("there is a PVC volume with an empty claim name", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "",
								},
							},
						},
					},
				}
			})
			It("should return an empty slice", func() {
				Expect(kubeutil.VMSpecVolumesPVCsIndexerFunc(vm)).To(BeEmpty())
			})
		})

		When("there are one or more PVC volumes with claim names", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
							},
						},
					},
					{
						Name:                       "vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{},
					},
					{
						Name: "vol3",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
							},
						},
					},
				}
			})
			It("should return the claim names in spec order", func() {
				Expect(kubeutil.VMSpecVolumesPVCsIndexerFunc(vm)).To(Equal([]string{"pvc1", "pvc2"}))
			})
		})
	})
})
