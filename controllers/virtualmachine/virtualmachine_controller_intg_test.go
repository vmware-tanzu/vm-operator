// Copyright (c) 2020-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.V1Alpha3,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	var (
		ctx *builder.IntegrationTestContext

		vm    *vmopv1.VirtualMachine
		vmKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				ClassName:  "dummy-class",
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	waitForVirtualMachineFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func(g Gomega) {
			vm := getVirtualMachine(ctx, objKey)
			g.Expect(vm).ToNot(BeNil())
			g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
		}).Should(Succeed(), "waiting for VirtualMachine finalizer")
	}

	Context("Reconcile", func() {
		const dummyInstanceUUID = "instanceUUID1234"
		const dummyIPAddress = "1.2.3.4"

		BeforeEach(func() {
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
				return nil
			}
			intgFakeVMProvider.Unlock()
		})

		AfterEach(func() {
			By("Delete VirtualMachine", func() {
				if err := ctx.Client.Delete(ctx, vm); err == nil {
					vm := &vmopv1.VirtualMachine{}
					// If VM is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
						vm.Finalizers = nil
						_ = ctx.Client.Update(ctx, vm)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			By("Set InstanceUUID in CreateOrUpdateVM", func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Just using InstanceUUID here for a field to update.
					vm.Status.InstanceUUID = dummyInstanceUUID
					return nil
				}
				intgFakeVMProvider.Unlock()
			})

			By("VirtualMachine should have InstanceUUID set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
				}, "4s").Should(Succeed(), "waiting for expected InstanceUUID")
			})

			By("Set Created condition in CreateOrUpdateVM", func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
					return nil
				}
				intgFakeVMProvider.Unlock()
			})

			By("VirtualMachine should have Created condition set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for Created condition")
			})

			By("Set IP address in CreateOrUpdateVM", func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
						PrimaryIP4: dummyIPAddress,
					}
					return nil
				}
				intgFakeVMProvider.Unlock()
			})

			By("VirtualMachine should have IP address set", func() {
				// This depends on PoweredOnVMHasIPRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Network).ToNot(BeNil())
					g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
				}, "4s").Should(Succeed(), "waiting for IP address")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				vm := getVirtualMachine(ctx, vmKey)
				Expect(vm).ToNot(BeNil())
				rv := vm.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, vm.GetGeneration())
				Consistently(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(fmt.Sprintf("%s :: %d", vm.GetResourceVersion(), vm.GetGeneration())).To(Equal(expected))
				}, "4s").Should(Succeed(), "no updates in steady state")
			})
		})

		When("VM schema needs upgrade", func() {
			instanceUUID := uuid.NewString()
			biosUUID := uuid.NewString()

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.InstanceUUID = instanceUUID
					vm.Status.BiosUUID = biosUUID
					return nil
				}
				intgFakeVMProvider.Unlock()
			})

			// NOTE: mutating webhook sets the default spec.instanceUUID, but is not run in this test -
			// leaving spec.instanceUUID empty as it would be for a pre-v1alpha3 VM
			It("will set spec.instanceUUID", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Spec.InstanceUUID
					}
					return ""
				}).Should(Equal(instanceUUID), "waiting for expected instanceUUID")
			})

			// NOTE: mutating webhook sets the default spec.biosUUID, but is not run in this test -
			// leaving spec.biosUUID empty as it would be for a pre-v1alpha3 VM
			It("will set spec.biosUUID", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Spec.BiosUUID
					}
					return ""
				}).Should(Equal(biosUUID), "waiting for expected biosUUID")
			})

			It("will set cloudInit.instanceID", func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Eventually(func() string {
					if vm = getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Spec.Bootstrap.CloudInit.InstanceID
					}
					return ""
				}).Should(Equal(string(vm.UID)), "waiting for expected instanceID")
			})
		})

		When("CreateOrUpdateVirtualMachine returns an error", func() {
			errMsg := "create error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.BiosUUID = "dummy-bios-uuid"
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Creating Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

			By("VirtualMachine should be deleted", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
				}).Should(Succeed())
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Deleting Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

				By("Finalizer should still be present", func() {
					vm := getVirtualMachine(ctx, vmKey)
					Expect(vm).ToNot(BeNil())
					Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
				})
			})
		})
	})
}
