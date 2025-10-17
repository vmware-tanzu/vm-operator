// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("AllDisksArePVCs", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		vm        *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = context.Background()
		k8sClient = builder.NewFakeClient()
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test-namespace",
				UID:       types.UID("test-uid"),
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}
		Expect(k8sClient.Create(ctx, vm)).To(Succeed())
	})

	Context("CnsRegisterVolumeToVirtualMachineMapper", func() {
		var mapperFunc func(context.Context, client.Object) []reconcile.Request

		BeforeEach(func() {
			mapperFunc = vmopv1util.CnsRegisterVolumeToVirtualMachineMapper(ctx, k8sClient)
		})

		It("should panic with nil context", func() {
			Expect(func() {
				vmopv1util.CnsRegisterVolumeToVirtualMachineMapper(nil, k8sClient)
			}).To(Panic())
		})

		It("should panic with nil client", func() {
			Expect(func() {
				vmopv1util.CnsRegisterVolumeToVirtualMachineMapper(ctx, nil)
			}).To(Panic())
		})

		Context("when CnsRegisterVolume has owner reference to VM", func() {
			var crv *cnsv1alpha1.CnsRegisterVolume

			BeforeEach(func() {
				crv = &cnsv1alpha1.CnsRegisterVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crv",
						Namespace: vm.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: vmopv1.GroupVersion.String(),
								Kind:       "VirtualMachine",
								Name:       vm.Name,
								UID:        vm.UID,
							},
						},
					},
					Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
						PvcName: "test-pvc",
					},
				}
			})

			It("should return reconcile request for the owner VM", func() {
				requests := mapperFunc(ctx, crv)
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].NamespacedName.Name).To(Equal(vm.Name))
				Expect(requests[0].NamespacedName.Namespace).To(Equal(vm.Namespace))
			})
		})

		Context("when CnsRegisterVolume has label pointing to VM", func() {
			var crv *cnsv1alpha1.CnsRegisterVolume

			BeforeEach(func() {
				crv = &cnsv1alpha1.CnsRegisterVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crv",
						Namespace: vm.Namespace,
						Labels: map[string]string{
							"vmoperator.vmware.com/created-by": vm.Name,
						},
					},
					Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
						PvcName: "test-pvc",
					},
				}
			})

			It("should return reconcile request for the labeled VM", func() {
				requests := mapperFunc(ctx, crv)
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].NamespacedName.Name).To(Equal(vm.Name))
				Expect(requests[0].NamespacedName.Namespace).To(Equal(vm.Namespace))
			})
		})

		Context("when CnsRegisterVolume has both owner reference and label", func() {
			var crv *cnsv1alpha1.CnsRegisterVolume

			BeforeEach(func() {
				crv = &cnsv1alpha1.CnsRegisterVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crv",
						Namespace: vm.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: vmopv1.GroupVersion.String(),
								Kind:       "VirtualMachine",
								Name:       vm.Name,
								UID:        vm.UID,
							},
						},
						Labels: map[string]string{
							"vmoperator.vmware.com/created-by": vm.Name,
						},
					},
					Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
						PvcName: "test-pvc",
					},
				}
			})

			It("should return only one reconcile request (no duplicates)", func() {
				requests := mapperFunc(ctx, crv)
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].NamespacedName.Name).To(Equal(vm.Name))
				Expect(requests[0].NamespacedName.Namespace).To(Equal(vm.Namespace))
			})
		})

		Context("when CnsRegisterVolume has label pointing to non-existent VM", func() {
			var crv *cnsv1alpha1.CnsRegisterVolume

			BeforeEach(func() {
				crv = &cnsv1alpha1.CnsRegisterVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crv",
						Namespace: vm.Namespace,
						Labels: map[string]string{
							"vmoperator.vmware.com/created-by": "non-existent-vm",
						},
					},
					Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
						PvcName: "test-pvc",
					},
				}
			})

			It("should return no reconcile requests", func() {
				requests := mapperFunc(ctx, crv)
				Expect(requests).To(HaveLen(0))
			})
		})

		Context("when CnsRegisterVolume has no VM association", func() {
			var crv *cnsv1alpha1.CnsRegisterVolume

			BeforeEach(func() {
				crv = &cnsv1alpha1.CnsRegisterVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crv",
						Namespace: vm.Namespace,
					},
					Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
						PvcName: "test-pvc",
					},
				}
			})

			It("should return no reconcile requests", func() {
				requests := mapperFunc(ctx, crv)
				Expect(requests).To(HaveLen(0))
			})
		})

		Context("when mapper function is called with nil context", func() {
			It("should panic", func() {
				Expect(func() {
					mapperFunc(nil, &cnsv1alpha1.CnsRegisterVolume{})
				}).To(Panic())
			})
		})

		Context("when mapper function is called with nil object", func() {
			It("should panic", func() {
				Expect(func() {
					mapperFunc(ctx, nil)
				}).To(Panic())
			})
		})
	})
})
