// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {

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
				ImageName:    "dummy-image",
				ClassName:    "dummy-class",
				PowerState:   vmopv1.VirtualMachinePoweredOn,
				StorageClass: "dummy-storageclass",
				VmMetadata: &vmopv1.VirtualMachineMetadata{
					Transport:     vmopv1.VirtualMachineMetadataOvfEnvTransport,
					ConfigMapName: "dummy-configmap",
				},
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
		Eventually(func() []string {
			if vm := getVirtualMachine(ctx, objKey); vm != nil {
				return vm.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
	}

	Context("Reconcile", func() {
		dummyInstanceUUID := "instanceUUID1234"

		BeforeEach(func() {
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
				// Used below just to check for something in the Status is updated.
				vm.Status.InstanceUUID = dummyInstanceUUID
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
					Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}
			})
		})

		When("the pause annotation is set", func() {
			It("Reconcile returns early and the finalizer never gets added", func() {
				// Set the Pause annotation on the VM
				vm.Annotations = map[string]string{
					vmopv1.PauseAnnotation: "",
				}

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Consistently(func() []string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			By("VirtualMachine should reflect VMProvider updates", func() {
				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.InstanceUUID
					}
					return ""
				}).Should(Equal(dummyInstanceUUID), "waiting for expected InstanceUUID")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				vm := getVirtualMachine(ctx, vmKey)
				Expect(vm).ToNot(BeNil())
				rv := vm.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, vm.GetGeneration())
				// The resync period is 1 second, so balance between giving enough time vs a slow test.
				// Note: the kube-apiserver we test against (obtained from kubebuilder) is old and
				// appears to behavior differently than newer versions (like used in the SV) in that noop
				// Status subresource updates don't increment the ResourceVersion.
				Consistently(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return fmt.Sprintf("%s :: %d", vm.GetResourceVersion(), vm.GetGeneration())
					}
					return ""
				}, 4*time.Second).Should(Equal(expected))
			})
		})

		When("CreateOrUpdateVirtualMachine returns an error", func() {
			errMsg := "create error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.Phase = vmopv1.Creating
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Creating Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				By("Phase should be Creating", func() {
					Eventually(func() vmopv1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1.Creating))
				})
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer))
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
				By("Phase should be Deleting", func() {
					Eventually(func() vmopv1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1.Deleting))
				})

				By("Finalizer should still be present", func() {
					vm := getVirtualMachine(ctx, vmKey)
					Expect(vm).ToNot(BeNil())
					Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
				})
			})
		})
	})
}
