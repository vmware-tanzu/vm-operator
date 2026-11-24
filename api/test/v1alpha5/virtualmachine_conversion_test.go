// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/test/utilconversion"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
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
									UnitNumber:  ptr.To(int32(9)),
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
								CPU:    ptr.To(resource.MustParse("4")),
								Memory: ptr.To(resource.MustParse("8Gi")),
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
								CPU:    ptr.To(resource.MustParse("2000")),
								Memory: ptr.To(resource.MustParse("4Gi")),
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
								CPU:    ptr.To(resource.MustParse("4000")),
								Memory: ptr.To(resource.MustParse("8Gi")),
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
								CPU:    ptr.To(resource.MustParse("8")),
								Memory: ptr.To(resource.MustParse("16Gi")),
							},
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    ptr.To(resource.MustParse("2000")),
								Memory: ptr.To(resource.MustParse("8Gi")),
							},
							Limits: &vmopv1.VirtualMachineResourceQuantity{
								CPU:    ptr.To(resource.MustParse("4000")),
								Memory: ptr.To(resource.MustParse("16Gi")),
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
								CoresPerSocket: ptr.To(int32(4)),
								VNUMANodeCount: ptr.To(int32(8)),
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
								CPU:    ptr.To(resource.MustParse("4")),
								Memory: ptr.To(resource.MustParse("8Gi")),
							},
							Requests: &vmopv1.VirtualMachineResourceQuantity{
								Memory: ptr.To(resource.MustParse("8Gi")),
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
			{
				// DHCP4=&false must survive the round-trip via annotation restoration.
				name: "spec.network.interfaces.dhcp4=false (explicit static)",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{Name: "eth0", DHCP4: ptr.To(false)},
							},
						},
					},
				},
			},
			{
				// DHCP6=&false must survive the round-trip via annotation restoration.
				name: "spec.network.interfaces.dhcp6=false (explicit static)",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{Name: "eth0", DHCP6: ptr.To(false)},
							},
						},
					},
				},
			},
			{
				// DHCP4=&true round-trips without annotation (basic conversion handles it).
				name: "spec.network.interfaces.dhcp4=true",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{Name: "eth0", DHCP4: ptr.To(true)},
							},
						},
					},
				},
			},
			{
				// DHCP6=&true round-trips without annotation (basic conversion handles it).
				name: "spec.network.interfaces.dhcp6=true",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{Name: "eth0", DHCP6: ptr.To(true)},
							},
						},
					},
				},
			},
			{
				// DHCP4=nil round-trips correctly (no annotation needed).
				name: "spec.network.interfaces.dhcp4=nil",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{Name: "eth0"},
							},
						},
					},
				},
			},
			{
				// Dual-stack with mixed explicit DHCP intent.
				name: "spec.network.interfaces.dhcp4=false, dhcp6=true",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:  "eth0",
									DHCP4: ptr.To(false),
									DHCP6: ptr.To(true),
								},
							},
						},
					},
				},
			},
			{
				name: "spec.affinity",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Affinity: &vmopv1.AffinitySpec{
							VMAffinity: &vmopv1.VMAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar",
											},
										},
										TopologyKey: "topology.kubernetes.io/abc",
									},
								},
								PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "trivia",
											},
										},
										TopologyKey: "topology.kubernetes.io/xyz",
									},
								},
								RequiredDuringSchedulingRequiredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "hard-affinity",
											},
										},
										TopologyKey: "topology.kubernetes.io/jkl",
									},
								},
							},
							VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "chess",
											},
										},
										TopologyKey: "topology.kubernetes.io/def",
									},
								},
								PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "football",
											},
										},
										TopologyKey: "topology.kubernetes.io/ghi",
									},
								},
								RequiredDuringSchedulingRequiredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "hard-anti-affinity",
											},
										},
										TopologyKey: "topology.kubernetes.io/mno",
									},
								},
							},
						},
					},
				},
			},
			{
				name: "status.network.interfaces[].vnumaNodeID and vmxnet3",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Network: &vmopv1.VirtualMachineNetworkStatus{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceStatus{
								{
									Name:        "eth0",
									VNUMANodeID: ptr.To(int32(2)),
									UnitNumber:  ptr.To(int32(3)),
									VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Status{
										UPTv2Enabled:             ptr.To(true),
										UPTv2Active:              ptr.To(false),
										UPTv2InactiveReasonVM:    []string{"vm-reason"},
										UPTv2InactiveReasonOther: []string{"other-reason"},
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

	t.Run("spec.network.interfaces unitNumber restored by name across interface removal and re-add", func(t *testing.T) {
		// An old-version UPDATE that removes an interface and adds a different
		// one reusing the same name must not wipe the hub-only unitNumber
		// (k8s#111703): the annotation-based restore re-attaches the saved
		// value by name. This is safe because, once set, the unit number is
		// the interface's identity for the hardware at that slot, and the
		// reconciler replaces the device at that slot to converge the new
		// interface's desired state (spec G12).
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:       "eth0",
							Type:       vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
							UnitNumber: ptr.To(int32(9)),
						},
					},
				},
			},
		}

		// Down-convert hub -> spoke (stores hub data in the annotation).
		var spoke vmopv1a5.VirtualMachine
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())

		// Simulate the old-version UPDATE: remove eth0 and add a different
		// interface that reuses the name.
		spoke.Spec.Network.Interfaces = []vmopv1a5.VirtualMachineNetworkInterfaceSpec{
			{
				Name: "eth0",
			},
		}

		// Up-convert spoke -> hub: unitNumber must be re-attached by name.
		var hubAfter vmopv1.VirtualMachine
		g.Expect(spoke.ConvertTo(&hubAfter)).To(Succeed())
		g.Expect(hubAfter.Spec.Network.Interfaces).To(HaveLen(1))
		g.Expect(hubAfter.Spec.Network.Interfaces[0].Name).To(Equal("eth0"))
		g.Expect(hubAfter.Spec.Network.Interfaces[0].UnitNumber).To(Equal(ptr.To(int32(9))))
	})

	t.Run("spec.network.interfaces.dhcp-spoke-override", func(t *testing.T) {
		// Verify that when hub has DHCP4=*false the spoke user can override it
		// to true and the up-conversion produces *true, not *false.
		// The restore guard must not overwrite a spoke-expressed true with the
		// annotation's false.
		g := NewWithT(t)

		hub := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0", DHCP4: ptr.To(false), DHCP6: ptr.To(false)},
					},
				},
			},
		}

		spoke := &vmopv1a5.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		// Spoke reflects down-conversion: both are false (zero value).
		g.Expect(spoke.Spec.Network.Interfaces[0].DHCP4).To(BeFalse())
		g.Expect(spoke.Spec.Network.Interfaces[0].DHCP6).To(BeFalse())

		// Spoke user explicitly enables DHCP.
		spoke.Spec.Network.Interfaces[0].DHCP4 = true
		spoke.Spec.Network.Interfaces[0].DHCP6 = true

		after := &vmopv1.VirtualMachine{}
		g.Expect(spoke.ConvertTo(after)).To(Succeed())

		// Hub must reflect the spoke user's intent (*true), not the annotation's *false.
		iface := after.Spec.Network.Interfaces[0]
		g.Expect(iface.DHCP4).NotTo(BeNil(), "DHCP4 must not be nil")
		g.Expect(*iface.DHCP4).To(BeTrue(), "DHCP4 must be *true after spoke override")
		g.Expect(iface.DHCP6).NotTo(BeNil(), "DHCP6 must not be nil")
		g.Expect(*iface.DHCP6).To(BeTrue(), "DHCP6 must be *true after spoke override")
	})

	t.Run("status.network.interfaces[].vnumaNodeID and vmxnet3 dropped on spoke", func(t *testing.T) {
		g := NewWithT(t)
		hub := &vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Network: &vmopv1.VirtualMachineNetworkStatus{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceStatus{
						{
							Name:        "eth0",
							VNUMANodeID: ptr.To(int32(2)),
							VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Status{
								UPTv2Active:              ptr.To(false),
								UPTv2InactiveReasonVM:    []string{"vm-reason"},
								UPTv2InactiveReasonOther: []string{"other-reason"},
							},
						},
					},
				},
			},
		}
		spoke := &vmopv1a5.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.Status.Network).NotTo(BeNil())
		g.Expect(spoke.Status.Network.Interfaces).To(HaveLen(1))
		g.Expect(spoke.Status.Network.Interfaces[0].Name).To(Equal("eth0"))
	})

	t.Run("spec.affinity RequiredDuringSchedulingRequiredDuringExecution dropped on spoke", func(t *testing.T) {
		g := NewWithT(t)
		hub := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Affinity: &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							{TopologyKey: "topology.kubernetes.io/abc"},
						},
						RequiredDuringSchedulingRequiredDuringExecution: []vmopv1.VMAffinityTerm{
							{TopologyKey: "topology.kubernetes.io/jkl"},
						},
					},
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingRequiredDuringExecution: []vmopv1.VMAffinityTerm{
							{TopologyKey: "topology.kubernetes.io/mno"},
						},
					},
				},
			},
		}

		spoke := &vmopv1a5.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		// The v1alpha5 VMAffinitySpec/VMAntiAffinitySpec types have no field
		// for RequiredDuringSchedulingRequiredDuringExecution, so the
		// basic conversion still carries the sibling field that DOES
		// exist on both versions.
		g.Expect(spoke.Spec.Affinity.VMAffinity.RequiredDuringSchedulingPreferredDuringExecution).To(HaveLen(1))

		// The hard-affinity data must be stashed in the conversion
		// annotation rather than silently discarded.
		annotation, ok := spoke.Annotations[utilconversion.AnnotationKey]
		g.Expect(ok).To(BeTrue(), "conversion annotation must be set")
		g.Expect(annotation).To(ContainSubstring("topology.kubernetes.io/jkl"))
		g.Expect(annotation).To(ContainSubstring("topology.kubernetes.io/mno"))

		// Converting back up must restore it from the annotation.
		after := &vmopv1.VirtualMachine{}
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(after.Spec.Affinity.VMAffinity.RequiredDuringSchedulingRequiredDuringExecution).To(HaveLen(1))
		g.Expect(after.Spec.Affinity.VMAffinity.RequiredDuringSchedulingRequiredDuringExecution[0].TopologyKey).
			To(Equal("topology.kubernetes.io/jkl"))
		g.Expect(after.Spec.Affinity.VMAntiAffinity.RequiredDuringSchedulingRequiredDuringExecution).To(HaveLen(1))
		g.Expect(after.Spec.Affinity.VMAntiAffinity.RequiredDuringSchedulingRequiredDuringExecution[0].TopologyKey).
			To(Equal("topology.kubernetes.io/mno"))
	})
}

