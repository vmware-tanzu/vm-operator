// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachinegroup"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizer               = "vmoperator.vmware.com/virtualmachinegroup"
	testNS                  = "test-ns"
	virtualMachineKind      = "VirtualMachine"
	virtualMachineGroupKind = "VirtualMachineGroup"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController
		reconciler  *virtualmachinegroup.Reconciler

		vmGroup1, vmGroup2       *vmopv1.VirtualMachineGroup
		vmGroup1Key, vmGroup2Key types.NamespacedName
		vm1, vm2                 *vmopv1.VirtualMachine
		vm1Key, vm2Key           types.NamespacedName
	)

	BeforeEach(func() {
		vmGroup1Key = types.NamespacedName{
			Name:      "vmgroup1",
			Namespace: testNS,
		}
		vmGroup2Key = types.NamespacedName{
			Name:      "vmgroup2",
			Namespace: testNS,
		}
		vm1Key = types.NamespacedName{
			Name:      "vm1",
			Namespace: testNS,
		}
		vm2Key = types.NamespacedName{
			Name:      "vm2",
			Namespace: testNS,
		}

		vmGroup1 = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmGroup1Key.Name,
				Namespace:   vmGroup1Key.Namespace,
				Annotations: map[string]string{},
				Finalizers:  []string{finalizer},
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				Members: []vmopv1.GroupMember{
					{
						Kind: virtualMachineKind,
						Name: vm1Key.Name,
					},
					{
						Kind: virtualMachineKind,
						Name: vm2Key.Name,
					},
					{
						Kind: virtualMachineGroupKind,
						Name: vmGroup2Key.Name,
					},
				},
			},
		}
		vmGroup2 = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmGroup2Key.Name,
				Namespace:   vmGroup2Key.Namespace,
				Annotations: map[string]string{},
				Finalizers:  []string{finalizer},
			},
		}

		vm1 = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vm1Key.Name,
				Namespace:   vm1Key.Namespace,
				Annotations: map[string]string{},
			},
		}
		vm2 = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vm2Key.Name,
				Namespace:   vm2Key.Namespace,
				Annotations: map[string]string{},
			},
		}
	})

	JustBeforeEach(func() {
		initObjects = append(initObjects, vmGroup1, vmGroup2, vm1, vm2)
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachinegroup.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: vmGroup1Key})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
	})

	Context("ReconcileNormal", func() {
		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vmGroup1.Finalizers = nil
			})

			It("will set finalizer", func() {
				updatedVMGroup1 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup1Key, updatedVMGroup1)).To(Succeed())
				Expect(updatedVMGroup1.GetFinalizers()).To(ContainElement(finalizer))
			})
		})

		Context("OwnerReferences", func() {
			It("should set owner references on group members", func() {
				vm1 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
				Expect(vm1.OwnerReferences).To(HaveLen(1))
				Expect(vm1.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

				vm2 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
				Expect(vm2.OwnerReferences).To(HaveLen(1))
				Expect(vm2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

				vmGroup2 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
				Expect(vmGroup2.OwnerReferences).To(HaveLen(1))
				Expect(vmGroup2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))
			})
		})
	})

	Context("ReconcileDelete", func() {
		BeforeEach(func() {
			now := metav1.Now()
			vmGroup1.DeletionTimestamp = &now
		})

		// Please note it is not possible to validate garbage collection with
		// the fake client or with envtest, because neither of them implement
		// the Kubernetes garbage collector.
		It("will remove finalizer and delete the object", func() {
			vmGroup1 := &vmopv1.VirtualMachineGroup{}
			err := ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
}
