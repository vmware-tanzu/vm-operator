// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	const (
		storageClassName = "foo-class"
	)

	var (
		ctx *builder.IntegrationTestContext

		vm                *vmopv1alpha1.VirtualMachine
		vmKey             types.NamespacedName
		vmClass           *vmopv1alpha1.VirtualMachineClass
		metadataConfigMap *corev1.ConfigMap
		storageClass      *storagev1.StorageClass
		resourceQuota     *corev1.ResourceQuota
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vmClass = &vmopv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "small",
			},
			Spec: vmopv1alpha1.VirtualMachineClassSpec{
				Hardware: vmopv1alpha1.VirtualMachineClassHardware{
					Cpus:   4,
					Memory: resource.MustParse("1Mi"),
				},
				Policies: vmopv1alpha1.VirtualMachineClassPolicies{
					Resources: vmopv1alpha1.VirtualMachineClassResources{
						Requests: vmopv1alpha1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("1000Mi"),
							Memory: resource.MustParse("100Mi"),
						},
						Limits: vmopv1alpha1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("2000Mi"),
							Memory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		}

		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: storageClassName,
			},
			Provisioner: "foo",
			Parameters: map[string]string{
				"storagePolicyID": "foo",
			},
		}

		metadataConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-metadata",
				Namespace: ctx.Namespace,
			},
			Data: map[string]string{
				"someKey": "someValue",
			},
		}

		resourceQuota = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-rq",
				Namespace: ctx.Namespace,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					storageClassName + ".storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("1"),
					"simple-class" + ".storageclass.storage.k8s.io/persistentvolumeclaims":   resource.MustParse("1"),
					"limits.cpu":    resource.MustParse("2"),
					"limits.memory": resource.MustParse("2Gi"),
				},
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ImageName:    "dummy-image",
				ClassName:    vmClass.Name,
				PowerState:   vmopv1alpha1.VirtualMachinePoweredOn,
				StorageClass: storageClass.Name,
				VmMetadata: &vmopv1alpha1.VirtualMachineMetadata{
					Transport:     vmopv1alpha1.VirtualMachineMetadataOvfEnvTransport,
					ConfigMapName: metadataConfigMap.Name,
				},
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVmProvider.Reset()
	})

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.VirtualMachine {
		vm := &vmopv1alpha1.VirtualMachine{}
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
		dummyBiosUUID := "biosUUID42"

		BeforeEach(func() {
			intgFakeVmProvider.Lock()
			intgFakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, _ vmprovider.VmConfigArgs) error {
				vm.Status.BiosUUID = dummyBiosUUID
				return nil
			}
			intgFakeVmProvider.Unlock()

			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
			Expect(ctx.Client.Create(ctx, metadataConfigMap)).To(Succeed())
		})

		AfterEach(func() {
			By("Delete VirtualMachine", func() {
				if err := ctx.Client.Delete(ctx, vm); err == nil {
					vm := &vmopv1alpha1.VirtualMachine{}
					// If VM is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
						vm.Finalizers = nil
						_ = ctx.Client.Update(ctx, vm)
					}
				} else {
					Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}
			})

			By("Delete cluster scoped resources", func() {
				err := ctx.Client.Delete(ctx, vmClass)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, storageClass)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			By("VirtualMachine should exist in Fake Provider", func() {
				Eventually(func() bool {
					exists, err := intgFakeVmProvider.DoesVirtualMachineExist(ctx, vm)
					if err != nil {
						return false
					}
					return exists
				}).Should(BeTrue())
			})

			By("VirtualMachine should reflect VMProvider updates", func() {
				Eventually(func() string {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.BiosUUID
					}
					return ""
				}).Should(Equal(dummyBiosUUID), "waiting for expected BiosUUID")

				Eventually(func() vmopv1alpha1.VMStatusPhase {
					if vm := getVirtualMachine(ctx, vmKey); vm != nil {
						return vm.Status.Phase
					}
					return ""
				}).Should(Equal(vmopv1alpha1.Created), "waiting for expected VM Phase")
			})
		})

		When("Provider CreateVM returns an error", func() {
			errMsg := "create error"

			BeforeEach(func() {
				intgFakeVmProvider.Lock()
				intgFakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
					return errors.New(errMsg)
				}
				intgFakeVmProvider.Unlock()
			})

			It("VirtualMachine is in Creating Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				By("Phase should be Creating", func() {
					Eventually(func() vmopv1alpha1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1alpha1.Creating))
				})
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			//vm.Finalizers = append(vm.Finalizers, "test-finalizer")
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

			/*
				By("VM should be in Deleted Phase", func() {
					vm := getVirtualMachine(ctx, vmKey)
					Expect(vm).ToNot(BeNil())
					Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Deleted))
				})
			*/

			By("VirtualMachine should not exist in Fake Provider", func() {
				exists, err := intgFakeVmProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"

			BeforeEach(func() {
				intgFakeVmProvider.Lock()
				intgFakeVmProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
					return errors.New(errMsg)
				}
				intgFakeVmProvider.Unlock()
			})

			It("VirtualMachine is in Deleting Phase", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())
				By("Phase should be Deleting", func() {
					Eventually(func() vmopv1alpha1.VMStatusPhase {
						if vm := getVirtualMachine(ctx, vmKey); vm != nil {
							return vm.Status.Phase
						}
						return ""
					}).Should(Equal(vmopv1alpha1.Deleting))
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