func TestVirtualMachineFlexAPIRoundTrip(t *testing.T) {
	for _, profile := range []*string{nil, ptr.To(""), ptr.To("reserved-small")} {
		profileName := "nil"
		if profile != nil {
			profileName = "value-" + *profile
		}
		for name, newSpoke := range map[string]func() ctrlconversion.Convertible{
			"v1alpha1": func() ctrlconversion.Convertible { return &vmopv1a1.VirtualMachine{} },
			"v1alpha2": func() ctrlconversion.Convertible { return &vmopv1a2.VirtualMachine{} },
			"v1alpha3": func() ctrlconversion.Convertible { return &vmopv1a3.VirtualMachine{} },
			"v1alpha4": func() ctrlconversion.Convertible { return &vmopv1a4.VirtualMachine{} },
			"v1alpha5": func() ctrlconversion.Convertible { return &vmopv1a5.VirtualMachine{} },
		} {
			t.Run(name+"/reservedProfileName="+profileName, func(t *testing.T) {
				spoke := newSpoke()
				g := NewWithT(t)
				original := &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						ReservedProfileName: profile,
						Hardware: &vmopv1.VirtualMachineHardwareSpec{
							VGPUs: []vmopv1.VirtualMachineVGPUDevice{
								{Name: "gpu-0", Type: vmopv1.VirtualMachineVGPUTypeNVIDIA, ProfileName: "gpu-small", EnableUVM: ptr.To(false)},
								{Name: "gpu-1", ProfileName: "gpu-small"},
							},
							DynamicDirectPathIODevices: []vmopv1.VirtualMachineDynamicDirectPathIODevice{
								{Name: "pci-0", VendorID: ptr.To[int64](4318), DeviceID: ptr.To[int64](0), CustomLabel: "device"},
								{Name: "pci-1"},
							},
						},
					},
					Status: vmopv1.VirtualMachineStatus{
						ChangeBlockTracking: ptr.To(false),
						Hardware: &vmopv1.VirtualMachineHardwareStatus{
							Firmware:   vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
							SEVEnabled: ptr.To(false), SEVSNPEnabled: ptr.To(true), TDXEnabled: ptr.To(false), SMCPresent: ptr.To(true),
							CPU: &vmopv1.VirtualMachineCPUAllocationStatus{
								HotRemoveEnabled: ptr.To(false),
							},
						},
					},
				}
				g.Expect(spoke.ConvertFrom(original)).To(Succeed())
				// Exercise the actual wire format, including the conversion annotation.
				wire, err := json.Marshal(spoke)
				g.Expect(err).NotTo(HaveOccurred())
				fromWire := newSpoke()
				g.Expect(json.Unmarshal(wire, fromWire)).To(Succeed())
				restored := &vmopv1.VirtualMachine{}
				g.Expect(fromWire.ConvertTo(restored)).To(Succeed())
				g.Expect(restored.Spec.ReservedProfileName).To(Equal(profile))
				g.Expect(restored.Spec.Hardware.VGPUs).To(Equal(original.Spec.Hardware.VGPUs))
				g.Expect(restored.Spec.Hardware.DynamicDirectPathIODevices).To(Equal(original.Spec.Hardware.DynamicDirectPathIODevices))
				g.Expect(restored.Status).To(Equal(original.Status))
			})
		}
	}
}

