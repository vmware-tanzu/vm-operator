// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

const finalizer = "virtualmachine.vmoperator.vmware.com"

func unitTestsReconcile() {
	const (
		providerError = "provider error"
	)

	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *virtualmachine.VirtualMachineReconciler
		fakeVmProvider *providerfake.FakeVmProvider
		vmCtx          *vmopContext.VirtualMachineContext
		vm             *vmoperatorv1alpha1.VirtualMachine
		vmClass        *vmoperatorv1alpha1.VirtualMachineClass
		vmClassBinding *vmoperatorv1alpha1.VirtualMachineClassBinding
	)

	BeforeEach(func() {
		vmClass = &vmoperatorv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}

		vm = &vmoperatorv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmoperatorv1alpha1.VirtualMachineSpec{
				ClassName: vmClass.Name,
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachine.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VmProvider,
		)
		fakeVmProvider = ctx.VmProvider.(*providerfake.FakeVmProvider)

		vmCtx = &vmopContext.VirtualMachineContext{
			Context: ctx,
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

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass)
		})

		When("the WCP_VMService FSS is enabled", func() {
			var oldVMServiceEnableFunc func() bool

			BeforeEach(func() {
				oldVMServiceEnableFunc = lib.IsVMServiceFSSEnabled
				lib.IsVMServiceFSSEnabled = func() bool {
					return true
				}
			})

			AfterEach(func() {
				lib.IsVMServiceFSSEnabled = oldVMServiceEnableFunc
			})

			Context("VirtualMachineClassBinding does not exist for the class", func() {
				It("should fail to reconcile the VM with the appropriate error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VM class binding does not exist for VM class: %v", vm.Spec.ClassName)))
				})
			})

			Context("VirtualMachineClassBinding exists for the class", func() {
				BeforeEach(func() {
					vmClassBinding = &vmoperatorv1alpha1.VirtualMachineClassBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-classbinding",
							Namespace: vm.Namespace,
						},
						ClassRef: vmoperatorv1alpha1.ClassReference{
							APIVersion: vmoperatorv1alpha1.SchemeGroupVersion.Group,
							Name:       vm.Spec.ClassName,
							Kind:       reflect.TypeOf(vmClass).Elem().Name(),
						},
					}
					initObjects = append(initObjects, vmClassBinding)
				})

				It("should successfully reconcile the VM, add finalizer and verify the phase", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
					expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)
				})
			})
		})

		It("will have finalizer set after reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)
		})

		It("will emit corresponding event when provider fails to create VM", func() {
			// Simulate an error during VM create
			fakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
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
			fakeVmProvider.UpdateVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}
			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			expectEvent(ctx, "UpdateFailure")
		})
	})

	Context("ReconcileDelete", func() {

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass)
		})

		JustBeforeEach(func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Created)
		})

		It("will delete the created VM and emit corresponding event", func() {
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeFalse())

			expectEvent(ctx, "DeleteSuccess")
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Deleted)
		})

		It("will emit corresponding event during delete failure", func() {
			// Simulate delete failure
			fakeVmProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeTrue())

			expectEvent(ctx, "DeleteFailure")
			expectPhase(vmCtx, ctx.Client, vmoperatorv1alpha1.Deleting)
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	var event string
	Eventually(ctx.Events).Should(Receive(&event))
	eventComponents := strings.Split(event, " ")
	ExpectWithOffset(1, eventComponents[1]).To(Equal(eventStr))
}

func expectPhase(ctx *vmopContext.VirtualMachineContext, k8sClient client.Client, expectedPhase vmoperatorv1alpha1.VMStatusPhase) {
	vm := &vmoperatorv1alpha1.VirtualMachine{}
	err := k8sClient.Get(ctx, ctx.VMObjectKey, vm)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, vm.Status.Phase).To(Equal(expectedPhase))
}
