// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("RemoveStaleVMOwnerRefFromPVCs", func() {

	const namespace = "my-namespace"

	var (
		ctx         context.Context
		k8sClient   ctrlclient.Client
		vm          *vmopv1.VirtualMachine
		initObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		reterr      error
	)

	ownerRefFor := func(vm *vmopv1.VirtualMachine) metav1.OwnerReference {
		return metav1.OwnerReference{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       "VirtualMachine",
			Name:       vm.Name,
			UID:        vm.UID,
		}
	}

	getPVC := func(name string) *corev1.PersistentVolumeClaim {
		pvc := &corev1.PersistentVolumeClaim{}
		ExpectWithOffset(1, k8sClient.Get(
			ctx,
			ctrlclient.ObjectKey{Namespace: namespace, Name: name},
			pvc)).To(Succeed())
		return pvc
	}

	BeforeEach(func() {
		ctx = context.Background()
		withFuncs = interceptor.Funcs{}
		initObjects = nil

		vm = builder.DummyBasicVirtualMachine("my-vm", namespace)
		vm.UID = "my-vm-uid"
		vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
			builder.DummyPVCVolume("disk-1", "attached-pvc"),
		}
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().
			WithScheme(builder.NewScheme()).
			WithObjects(initObjects...).
			WithInterceptorFuncs(withFuncs).
			WithIndex(
				&corev1.PersistentVolumeClaim{},
				kubeutil.PVCOwnerRefVMUIDIndexKey,
				kubeutil.PVCOwnerRefVMUIDIndexerFunc).
			Build()

		reterr = kubeutil.RemoveStaleVMOwnerRefFromPVCs(ctx, k8sClient, vm)
	})

	When("a PVC is still attached to the VM", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "attached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
			}
		})

		It("leaves the OwnerRef intact", func() {
			Expect(reterr).ToNot(HaveOccurred())
			pvc := getPVC("attached-pvc")
			Expect(pvc.OwnerReferences).To(ConsistOf(ownerRefFor(vm)))
		})
	})

	When("a PVC has been detached from the VM", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
			}
		})

		It("removes the VM's OwnerRef", func() {
			Expect(reterr).ToNot(HaveOccurred())
			pvc := getPVC("detached-pvc")
			Expect(pvc.OwnerReferences).To(BeEmpty())
		})
	})

	When("a detached PVC carries the keep-owner-ref annotation", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
						Annotations: map[string]string{
							pkgconst.KeepOwnerRefAnnotationKey: "",
						},
					},
				},
			}
		})

		It("leaves the OwnerRef intact", func() {
			Expect(reterr).ToNot(HaveOccurred())
			pvc := getPVC("detached-pvc")
			Expect(pvc.OwnerReferences).To(ConsistOf(ownerRefFor(vm)))
		})
	})

	When("a detached PVC carries the keep-owner-ref annotation with a non-empty value", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
						Annotations: map[string]string{
							pkgconst.KeepOwnerRefAnnotationKey: "anything",
						},
					},
				},
			}
		})

		It("still leaves the OwnerRef intact, since only the key's presence is checked", func() {
			Expect(reterr).ToNot(HaveOccurred())
			pvc := getPVC("detached-pvc")
			Expect(pvc.OwnerReferences).To(ConsistOf(ownerRefFor(vm)))
		})
	})

	When("a detached PVC has an additional, unrelated OwnerRef", func() {
		var otherRef metav1.OwnerReference

		BeforeEach(func() {
			otherRef = metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "some-other-owner",
				UID:        "some-other-uid",
			}
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "detached-pvc-multi-owner",
						Namespace: namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFor(vm),
							otherRef,
						},
					},
				},
			}
		})

		It("removes only the VM's OwnerRef entry", func() {
			Expect(reterr).ToNot(HaveOccurred())
			pvc := getPVC("detached-pvc-multi-owner")
			Expect(pvc.OwnerReferences).To(ConsistOf(otherRef))
		})
	})

	When("no PVCs are owned by the VM", func() {
		BeforeEach(func() {
			initObjects = nil
		})

		It("returns without error", func() {
			Expect(reterr).ToNot(HaveOccurred())
		})
	})

	When("a detached PVC is deleted between listing and patching", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
			}
			withFuncs.Get = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if key.Name == "detached-pvc" {
					return apierrors.NewNotFound(
						corev1.Resource("persistentvolumeclaims"), key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			}
		})

		It("does not fail", func() {
			Expect(reterr).ToNot(HaveOccurred())
		})
	})

	When("the patch conflicts once and then succeeds on retry", func() {
		var attempts int

		BeforeEach(func() {
			attempts = 0
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
			}
			withFuncs.Patch = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				attempts++
				if attempts == 1 {
					return apierrors.NewConflict(
						corev1.Resource("persistentvolumeclaims"), obj.GetName(), errors.New("conflict"))
				}
				return c.Patch(ctx, obj, patch, opts...)
			}
		})

		It("retries and succeeds", func() {
			Expect(reterr).ToNot(HaveOccurred())
			Expect(attempts).To(BeNumerically(">=", 2))
			pvc := getPVC("detached-pvc")
			Expect(pvc.OwnerReferences).To(BeEmpty())
		})
	})

	When("removing the OwnerRef from one of several detached PVCs fails permanently", func() {
		BeforeEach(func() {
			initObjects = []ctrlclient.Object{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc-1",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "detached-pvc-2",
						Namespace:       namespace,
						OwnerReferences: []metav1.OwnerReference{ownerRefFor(vm)},
					},
				},
			}
			withFuncs.Patch = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				if obj.GetName() == "detached-pvc-1" {
					return apierrors.NewInternalError(errors.New("boom"))
				}
				return c.Patch(ctx, obj, patch, opts...)
			}
		})

		It("returns an error but still processes the other PVC", func() {
			Expect(reterr).To(HaveOccurred())
			Expect(reterr.Error()).To(ContainSubstring("detached-pvc-1"))

			pvc2 := getPVC("detached-pvc-2")
			Expect(pvc2.OwnerReferences).To(BeEmpty())
		})
	})
})
