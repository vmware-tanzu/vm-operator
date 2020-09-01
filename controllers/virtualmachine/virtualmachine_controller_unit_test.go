// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizer = "virtualmachine.vmoperator.vmware.com"
)

func unitTestsReconcile() {
	const (
		className      = "sample-class"
		vmName         = "sample-vm"
		nsName         = "sample-ns"
		controllerName = "virtualmachine"
		providerError  = "provider error"
	)
	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		reconciler *virtualmachine.VirtualMachineReconciler
		vmCtx      *vmopContext.VirtualMachineContext
		vm         *vmoperatorv1alpha1.VirtualMachine
		vmClass    *vmoperatorv1alpha1.VirtualMachineClass
	)
	BeforeEach(func() {
		vmClass = &vmoperatorv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: className,
			},
		}
		vm = &vmoperatorv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: nsName,
			},
			Spec: vmoperatorv1alpha1.VirtualMachineSpec{
				ClassName: className,
			},
		}
	})
	JustBeforeEach(func() {
		initObjects = []runtime.Object{vm, vmClass}
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = &virtualmachine.VirtualMachineReconciler{
			Client:     ctx.Client,
			Logger:     ctx.Logger.WithName("controllers").WithName(controllerName),
			Recorder:   ctx.Recorder,
			VmProvider: ctx.VmProvider,
		}

		vmCtx = &vmopContext.VirtualMachineContext{
			Context: ctx.Context,
			VM:      vm,
			VMObjectKey: client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      vm.Name,
			},
		}
	})
	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmCtx = nil
		reconciler = nil
	})
	Context("ReconcileNormal", func() {
		It("will have finalizer set after reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vm.ObjectMeta.Finalizers)).ToNot(BeZero())
			Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)
		})
		It("will emit corresponding event when provider fails to create VM", func() {
			// Simulate an error during VM create
			fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
			fakeProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(providerError))
			expectEvent(ctx, "CreateFailure")
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Creating)
		})
		It("will emit corresponding event when provider fails to update VM", func() {
			// The first reconcile creates the VM
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)

			// Simulate an error during the next reconcile (i.e., while trying to update the created VM)
			fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
			fakeProvider.UpdateVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}
			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			expectEvent(ctx, "UpdateFailure")
		})
	})
	Context("ReconcileDelete", func() {
		It("will delete the created VM and emit corresponding event", func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)

			// Delete the created VM
			err = reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
			vmExists, err := fakeProvider.DoesVirtualMachineExist(vmCtx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).ToNot(BeTrue())
			expectEvent(ctx, "DeleteSuccess")
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Deleted)
		})
		It("will emit corresponding event during delete failure", func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)

			// Simulate delete failure
			fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
			fakeProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err = reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())
			vmExists, err := fakeProvider.DoesVirtualMachineExist(vmCtx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeTrue())
			expectEvent(ctx, "DeleteFailure")
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Deleting)
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	event := <-ctx.Events
	eventComponents := strings.Split(event, " ")
	ExpectWithOffset(1, eventComponents[1]).To(Equal(eventStr))
}

func expectPhase(ctx *vmopContext.VirtualMachineContext, k8sClient client.Client, expectedPhase vmoperatorv1alpha1.VMStatusPhase) {
	vm := &vmoperatorv1alpha1.VirtualMachine{}
	err := k8sClient.Get(ctx, ctx.VMObjectKey, vm)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, vm.Status.Phase).To(Equal(expectedPhase))
}