func TestVirtualMachineFlexAPIDeepCopy(t *testing.T) {
	g := NewWithT(t)
	original := &vmopv1.VirtualMachine{
		Spec: vmopv1.VirtualMachineSpec{ReservedProfileName: ptr.To(""), Hardware: &vmopv1.VirtualMachineHardwareSpec{
			VGPUs:                      []vmopv1.VirtualMachineVGPUDevice{{Name: "gpu-0", ProfileName: "gpu", EnableUVM: ptr.To(false)}},
			DynamicDirectPathIODevices: []vmopv1.VirtualMachineDynamicDirectPathIODevice{{Name: "pci-0", VendorID: ptr.To[int64](1)}},
		}},
		Status: vmopv1.VirtualMachineStatus{Hardware: &vmopv1.VirtualMachineHardwareStatus{
			SEVEnabled: ptr.To(false), CPU: &vmopv1.VirtualMachineCPUAllocationStatus{HotRemoveEnabled: ptr.To(false)},
		}},
	}
	copied := original.DeepCopy()
	*copied.Spec.ReservedProfileName = "changed"
	copied.Spec.Hardware.VGPUs[0].ProfileName = "changed"
	*copied.Spec.Hardware.VGPUs[0].EnableUVM = true
	*copied.Spec.Hardware.DynamicDirectPathIODevices[0].VendorID = 2
	*copied.Status.Hardware.SEVEnabled = true
	*copied.Status.Hardware.CPU.HotRemoveEnabled = true
	g.Expect(*original.Spec.ReservedProfileName).To(BeEmpty())
	g.Expect(original.Spec.Hardware.VGPUs[0].ProfileName).To(Equal("gpu"))
	g.Expect(*original.Spec.Hardware.VGPUs[0].EnableUVM).To(BeFalse())
	g.Expect(*original.Spec.Hardware.DynamicDirectPathIODevices[0].VendorID).To(Equal(int64(1)))
	g.Expect(*original.Status.Hardware.SEVEnabled).To(BeFalse())
	g.Expect(*original.Status.Hardware.CPU.HotRemoveEnabled).To(BeFalse())
	wire, err := json.Marshal(original.Spec)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(wire)).To(ContainSubstring(`"reservedProfileName":""`))
	original.Spec.ReservedProfileName = nil
	wire, err = json.Marshal(original.Spec)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(wire)).NotTo(ContainSubstring("reservedProfileName"))
}

