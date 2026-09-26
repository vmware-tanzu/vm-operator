// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	. "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha6"
)

var _ = Describe("VirtualMachineClass conversion",
	Label(testlabels.API, testlabels.Webhook),
	func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1a1.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1a2.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1a3.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1a4.AddToScheme(scheme)).To(Succeed())
			Expect(vmopv1a5.AddToScheme(scheme)).To(Succeed())
		})

		Describe("v1alpha2 spoke", func() {
			It("round-trips hub→spoke→hub without loss", func() {
				converter, err := VirtualMachineClass(scheme)
				Expect(err).NotTo(HaveOccurred())

				hub := &vmopv1.VirtualMachineClass{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "vmoperator.vmware.com/v1alpha6",
						Kind:       "VirtualMachineClass",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-class",
					},
					Spec: vmopv1.VirtualMachineClassSpec{
						Hardware: vmopv1.VirtualMachineClassHardware{
							Cpus:   4,
							Memory: resource.MustParse("8Gi"),
						},
					},
				}

				spoke := &vmopv1a2.VirtualMachineClass{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "vmoperator.vmware.com/v1alpha2",
						Kind:       "VirtualMachineClass",
					},
				}

				Expect(converter.ConvertObject(ctx, hub, spoke)).To(Succeed())
				Expect(spoke.Name).To(Equal(hub.Name))
				Expect(spoke.Spec.Hardware.Cpus).To(Equal(int64(4)))

				hub2 := &vmopv1.VirtualMachineClass{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "vmoperator.vmware.com/v1alpha6",
						Kind:       "VirtualMachineClass",
					},
				}
				Expect(converter.ConvertObject(ctx, spoke, hub2)).To(Succeed())
				Expect(hub2.Name).To(Equal(hub.Name))
				Expect(hub2.Spec.Hardware.Cpus).To(Equal(hub.Spec.Hardware.Cpus))
			})
		})
	})
