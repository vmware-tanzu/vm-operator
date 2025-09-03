// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("MapEthernetDevicesToSpecIdx", func() {

	var (
		client      ctrlclient.Client
		initObjs    []ctrlclient.Object
		vmCtx       pkgctx.VirtualMachineContext
		devices     object.VirtualDeviceList
		devKeyToIdx map[int32]int
	)

	BeforeEach(func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "map-eth-dev-test",
				Namespace: "map-eth-dev-test",
			},
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}

		vmCtx = pkgctx.VirtualMachineContext{
			Context: pkgcfg.NewContextWithDefaultConfig(),
			Logger:  suite.GetLogger().WithName("map_eth_devices"),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClient(initObjs...)

		vmMo := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: devices,
				},
			},
		}
		devKeyToIdx = network.MapEthernetDevicesToSpecIdx(vmCtx, client, vmMo)
	})

	AfterEach(func() {
		initObjs = nil
		devices = nil
	})

	Context("Mutable networks is not enable", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.Features.MutableNetworks = false
			})
		})

		Context("Zips devices and interfaces together", func() {
			BeforeEach(func() {
				dev1 := &vimtypes.VirtualVmxnet3{}
				dev1.Key = 4000
				dev2 := &vimtypes.VirtualE1000e{}
				dev2.Key = 4001
				devices = append(devices, dev1, dev2)

				vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
					},
					{
						Name: "eth1",
					},
				}
			})

			It("returns expected mapping", func() {
				Expect(devKeyToIdx).To(HaveLen(2))
				Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4000), 0))
				Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4001), 1))
			})
		})
	})

	Context("Mutable networks is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.Features.MutableNetworks = true
			})
		})

		Context("VDS", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
				})
			})

			Context("Matches", func() {
				const networkName1, networkName2, networkName3 = "network-1", "network-2", "network-3"
				const backing1, backing2, backing3 = "dvpg-1", "dvpg-2", "dvpg-3"
				const externalID = "extid-1"
				const macAddress = "f8:e4:3b:7e:88:ca"

				BeforeEach(func() {
					dev1 := &vimtypes.VirtualVmxnet3{}
					dev1.Key = 4000
					dev1.Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: backing1,
						},
					}
					dev2 := &vimtypes.VirtualE1000e{}
					dev2.Key = 4001
					dev2.ExternalId = externalID
					dev2.Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: backing2,
						},
					}
					dev3 := &vimtypes.VirtualVmxnet2{}
					dev3.Key = 4002
					dev3.Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: backing2,
						},
					}
					dev4 := &vimtypes.VirtualVmxnet3{}
					dev4.Key = 4003
					dev4.MacAddress = macAddress
					dev4.Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: backing3,
						},
					}
					devices = append(devices, dev1, dev2, dev3, dev4)

					netIf1 := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vmCtx.VM.Name, networkName1, "eth0", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: netopv1alpha1.NetworkInterfaceStatus{
							NetworkID: backing1,
						},
					}
					netIf2 := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vmCtx.VM.Name, networkName2, "eth1", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: netopv1alpha1.NetworkInterfaceStatus{
							NetworkID: backing2,
						},
					}
					netIf3 := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vmCtx.VM.Name, networkName2, "eth2", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: netopv1alpha1.NetworkInterfaceStatus{
							ExternalID: externalID,
							NetworkID:  backing2,
						},
					}
					netIf4 := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vmCtx.VM.Name, networkName3, "eth3", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: netopv1alpha1.NetworkInterfaceStatus{
							NetworkID:  backing3,
							MacAddress: macAddress,
						},
					}
					initObjs = append(initObjs, netIf1, netIf2, netIf3, netIf4)

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth1",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName2,
							},
						},
						{
							Name: "eth0",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName1,
							},
						},
						{
							Name: "eth2",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName2,
							},
						},
						{
							Name: "eth3",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName3,
							},
							MACAddr: macAddress,
						},
					}
				})

				It("returns expected mapping", func() {
					Expect(devKeyToIdx).To(HaveLen(4))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4000), 1))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4001), 2))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4002), 0))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4003), 3))
				})
			})
		})

		Context("NSXT", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeNSXT
				})
			})

			Context("Matches", func() {
				const networkName1, networkName2 = "network-1", "network-2"
				const externalID1, macAddress2 = "extid-1", "macaddr-2"

				BeforeEach(func() {
					dev1 := &vimtypes.VirtualVmxnet3{}
					dev1.Key = 4000
					dev1.ExternalId = externalID1
					dev2 := &vimtypes.VirtualE1000e{}
					dev2.Key = 4001
					dev2.MacAddress = macAddress2
					devices = append(devices, dev1, dev2)

					netIf1 := &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NCPCRName(vmCtx.VM.Name, networkName1, "eth0", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
							InterfaceID: externalID1,
						},
					}
					netIf2 := &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NCPCRName(vmCtx.VM.Name, networkName2, "eth1", false),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
							MacAddress: macAddress2,
						},
					}
					initObjs = append(initObjs, netIf1, netIf2)

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth1",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName2,
							},
						},
						{
							Name: "eth0",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName1,
							},
						},
					}
				})

				It("returns expected mapping", func() {
					Expect(devKeyToIdx).To(HaveLen(2))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4000), 1))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4001), 0))
				})
			})
		})

		Context("VPC", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
				})
			})

			Context("Matches", func() {
				const networkName1, networkName2 = "network-1", "network-2"
				const externalID1, macAddress2 = "extid-1", "macaddr-2"

				BeforeEach(func() {
					dev1 := &vimtypes.VirtualVmxnet3{}
					dev1.Key = 4000
					dev1.ExternalId = externalID1
					dev2 := &vimtypes.VirtualE1000e{}
					dev2.Key = 4001
					dev2.MacAddress = macAddress2
					devices = append(devices, dev1, dev2)

					netIf1 := &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.VPCCRName(vmCtx.VM.Name, networkName1, "eth0"),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: vpcv1alpha1.SubnetPortStatus{
							Attachment: vpcv1alpha1.PortAttachment{
								ID: externalID1,
							},
						},
					}
					netIf2 := &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.VPCCRName(vmCtx.VM.Name, networkName2, "eth1"),
							Namespace: vmCtx.VM.Namespace,
						},
						Status: vpcv1alpha1.SubnetPortStatus{
							NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
								MACAddress: macAddress2,
							},
						},
					}
					initObjs = append(initObjs, netIf1, netIf2)

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth1",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName2,
							},
						},
						{
							Name: "eth0",
							Network: &vmopv1common.PartialObjectRef{
								Name: networkName1,
							},
						},
					}
				})

				It("returns expected mapping", func() {
					Expect(devKeyToIdx).To(HaveLen(2))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4000), 1))
					Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4001), 0))
				})
			})
		})
	})
})
