// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"math"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func TestVirtualMachineClassConversion(t *testing.T) {

	hubSpokeHub := func(g *WithT, hub, hubAfter ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
	}

	t.Run("VirtualMachineClass hub-spoke-hub", func(t *testing.T) {

		t.Run("empty class", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a1.VirtualMachineClass{})
		})
		t.Run("empty class w some annotations", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
					Annotations: map[string]string{
						"fizz": "buzz",
					},
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a1.VirtualMachineClass{})
		})
		t.Run("class w some annotations and reserved profile ID and slots", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
					Annotations: map[string]string{
						"fizz": "buzz",
					},
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					ReservedProfileID: "my-profile-id",
					ReservedSlots:     4,
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a1.VirtualMachineClass{})
		})
		t.Run("class w reserved profile ID and slots", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					ReservedProfileID: "my-profile-id",
					ReservedSlots:     4,
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a1.VirtualMachineClass{})
		})

		t.Run("class w negative reserved slots", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					ReservedSlots: -1,
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a1.VirtualMachineClass{})
		})
	})

	t.Run("VirtualMachineClass spoke-hub", func(t *testing.T) {
		t.Run("class w reserved slots gt math.MaxInt32", func(t *testing.T) {
			spoke := vmopv1a1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"vmoperator.vmware.com/reserved-slots": strconv.Itoa(math.MaxInt32 + 2),
					},
				},
			}

			expectedHub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					ReservedSlots: 0,
				},
			}

			g := NewWithT(t)
			var hub vmopv1.VirtualMachineClass
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(apiequality.Semantic.DeepEqual(hub, expectedHub)).To(BeTrue(), cmp.Diff(hub, expectedHub))
		})

		t.Run("class w reserved slots lt -math.MaxInt32", func(t *testing.T) {
			spoke := vmopv1a1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"vmoperator.vmware.com/reserved-slots": strconv.Itoa(-math.MaxInt32 - 2),
					},
				},
			}

			expectedHub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					ReservedSlots: 0,
				},
			}

			g := NewWithT(t)
			var hub vmopv1.VirtualMachineClass
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(apiequality.Semantic.DeepEqual(hub, expectedHub)).To(BeTrue(), cmp.Diff(hub, expectedHub))
		})
	})
}
