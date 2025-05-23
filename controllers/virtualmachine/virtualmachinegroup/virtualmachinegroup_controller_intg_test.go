// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyImage   = "dummy-image"
	dummyClass   = "dummy-class"
	storageClass = "my-storage-class"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx *builder.IntegrationTestContext

		vm1Key, vm2Key, vm3Key                types.NamespacedName
		vmGroup1Key, vmGroup2Key, vmGroup3Key types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vmGroup1Key = types.NamespacedName{
			Name:      "vmgroup-1",
			Namespace: ctx.Namespace,
		}
		vmGroup2Key = types.NamespacedName{
			Name:      "vmgroup-2",
			Namespace: ctx.Namespace,
		}
		vmGroup3Key = types.NamespacedName{
			Name:      "vmgroup-3",
			Namespace: ctx.Namespace,
		}
		vm1Key = types.NamespacedName{
			Name:      "vm-1",
			Namespace: ctx.Namespace,
		}
		vm2Key = types.NamespacedName{
			Name:      "vm-2",
			Namespace: ctx.Namespace,
		}
		vm3Key = types.NamespacedName{
			Name:      "vm-3",
			Namespace: ctx.Namespace,
		}

		// Create VM and VM Group objects used in the tests.
		vmGroup1 := &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vmGroup1Key.Namespace,
				Name:      vmGroup1Key.Name,
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				Members: []vmopv1.GroupMember{
					{
						Kind: virtualMachineKind,
						Name: vm1Key.Name,
					},
					{
						Kind: virtualMachineGroupKind,
						Name: vmGroup2Key.Name,
					},
					{
						Kind: virtualMachineGroupKind,
						Name: vmGroup3Key.Name,
					},
				},
			},
		}
		Expect(ctx.Client.Create(ctx, vmGroup1)).To(Succeed())

		vm1 := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm1Key.Namespace,
				Name:      vm1Key.Name,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    dummyImage,
				ClassName:    dummyClass,
				StorageClass: storageClass,
			},
		}
		Expect(ctx.Client.Create(ctx, vm1)).To(Succeed())

		vm2 := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm2Key.Namespace,
				Name:      vm2Key.Name,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    dummyImage,
				ClassName:    dummyClass,
				StorageClass: storageClass,
			},
		}
		Expect(ctx.Client.Create(ctx, vm2)).To(Succeed())

		vm3 := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm3Key.Namespace,
				Name:      vm3Key.Name,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    dummyImage,
				ClassName:    dummyClass,
				StorageClass: storageClass,
			},
		}
		Expect(ctx.Client.Create(ctx, vm3)).To(Succeed())

		vmGroup2 := &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vmGroup2Key.Namespace,
				Name:      vmGroup2Key.Name,
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				Members: []vmopv1.GroupMember{
					{
						Kind: virtualMachineKind,
						Name: vm2Key.Name,
					},
				},
			},
		}
		Expect(ctx.Client.Create(ctx, vmGroup2)).To(Succeed())

		vmGroup3 := &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vmGroup3Key.Namespace,
				Name:      vmGroup3Key.Name,
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				Members: []vmopv1.GroupMember{
					{
						Kind: virtualMachineKind,
						Name: vm3Key.Name,
					},
				},
			},
		}
		Expect(ctx.Client.Create(ctx, vmGroup3)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {

		It("Reconciles after VirtualMachineGroup creation", func() {

			By("VirtualMachineGroup should have finalizer added", func() {
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(vmGroup1.GetFinalizers()).To(ContainElement(finalizer))

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					g.Expect(vmGroup2.GetFinalizers()).To(ContainElement(finalizer))

					vmGroup3 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup3Key, vmGroup3)).To(Succeed())
					g.Expect(vmGroup3.GetFinalizers()).To(ContainElement(finalizer))
				}).Should(Succeed(), "waiting for VirtualMachineGroup finalizer")
			})

			By("Group members should have owner references set to its direct parent group", func() {
				Eventually(func(g Gomega) {
					// vm1 is a direct member of vmGroup1.
					vm1 := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					g.Expect(vm1.OwnerReferences).To(HaveLen(1))
					g.Expect(vm1.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

					// vmGroup2 is a direct member of vmGroup1.
					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					g.Expect(vmGroup2.OwnerReferences).To(HaveLen(1))
					g.Expect(vmGroup2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

					// vmGroup3 is a direct member of vmGroup1.
					vmGroup3 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup3Key, vmGroup3)).To(Succeed())
					g.Expect(vmGroup3.OwnerReferences).To(HaveLen(1))
					g.Expect(vmGroup3.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

					// vm2 is a direct member of vmGroup2.
					vm2 := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
					g.Expect(vm2.OwnerReferences).To(HaveLen(1))
					g.Expect(vm2.OwnerReferences[0].Name).To(Equal(vmGroup2Key.Name))

					// vm3 is a direct member of vmGroup3.
					vm3 := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vm3Key, vm3)).To(Succeed())
					g.Expect(vm3.OwnerReferences).To(HaveLen(1))
					g.Expect(vm3.OwnerReferences[0].Name).To(Equal(vmGroup3Key.Name))
				}).Should(Succeed(), "waiting for VirtualMachineGroup owner references to be set")
			})
		})
	})
}
