// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesnapshot_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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
		ctx        *builder.IntegrationTestContext
		vmSnapshot *vmopv1.VirtualMachineSnapshot
		vm         *vmopv1.VirtualMachine
	)

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "vmoperator.vmware.com/v1alpha4",
				Kind:       "VirtualMachine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
		}

		vmSnapshot = &vmopv1.VirtualMachineSnapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "vmoperator.vmware.com/v1alpha4",
				Kind:       "VirtualMachineSnapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-1",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineSnapshotSpec{
				VMRef: &vmopv1common.LocalObjectRef{
					APIVersion: vm.APIVersion,
					Kind:       vm.Kind,
					Name:       vm.Name,
				},
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			vm.Status.UniqueID = "unique-vm-id"
			Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmSnapshot)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("vm resource successfully patched with current snapshot", func() {
			vmObjKey := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
			Eventually(func(g Gomega) {
				vmObj := getVirtualMachine(ctx, vmObjKey)
				g.Expect(vmObj).ToNot(BeNil())
				g.Expect(vmObj.Spec.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
					APIVersion: "vmoperator.vmware.com/v1alpha4",
					Kind:       "VirtualMachineSnapshot",
					Name:       vmSnapshot.Name,
				}))
			}).Should(Succeed(), "waiting current snapshot to be set on virtualmachine")
		})
	})
}
