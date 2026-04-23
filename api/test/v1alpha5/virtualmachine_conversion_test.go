// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/ptr"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
)

func TestVirtualMachineConversion(t *testing.T) {

	t.Run("hub-spoke-hub", func(t *testing.T) {
		testCases := []struct {
			name string
			hub  ctrlconversion.Hub
		}{
			{
				name: "spec.bootstrap.disabled",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							Disabled: true,
						},
					},
				},
			},
			{
				name: "spec.bootstrap.disabled=false",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							Disabled: false,
						},
					},
				},
			},
			{
				name: "spec.bootstrap=nil",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: nil,
					},
				},
			},
			{
				name: "spec.volumeAttributesClassName=my-class",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						VolumeAttributesClassName: "my-class",
					},
				},
			},
			{
				name: "spec.network.vlans",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
								{
									Name: "eth1",
								},
							},
							VLANs: []vmopv1.VirtualMachineNetworkVLANSpec{
								{
									Name: "vlan100",
									ID:   100,
									Link: "eth1",
								},
								{
									Name: "vlan200",
									ID:   200,
									Link: "eth1",
								},
							},
						},
					},
				},
			},
			{
				name: "spec.network.interfaces.requestedAddressFamilyMode",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:                       "eth0",
									RequestedAddressFamilyMode: ptr.To(vmopv1.NetworkInterfaceIPFamilyPolicyDualStack),
								},
								{
									Name:                       "eth1",
									RequestedAddressFamilyMode: ptr.To(vmopv1.NetworkInterfaceIPFamilyPolicyIPv6Only),
								},
							},
						},
					},
				},
			},
			{
				name: "spec.advanced new fields",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Advanced: &vmopv1.VirtualMachineAdvancedSpec{
							PreferHTEnabled:                    ptr.To(true),
							HugePages1GEnabled:                 ptr.To(true),
							TimeTrackerLowLatencyEnabled:       ptr.To(true),
							CPUAffinityExclusiveNoStatsEnabled: ptr.To(true),
							VMXSwapEnabled:                     ptr.To(false),
							PNUMANodeAffinity:                  []int32{0, 1},
							ExtraConfig: []vmopv1common.KeyValuePair{
								{Key: "somekey", Value: "somevalue"},
							},
						},
					},
				},
			},
			{
				name: "spec.network.interfaces advanced nic fields",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:                       "eth0",
									RequestedAddressFamilyMode: ptr.To(vmopv1.NetworkInterfaceIPFamilyPolicyDualStack),
									Type:                       vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
									VNUMANodeID:    ptr.To(int32(1)),
									VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
										UPTv2Enabled: ptr.To(true),
									},
									AdvancedProperties: []vmopv1common.KeyValuePair{
										{Key: "test-key", Value: "test-val"},
									},
								},
							},
						},
					},
				},
			},
		}

		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)

				after := &vmopv1.VirtualMachine{}
				spoke := &vmopv1a5.VirtualMachine{}

				// First convert hub to spoke
				g.Expect(spoke.ConvertFrom(tc.hub)).To(Succeed())

				// Convert spoke back to hub.
				g.Expect(spoke.ConvertTo(after)).To(Succeed())

				// Check that everything is equal.
				g.Expect(apiequality.Semantic.DeepEqual(tc.hub, after)).To(BeTrue(), cmp.Diff(tc.hub, after))
			})
		}
	})
}
