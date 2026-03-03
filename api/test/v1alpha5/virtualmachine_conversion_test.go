// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineConversion(t *testing.T) {

	t.Run("VirtualMachine network VLANs", func(t *testing.T) {
		hubSpokeHub := func(g *WithT, hub, hubAfter ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
			hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

			// First convert hub to spoke
			dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
			g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

			// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
			g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

			g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
		}

		t.Run("VirtualMachine hub-spoke-hub with VLANs preserves VLANs", func(t *testing.T) {
			g := NewWithT(t)

			hub := vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-with-vlans",
					Namespace: "default",
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName: "my-name",
					ClassName: "my-class",
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{Name: "eth0"},
							{Name: "eth1"},
						},
						VLANs: map[string]vmopv1.VirtualMachineNetworkVLANSpec{
							"vlan100": {
								ID:   100,
								Link: "eth1",
							},
							"vlan200": {
								ID:   200,
								Link: "eth1",
							},
						},
					},
				},
			}

			hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a5.VirtualMachine{})
		})

		t.Run("VirtualMachine hub-spoke-hub with VLANs verifies round-trip", func(t *testing.T) {
			g := NewWithT(t)

			hubBefore := vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-with-vlans",
					Namespace: "default",
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName: "my-name",
					ClassName: "my-class",
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{Name: "eth0"},
							{Name: "eth1"},
						},
						VLANs: map[string]vmopv1.VirtualMachineNetworkVLANSpec{
							"vlan100": {
								ID:   100,
								Link: "eth1",
							},
							"vlan200": {
								ID:   200,
								Link: "eth1",
							},
						},
					},
				},
			}

			// Convert hub -> spoke
			var spoke vmopv1a5.VirtualMachine
			g.Expect(spoke.ConvertFrom(&hubBefore)).To(Succeed())

			// Verify spoke does not have VLANs field (it was removed from v1alpha5)
			g.Expect(spoke.Spec.Network).ToNot(BeNil())
			g.Expect(spoke.Spec.Network.Interfaces).To(HaveLen(2))

			// Convert spoke -> hub
			var hubAfter vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hubAfter)).To(Succeed())

			// Verify VLANs are preserved in hub. Unlike v1alpha2/3/4 which rely on
			// annotation-based restore, v1alpha5 preserves VLANs implicitly via the
			// unsafe.Pointer cast used in the generated conversion code, which shares
			// the same memory between v1alpha5 and v1alpha6 VirtualMachineNetworkSpec.
			g.Expect(hubAfter.Spec.Network).ToNot(BeNil())
			g.Expect(hubAfter.Spec.Network.VLANs).To(HaveLen(2))
			g.Expect(hubAfter.Spec.Network.VLANs).To(HaveKey("vlan100"))
			g.Expect(hubAfter.Spec.Network.VLANs).To(HaveKey("vlan200"))

			g.Expect(hubAfter.Spec.Network.VLANs["vlan100"].ID).To(Equal(int64(100)))
			g.Expect(hubAfter.Spec.Network.VLANs["vlan100"].Link).To(Equal("eth1"))
			g.Expect(hubAfter.Spec.Network.VLANs["vlan200"].ID).To(Equal(int64(200)))
			g.Expect(hubAfter.Spec.Network.VLANs["vlan200"].Link).To(Equal("eth1"))

			// Verify full round-trip equality
			g.Expect(apiequality.Semantic.DeepEqual(hubBefore.Spec.Network.VLANs, hubAfter.Spec.Network.VLANs)).To(BeTrue(),
				cmp.Diff(hubBefore.Spec.Network.VLANs, hubAfter.Spec.Network.VLANs))
		})
	})
}
