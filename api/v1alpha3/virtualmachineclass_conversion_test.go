// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
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
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a3.VirtualMachineClass{})
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
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a3.VirtualMachineClass{})
		})
		t.Run("with displayName", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-vm-class",
					Namespace: "my-namespace",
				},
				Spec: vmopv1.VirtualMachineClassSpec{
					DisplayName: "small-class",
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachineClass{}, &vmopv1a3.VirtualMachineClass{})
		})
	})
}
