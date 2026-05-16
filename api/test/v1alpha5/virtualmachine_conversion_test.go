// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha6/sysprep"
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
				name: "spec.bootstrap.sysprep.sysprep",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &vmopv1sysprep.Sysprep{
									ExpirePasswordAfterNextLogin: true,
									ScriptText: &vmopv1common.ValueOrSecretKeySelector{
										From: &vmopv1common.SecretKeySelector{
											Name: "sc-name",
											Key:  "sc-key",
										},
										Value: ptr.To("sc-value"),
									},
								},
							},
						},
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
				name: "spec.network.interfaces.ipamModes",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									IPAMModes: []corev1.IPFamily{
										corev1.IPv4Protocol,
										corev1.IPv6Protocol,
									},
								},
								{
									Name:      "eth1",
									IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
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
									Name: "eth0",
									IPAMModes: []corev1.IPFamily{
										corev1.IPv4Protocol,
										corev1.IPv6Protocol,
									},
									Type:        vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
									VNUMANodeID: ptr.To(int32(1)),
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
			{
				name: "spec.network.interfaces legacy NIC type",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Type: vmopv1.VirtualMachineNetworkInterfaceTypeE1000e,
								},
							},
						},
					},
				},
			},
			{
				name: "spec.resources.size",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Resources: &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("4"),
								Memory: resource.MustParse("8Gi"),
							},
						},
					},
				},
			},
			{
				name: "spec.resources.requests",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Resources: &vmopv1.VirtualMachineResourcesSpec{
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("2000"),
								Memory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
			{
				name: "spec.resources.limits",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Resources: &vmopv1.VirtualMachineResourcesSpec{
							Limits: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("4000"),
								Memory: resource.MustParse("8Gi"),
							},
						},
					},
				},
			},
			{
				name: "spec.resources size + requests + limits",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Resources: &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("8"),
								Memory: resource.MustParse("16Gi"),
							},
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("2000"),
								Memory: resource.MustParse("8Gi"),
							},
							Limits: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("4000"),
								Memory: resource.MustParse("16Gi"),
							},
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced.latencySensitivity=High",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced.latencySensitivity=HighWithHyperthreading",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading),
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced.topology",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket:         ptr.To(int32(4)),
								CoresPerNUMANode:       ptr.To(int32(8)),
								ExposeVNUMAOnCPUHotAdd: ptr.To(true),
							},
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced.nestedHardwareVirtualizationEnabled",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							NestedHardwareVirtualizationEnabled: ptr.To(true),
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced.performanceCountersEnabled",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							PerformanceCountersEnabled: ptr.To(true),
						},
					},
				},
			},
			{
				name: "spec.cpuAdvanced full",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket: ptr.To(int32(2)),
							},
							HotAddEnabled:                       ptr.To(false),
							IOMMUEnabled:                        ptr.To(true),
							NestedHardwareVirtualizationEnabled: ptr.To(true),
							PerformanceCountersEnabled:          ptr.To(true),
						},
					},
				},
			},
			{
				name: "spec.memoryAdvanced.reservationLockedToMax=true",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						MemoryAdvanced: &vmopv1.VirtualMachineMemoryAdvancedSpec{
							ReservationLockedToMax: ptr.To(true),
						},
					},
				},
			},
			{
				name: "spec.memoryAdvanced hotAddEnabled + reservationLockedToMax",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						MemoryAdvanced: &vmopv1.VirtualMachineMemoryAdvancedSpec{
							HotAddEnabled:          ptr.To(true),
							ReservationLockedToMax: ptr.To(false),
						},
					},
				},
			},
			{
				name: "spec.resources + spec.cpuAdvanced + spec.memoryAdvanced combined",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Resources: &vmopv1.VirtualMachineResourcesSpec{
							Size: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    resource.MustParse("4"),
								Memory: resource.MustParse("8Gi"),
							},
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								Memory: resource.MustParse("8Gi"),
							},
						},
						CPUAdvanced: &vmopv1.VirtualMachineCPUAdvancedSpec{
							LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading),
							Topology: &vmopv1.VirtualMachineCPUTopologySpec{
								CoresPerSocket: ptr.To(int32(2)),
							},
							IOMMUEnabled: ptr.To(true),
						},
						MemoryAdvanced: &vmopv1.VirtualMachineMemoryAdvancedSpec{
							ReservationLockedToMax: ptr.To(true),
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
