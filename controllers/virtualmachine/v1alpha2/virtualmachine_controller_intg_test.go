// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {

	const dummyInstanceUUID = "instanceUUID1234"

	var (
		ctx                  *builder.IntegrationTestContext
		storageClass         *storagev1.StorageClass
		resourceQuota        *corev1.ResourceQuota
		obj                  client.Object
		objKey               types.NamespacedName
		newObjFn             func() client.Object
		getInstanceUUIDFn    func(client.Object) string
		pauseAnnotationLabel string
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		objKey = types.NamespacedName{Name: "dummy-vm", Namespace: ctx.Namespace}

		// The validation webhook expects there to be a storage class associated
		// with the namespace where the VM is located.
		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dummy-storage-class-",
			},
			Provisioner: "dummy-provisioner",
		}
		Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
		resourceQuota = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dummy-resource-quota-",
				Namespace:    ctx.Namespace,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceName(storageClass.Name + ".storageclass.storage.k8s.io/dummy"): resourcev1.MustParse("0"),
				},
			},
		}
		Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())

		intgFakeVMProvider.Lock()
		intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1a2.VirtualMachine) error {
			// Used below just to check for something in the Status is updated.
			vm.Status.InstanceUUID = dummyInstanceUUID
			return nil
		}
		intgFakeVMProvider.Unlock()
	})

	AfterEach(func() {
		By("Delete VirtualMachine", func() {
			if err := ctx.Client.Delete(ctx, obj); err == nil {
				obj := newObjFn()
				// If VM is still around because of finalizer, try to cleanup for next test.
				if err := ctx.Client.Get(ctx, objKey, obj); err == nil && len(obj.GetFinalizers()) > 0 {
					obj.SetFinalizers(nil)
					_ = ctx.Client.Update(ctx, obj)
				}
			} else {
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}
		})

		Expect(ctx.Client.Delete(ctx, resourceQuota)).To(Succeed())
		resourceQuota = nil
		Expect(ctx.Client.Delete(ctx, storageClass)).To(Succeed())
		storageClass = nil

		obj = nil
		newObjFn = nil
		getInstanceUUIDFn = nil
		pauseAnnotationLabel = ""

		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getObject := func(
		ctx *builder.IntegrationTestContext,
		objKey types.NamespacedName,
		obj client.Object) client.Object {

		if err := ctx.Client.Get(ctx, objKey, obj); err != nil {
			return nil
		}
		return obj
	}

	waitForVirtualMachineFinalizer := func(
		ctx *builder.IntegrationTestContext,
		objKey types.NamespacedName,
		obj client.Object) {

		Eventually(func() []string {
			if obj := getObject(ctx, objKey, obj); obj != nil {
				return obj.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
	}

	reconcile := func() {
		When("the pause annotation is set", func() {
			It("Reconcile returns early and the finalizer never gets added", func() {
				obj.SetAnnotations(map[string]string{
					pauseAnnotationLabel: "",
				})
				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				Consistently(func() []string {
					if obj := getObject(ctx, objKey, newObjFn()); obj != nil {
						return obj.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer), "waiting for VirtualMachine finalizer")
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, objKey, newObjFn())
			})

			By("VirtualMachine should reflect VMProvider updates", func() {
				Eventually(func() string {
					if obj := getObject(ctx, objKey, newObjFn()); obj != nil {
						return getInstanceUUIDFn(obj)
					}
					return ""
				}).Should(Equal(dummyInstanceUUID), "waiting for expected InstanceUUID")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				obj := getObject(ctx, objKey, newObjFn())
				Expect(obj).ToNot(BeNil())
				rv := obj.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, obj.GetGeneration())
				// The resync period is 1 second, so balance between giving enough time vs a slow test.
				// Note: the kube-apiserver we test against (obtained from kubebuilder) is old and
				// appears to behavior differently than newer versions (like used in the SV) in that noop
				// Status subresource updates don't increment the ResourceVersion.
				Consistently(func() string {
					if obj := getObject(ctx, objKey, newObjFn()); obj != nil {
						return fmt.Sprintf("%s :: %d", obj.GetResourceVersion(), obj.GetGeneration())
					}
					return ""
				}, 4*time.Second).Should(Equal(expected))
			})
		})

		When("CreateOrUpdateVirtualMachine returns an error", func() {
			errMsg := "create error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1a2.VirtualMachine) error {
					vm.Status.BiosUUID = "dummy-bios-uuid"
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Creating Phase", func() {
				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, objKey, newObjFn())
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, objKey, newObjFn())
			Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if obj := getObject(ctx, objKey, newObjFn()); obj != nil {
						return obj.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer))
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1a2.VirtualMachine) error {
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine is in Deleting Phase", func() {
				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, objKey, newObjFn())
				Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
				By("Finalizer should still be present", func() {
					obj := getObject(ctx, objKey, newObjFn())
					Expect(obj).ToNot(BeNil())
					Expect(obj.GetFinalizers()).To(ContainElement(finalizer))
				})
			})
		})
	}

	Context("v1alpha2", func() {
		BeforeEach(func() {
			pauseAnnotationLabel = vmopv1a2.PauseAnnotation

			newObjFn = func() client.Object {
				return &vmopv1a2.VirtualMachine{}
			}

			getInstanceUUIDFn = func(obj client.Object) string {
				return obj.(*vmopv1a2.VirtualMachine).Status.InstanceUUID
			}

			obj = &vmopv1a2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: objKey.Namespace,
					Name:      objKey.Name,
				},
				Spec: vmopv1a2.VirtualMachineSpec{
					ImageName:    "dummy-image",
					ClassName:    "dummy-class",
					StorageClass: storageClass.Name,
					PowerState:   vmopv1a2.VirtualMachinePowerStateOn,
				},
			}
		})

		Context("Reconcile", reconcile)
	})

	Context("v1alpha1", func() {
		BeforeEach(func() {
			pauseAnnotationLabel = vmopv1.PauseAnnotation

			newObjFn = func() client.Object {
				return &vmopv1.VirtualMachine{}
			}

			getInstanceUUIDFn = func(obj client.Object) string {
				return obj.(*vmopv1.VirtualMachine).Status.InstanceUUID
			}

			obj = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: objKey.Namespace,
					Name:      objKey.Name,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    "dummy-image",
					ClassName:    "dummy-class",
					StorageClass: storageClass.Name,
					PowerState:   vmopv1.VirtualMachinePoweredOn,
				},
			}
		})

		Context("Reconcile", reconcile)
	})
}