func TestVirtualMachineReplicaSetFlexAPIRoundTrip(t *testing.T) {
	for _, profile := range []*string{nil, ptr.To(""), ptr.To("reserved-small")} {
		profileName := "nil"
		if profile != nil {
			profileName = "value-" + *profile
		}
		for name, newSpoke := range map[string]func() ctrlconversion.Convertible{
			"v1alpha3": func() ctrlconversion.Convertible { return &vmopv1a3.VirtualMachineReplicaSet{} }, //nolint:staticcheck // Deprecated spokes are still served.
			"v1alpha4": func() ctrlconversion.Convertible { return &vmopv1a4.VirtualMachineReplicaSet{} }, //nolint:staticcheck // Deprecated spokes are still served.
			"v1alpha5": func() ctrlconversion.Convertible { return &vmopv1a5.VirtualMachineReplicaSet{} },
		} {
			t.Run(name+"/reservedProfileName="+profileName, func(t *testing.T) {
				g := NewWithT(t)
				original := &vmopv1.VirtualMachineReplicaSet{
					Spec: vmopv1.VirtualMachineReplicaSetSpec{
						Template: vmopv1.VirtualMachineTemplateSpec{
							Spec: vmopv1.VirtualMachineSpec{
								ReservedProfileName: profile,
								Hardware: &vmopv1.VirtualMachineHardwareSpec{
									VGPUs: []vmopv1.VirtualMachineVGPUDevice{
										{Name: "gpu-0", Type: vmopv1.VirtualMachineVGPUTypeNVIDIA, ProfileName: "gpu-small", EnableUVM: ptr.To(false)},
									},
									DynamicDirectPathIODevices: []vmopv1.VirtualMachineDynamicDirectPathIODevice{
										{Name: "pci-0", VendorID: ptr.To[int64](4318), DeviceID: ptr.To[int64](0), CustomLabel: "device"},
									},
								},
							},
						},
					},
				}
				spoke := newSpoke()
				g.Expect(spoke.ConvertFrom(original)).To(Succeed())
				wire, err := json.Marshal(spoke)
				g.Expect(err).NotTo(HaveOccurred())
				fromWire := newSpoke()
				g.Expect(json.Unmarshal(wire, fromWire)).To(Succeed())
				restored := &vmopv1.VirtualMachineReplicaSet{}
				g.Expect(fromWire.ConvertTo(restored)).To(Succeed())
				tmpl := restored.Spec.Template.Spec
				g.Expect(tmpl.ReservedProfileName).To(Equal(profile))
				g.Expect(tmpl.Hardware).NotTo(BeNil())
				g.Expect(tmpl.Hardware.VGPUs).To(Equal(original.Spec.Template.Spec.Hardware.VGPUs))
				g.Expect(tmpl.Hardware.DynamicDirectPathIODevices).To(Equal(original.Spec.Template.Spec.Hardware.DynamicDirectPathIODevices))
			})
		}
	}
}
