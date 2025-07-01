// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("ReconcileNetworkInterfaces", func() {

	var (
		ctx             context.Context
		results         *network.NetworkInterfaceResults
		currentEthCards object.VirtualDeviceList

		deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
		err           error
	)

	BeforeEach(func() {
		results = &network.NetworkInterfaceResults{}
	})

	JustBeforeEach(func() {
		deviceChanges, err = network.ReconcileNetworkInterfaces(
			ctx,
			results,
			currentEthCards)
	})

	DescribeTableSubtree("NetworkEnv",
		func(networkEnv builder.NetworkEnv) {
			const (
				interfaceName0 = "eth0"
			)

			var (
				ethCard vimtypes.BaseVirtualEthernetCard
				obj     ctrlclient.Object
			)

			BeforeEach(func() {
				ethCard = &vimtypes.VirtualVmxnet3{}

				switch networkEnv {
				case builder.NetworkEnvVDS:
					ethCard.GetVirtualEthernetCard().AddressType = string(vimtypes.VirtualEthernetCardMacTypeGenerated)
					ethCard.GetVirtualEthernetCard().MacAddress = ""
					ethCard.GetVirtualEthernetCard().ExternalId = ""
					ethCard.GetVirtualEthernetCard().Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: "pg-1",
						},
					}
				case builder.NetworkEnvNSXT, builder.NetworkEnvVPC:
					ethCard.GetVirtualEthernetCard().MacAddress = "my-mac"
					ethCard.GetVirtualEthernetCard().AddressType = string(vimtypes.VirtualEthernetCardMacTypeAssigned)
					ethCard.GetVirtualEthernetCard().ExternalId = "my-ext-id"
					ethCard.GetVirtualEthernetCard().Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: "pg-1",
						},
					}
				}

				switch networkEnv {
				case builder.NetworkEnvVDS:
					obj = &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-vds-interface",
							Labels: map[string]string{
								network.VMInterfaceNameLabel: interfaceName0,
							},
						},
						Status: netopv1alpha1.NetworkInterfaceStatus{
							NetworkID: "pg-1",
						},
					}
				case builder.NetworkEnvNSXT:
					obj = &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ncp-interface",
							Labels: map[string]string{
								network.VMInterfaceNameLabel: interfaceName0,
							},
						},
						Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
							InterfaceID: ethCard.GetVirtualEthernetCard().ExternalId,
							MacAddress:  ethCard.GetVirtualEthernetCard().MacAddress,
						},
					}
				case builder.NetworkEnvVPC:
					obj = &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-vpc-interface",
							Labels: map[string]string{
								network.VMInterfaceNameLabel: interfaceName0,
							},
						},
						Status: vpcv1alpha1.SubnetPortStatus{
							Attachment: vpcv1alpha1.PortAttachment{
								ID: ethCard.GetVirtualEthernetCard().ExternalId,
							},
							NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
								MACAddress: ethCard.GetVirtualEthernetCard().MacAddress,
							},
						},
					}
				}
			})

			AfterEach(func() {
				ethCard = nil
				obj = nil
				currentEthCards = nil
			})

			It("Returns success for empty input", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
			})

			When("Add", func() {
				BeforeEach(func() {
					results.Results = append(results.Results, network.NetworkInterfaceResult{
						Name:   interfaceName0,
						Device: ethCard.(vimtypes.BaseVirtualDevice),
					})

				})

				It("Returns Add Operation", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(deviceChanges).To(HaveLen(1))
					dc0 := deviceChanges[0].GetVirtualDeviceConfigSpec()
					Expect(dc0.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
					Expect(dc0.Device).To(Equal(ethCard))
				})
			})

			When("Edit", func() {
				BeforeEach(func() {
					curEthCard := *ethCard.GetVirtualEthernetCard()

					ethCard.GetVirtualEthernetCard().Backing = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
						Port: vimtypes.DistributedVirtualSwitchPortConnection{
							PortgroupKey: "pg-new-42",
						},
					}

					results.Results = append(results.Results, network.NetworkInterfaceResult{
						Name:   interfaceName0,
						Device: ethCard.(vimtypes.BaseVirtualDevice),
					})

					results.OrphanedNetworkInterfaces = append(results.OrphanedNetworkInterfaces, obj)

					currentEthCards = append(currentEthCards, &curEthCard)
				})

				It("Returns Edit Operation", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(deviceChanges).To(HaveLen(1))
					dc0 := deviceChanges[0].GetVirtualDeviceConfigSpec()
					Expect(dc0.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
					Expect(dc0.Device.GetVirtualDevice().Backing).To(Equal(ethCard.GetVirtualEthernetCard().Backing))
				})

				When("Network Interface CR is old and does not have interface name label", func() {
					BeforeEach(func() {
						obj.SetName("foo-" + interfaceName0)
						delete(obj.GetLabels(), network.VMInterfaceNameLabel)
					})

					It("Returns Edit Operation", func() {
						Expect(err).ToNot(HaveOccurred())

						Expect(deviceChanges).To(HaveLen(1))
						dc0 := deviceChanges[0].GetVirtualDeviceConfigSpec()
						Expect(dc0.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
						Expect(dc0.Device.GetVirtualDevice().Backing).To(Equal(ethCard.GetVirtualEthernetCard().Backing))
					})
				})
			})

			When("Remove", func() {
				BeforeEach(func() {
					currentEthCards = append(currentEthCards, ethCard.(vimtypes.BaseVirtualDevice))
				})

				It("Returns Remove Operation", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(deviceChanges).To(HaveLen(1))
					dc0 := deviceChanges[0].GetVirtualDeviceConfigSpec()
					Expect(dc0.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
					Expect(dc0.Device.GetVirtualDevice().Backing).To(Equal(ethCard.GetVirtualEthernetCard().Backing))
				})
			})
		},
		Entry("VDS", builder.NetworkEnvVDS),
		Entry("NSX-T", builder.NetworkEnvNSXT),
		Entry("VPC", builder.NetworkEnvVPC),
	)
})
