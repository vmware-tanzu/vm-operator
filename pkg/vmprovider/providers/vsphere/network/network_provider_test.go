// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	goCtx "context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware/govmomi/vim25/types"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("NetworkProvider", func() {

	const (
		macAddress     = "01-23-45-67-89-AB-CD-EF"
		interfaceID    = "interface-id"
		vcsimPortGroup = "dvportgroup-11"
		dummyNetIfName = "dummy-netIf-name"
		doesNotExist   = "does-not-exist"
	)

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim

		vm    *vmopv1.VirtualMachine
		vmCtx context.VirtualMachineContext
		np    network.Provider
	)

	createInterface := func(_ *builder.TestContextForVCSim) {
		info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(info).NotTo(BeNil())

		nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
		Expect(nic).NotTo(BeNil())
		Expect(nic.ExternalId).To(Equal(interfaceID))
		Expect(nic.MacAddress).To(Equal(macAddress))
		Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))
	}

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "network-provider-test-vm",
				Namespace: "network-provider-test-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				NetworkInterfaces: []vmopv1.VirtualMachineNetworkInterface{
					{},
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  logf.Log.WithName("network_test"),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		np = nil
	})

	Context("Named Network Provider", func() {

		var (
			oldNetworkProviderType      string
			namedNetworkProviderEnabled bool
		)

		JustBeforeEach(func() {
			oldNetworkProviderType = os.Getenv(lib.NetworkProviderType)
			if namedNetworkProviderEnabled {
				Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeNamed)).To(Succeed())
			}
			np = network.NewProvider(ctx.Client, ctx.VCClient.Client, ctx.Finder, nil)
		})

		AfterEach(func() {
			Expect(os.Setenv(lib.NetworkProviderType, oldNetworkProviderType)).To(Succeed())
		})

		Context("Disabled", func() {
			Context("ensure interface", func() {
				It("should be fail with ErrNamedNetworkProviderNotSupported", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(network.ErrNamedNetworkProviderNotSupported))
				})
			})
		})

		Context("Enabled", func() {
			BeforeEach(func() {
				namedNetworkProviderEnabled = true
				vm.Spec.NetworkInterfaces[0].NetworkName = "DC0_DVPG0"
			})

			Context("ensure interface", func() {
				It("create expected virtual device", func() {
					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					Expect(info.Device).NotTo(BeNil())
					backing := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard().Backing
					Expect(backing).NotTo(BeNil())
					backingInfo, ok := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).To(BeTrue())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
				})

				It("create expected interface customization", func() {
					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info.Customization).ToNot(BeNil())
					Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
				})

				It("should return an error if network does not exist", func() {
					vmCtx.VM.Spec.NetworkInterfaces[0].NetworkName = doesNotExist
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Sprintf("unable to find network \"%s\": network '%s' not found", doesNotExist, doesNotExist)))
				})
			})
		})
	})

	Context("NetOP Network Provider", func() {
		const (
			networkName = "netop-network"
		)

		var (
			netIf   *netopv1alpha1.NetworkInterface
			dummyIP = "192.168.100.20"
		)

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS

			vm.Spec.NetworkInterfaces[0].NetworkType = network.VdsNetworkType
			vm.Spec.NetworkInterfaces[0].NetworkName = networkName

			netIf = &netopv1alpha1.NetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s", vm.Spec.NetworkInterfaces[0].NetworkName, vm.Name),
					Namespace: vm.Namespace,
				},
				Spec: netopv1alpha1.NetworkInterfaceSpec{
					NetworkName: networkName,
					Type:        netopv1alpha1.NetworkInterfaceTypeVMXNet3,
				},
				Status: netopv1alpha1.NetworkInterfaceStatus{
					Conditions: []netopv1alpha1.NetworkInterfaceCondition{
						{
							Type:   netopv1alpha1.NetworkInterfaceReady,
							Status: corev1.ConditionTrue,
						},
					},
					IPConfigs: []netopv1alpha1.IPConfig{
						{
							IP:       dummyIP,
							IPFamily: netopv1alpha1.IPv4Protocol,
						},
					},
					MacAddress: macAddress,
					ExternalID: interfaceID,
					NetworkID:  vcsimPortGroup,
				},
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, netIf)).To(Succeed())
			np = network.NewProvider(ctx.Client, ctx.VCClient.Client, ctx.Finder, ctx.GetSingleClusterCompute())
		})

		Context("ensure interface", func() {

			// Long test due to poll timeout.
			It("create netop network interface object", func() {
				Expect(ctx.Client.Delete(ctx, netIf)).To(Succeed())

				_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(goCtx.DeadlineExceeded))

				instance := &netopv1alpha1.NetworkInterface{}
				err = ctx.Client.Get(ctx, ctrlruntime.ObjectKeyFromObject(netIf), instance)
				Expect(err).ToNot(HaveOccurred())
				Expect(instance.Spec.NetworkName).To(Equal(networkName))

				Expect(instance.OwnerReferences).To(HaveLen(1))
				Expect(instance.OwnerReferences[0].Name).To(Equal(vm.Name))
			})

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = ""
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("unable to get ethernet card backing info for network DistributedVirtualPortgroup::"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					netIf.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(MatchError(goCtx.DeadlineExceeded))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to get ethernet card backing info for network DistributedVirtualPortgroup:" + doesNotExist))
				})
			})

			Context("when the network name is not specified", func() {
				BeforeEach(func() {
					netIf.Name = vm.Name
					vm.Spec.NetworkInterfaces[0].NetworkName = ""
				})

				It("should succeed", func() {
					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					instance := &netopv1alpha1.NetworkInterface{}
					err = ctx.Client.Get(ctx, ctrlruntime.ObjectKeyFromObject(netIf), instance)
					Expect(err).ToNot(HaveOccurred())
					Expect(instance.Spec.NetworkName).To(Equal(""))
				})
			})

			It("should succeed", func() {
				info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
				Expect(err).ToNot(HaveOccurred())
				Expect(info).NotTo(BeNil())

				Expect(info.Device).ToNot(BeNil())
				nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
				Expect(nic).NotTo(BeNil())
				Expect(nic.ExternalId).To(Equal(interfaceID))
				Expect(nic.MacAddress).To(Equal(macAddress))
				Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))

				backing := nic.Backing
				Expect(backing).To(BeAssignableToTypeOf(&types.VirtualEthernetCardDistributedVirtualPortBackingInfo{}))
				backingInfo := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(backingInfo.Port.PortgroupKey).To(Equal(vcsimPortGroup))
			})

			Context("interface without MAC address", func() {
				BeforeEach(func() {
					netIf.Status.MacAddress = ""
				})

				It("should succeed with generated mac", func() {
					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					Expect(info.Device).ToNot(BeNil())
					nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceID))
					Expect(nic.MacAddress).To(BeEmpty())
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeGenerated)))
				})
			})

			Context("with NetworkInterfaceProvider Referenced in vmNif", func() {
				BeforeEach(func() {
					netIf.Name = dummyNetIfName
					vm.Spec.NetworkInterfaces[0].ProviderRef = &vmopv1.NetworkInterfaceProviderReference{
						APIGroup:   "netoperator.vmware.com",
						APIVersion: "v1alpha1",
						Kind:       "NetworkInterface",
						Name:       dummyNetIfName,
					}
				})

				It("should succeed", func() {
					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					Expect(info.Device).ToNot(BeNil())
					nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceID))
					Expect(nic.MacAddress).To(Equal(macAddress))
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))

					backing := nic.Backing
					Expect(backing).To(BeAssignableToTypeOf(&types.VirtualEthernetCardDistributedVirtualPortBackingInfo{}))
					backingInfo := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(backingInfo.Port.PortgroupKey).To(Equal(vcsimPortGroup))
				})

				Context("referencing wrong netIf name", func() {
					BeforeEach(func() {
						vm.Spec.NetworkInterfaces[0].ProviderRef.Name = doesNotExist
					})

					It("should return an error", func() {
						_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(goCtx.DeadlineExceeded))
					})
				})

				Context("referencing unsupported GVK", func() {
					BeforeEach(func() {
						vm.Spec.NetworkInterfaces[0].ProviderRef = &vmopv1.NetworkInterfaceProviderReference{
							APIGroup:   "unsupported-group",
							APIVersion: "unsupported-version",
							Kind:       "unsupported-kind",
							Name:       dummyNetIfName,
						}
					})

					It("should return an error", func() {
						_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("unsupported NetworkInterface ProviderRef"))
					})
				})
			})

			Context("with NSX-T NetworkType in ProviderRef", func() {
				BeforeEach(func() {
					testConfig.WithNetworkEnv = builder.NetworkEnvNSXT

					netIf.Name = dummyNetIfName
					netIf.Status.NetworkID = builder.NsxTLogicalSwitchUUID
					netIf.Status.IPConfigs = nil

					vm.Spec.NetworkInterfaces[0].NetworkType = network.NsxtNetworkType
					vm.Spec.NetworkInterfaces[0].ProviderRef = &vmopv1.NetworkInterfaceProviderReference{
						APIGroup:   "netoperator.vmware.com",
						APIVersion: "v1alpha1",
						Kind:       "NetworkInterface",
						Name:       dummyNetIfName,
					}
				})

				Context("should succeed", func() {

					It("with no provider IP configuration", func() {
						createInterface(ctx)

						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
					})
				})
			})

			Context("expected guest customization", func() {

				Context("no IPConfigs", func() {
					BeforeEach(func() { netIf.Status.IPConfigs = nil })

					It("dhcp customization", func() {
						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
					})
				})

				Context("IPv4 IPConfigs", func() {
					ip := "192.168.100.1"

					BeforeEach(func() {
						netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
							{
								IP:       ip,
								IPFamily: netopv1alpha1.IPv4Protocol,
							},
						}
					})

					It("fixed ipv4 customization", func() {
						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationFixedIp{}))
						fixedIP := info.Customization.Adapter.Ip.(*types.CustomizationFixedIp)
						Expect(fixedIP.IpAddress).To(Equal(ip))
					})
				})

				Context("IPv6 IPConfigs", func() {
					ip := "2607:f8b0:4004:809::2004"

					BeforeEach(func() {
						netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
							{
								IP:       ip,
								IPFamily: netopv1alpha1.IPv6Protocol,
							},
						}
					})

					It("fixed ipv6 customization", func() {
						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.IpV6Spec).To(BeAssignableToTypeOf(&types.CustomizationIPSettingsIpV6AddressSpec{}))
						Expect(info.Customization.Adapter.IpV6Spec.Ip).To(HaveLen(1))
						fixedIP := info.Customization.Adapter.IpV6Spec.Ip[0].(*types.CustomizationFixedIpV6)
						Expect(fixedIP.IpAddress).To(Equal(ip))
					})
				})
			})

			Context("expected Netplan Ethernets", func() {

				Context("no IPConfigs", func() {
					BeforeEach(func() { netIf.Status.IPConfigs = nil })

					It("dhcp should be True", func() {
						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.NetplanEthernet.Dhcp4).To(BeTrue())
					})
				})

				Context("IPv4 IPConfigs", func() {
					ip := "192.168.1.37"
					mask := "255.255.255.0"
					gateway := "192.168.1.1"
					expectedCidrNotation := "192.168.1.37/24"

					BeforeEach(func() {
						netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
							{
								IP:         ip,
								IPFamily:   netopv1alpha1.IPv4Protocol,
								Gateway:    gateway,
								SubnetMask: mask,
							},
						}
					})

					It("NetplanEthernet with ipv4 customization", func() {
						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.NetplanEthernet.Dhcp4).To(BeFalse())
						Expect(info.NetplanEthernet.Match.MacAddress).To(Equal(network.NormalizeNetplanMac(netIf.Status.MacAddress)))
						Expect(info.NetplanEthernet.Gateway4).ToNot(BeEmpty())
						Expect(info.NetplanEthernet.Gateway4).To(Equal(netIf.Status.IPConfigs[0].Gateway))
						Expect(info.NetplanEthernet.Addresses).To(HaveLen(1))
						Expect(info.NetplanEthernet.Addresses[0]).To(Equal(expectedCidrNotation))
					})
				})
			})
		})
	})

	Context("NSX-T Network Provider", func() {
		const (
			networkName = "ncp-network"
		)

		var (
			ncpVif *ncpv1alpha1.VirtualNetworkInterface
		)

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNSXT

			vm.Spec.NetworkInterfaces[0].NetworkType = network.NsxtNetworkType
			vm.Spec.NetworkInterfaces[0].NetworkName = networkName

			ncpVif = &ncpv1alpha1.VirtualNetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-lsp", vm.Spec.NetworkInterfaces[0].NetworkName, vm.Name),
					Namespace: vm.Namespace,
				},
				Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
					VirtualNetwork: networkName,
				},
				Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
					MacAddress:  macAddress,
					InterfaceID: interfaceID,
					Conditions:  []ncpv1alpha1.VirtualNetworkCondition{{Type: "Ready", Status: "True"}},
					ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: builder.NsxTLogicalSwitchUUID,
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ncpVif)).To(Succeed())
			np = network.NewProvider(ctx.Client, ctx.VCClient.Client, ctx.Finder, ctx.GetSingleClusterCompute())
		})

		Context("ensure interface", func() {

			// Long test due to poll timeout.
			It("create ncp virtual network interface object", func() {
				Expect(ctx.Client.Delete(ctx, ncpVif)).To(Succeed())

				_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(goCtx.DeadlineExceeded))

				instance := &ncpv1alpha1.VirtualNetworkInterface{}
				err = ctx.Client.Get(ctx, ctrlruntime.ObjectKeyFromObject(ncpVif), instance)
				Expect(err).ToNot(HaveOccurred())
				Expect(instance.Spec.VirtualNetwork).To(Equal(networkName))

				Expect(instance.OwnerReferences).To(HaveLen(1))
				Expect(instance.OwnerReferences[0].Name).To(Equal(vm.Name))
			})

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: "",
					}
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetIf '"))
				})
			})

			Context("when interface has no opaque network id defined in the provider status", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetIf '"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					ncpVif.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(MatchError(goCtx.DeadlineExceeded))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus.NsxLogicalSwitchID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).To(MatchError(fmt.Sprintf("no DVPG with NSX-T network ID %q found", doesNotExist)))
				})
			})

			Context("should succeed", func() {

				It("with no provider IP configuration", func() {
					createInterface(ctx)

					info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
					Expect(err).ToNot(HaveOccurred())
					Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
					Expect(info.NetplanEthernet.Dhcp4).To(BeTrue())
				})

				Context("with empty IP configuration and gatewayIP supports DHCP", func() {
					gatewayIP := "192.168.100.00"
					BeforeEach(func() {
						ncpVif.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
							{
								IP:      "",
								Gateway: gatewayIP,
							},
						}
					})
					It("should work", func() {
						createInterface(ctx)

						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
						Expect(info.NetplanEthernet.Dhcp4).To(BeTrue())
					})
				})

				Context("with provider IP configuration", func() {
					ip := "192.168.100.10"
					mask := "255.255.255.0"
					gateway := "192.168.1.1"
					expectedCidrNotation := "192.168.100.10/24"

					BeforeEach(func() {
						ncpVif.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
							{
								IP:         ip,
								SubnetMask: mask,
								Gateway:    gateway,
							},
						}
					})

					It("should work", func() {
						createInterface(ctx)

						info, err := np.EnsureNetworkInterface(vmCtx, &vmCtx.VM.Spec.NetworkInterfaces[0])
						Expect(err).ToNot(HaveOccurred())
						fixedIP := info.Customization.Adapter.Ip.(*types.CustomizationFixedIp)
						Expect(fixedIP.IpAddress).To(Equal(ip))
						Expect(info.NetplanEthernet.Dhcp4).To(BeFalse())
						Expect(info.NetplanEthernet.Match.MacAddress).To(Equal(network.NormalizeNetplanMac(ncpVif.Status.MacAddress)))
						Expect(info.NetplanEthernet.Gateway4).ToNot(BeEmpty())
						Expect(info.NetplanEthernet.Gateway4).To(Equal(ncpVif.Status.IPAddresses[0].Gateway))
						Expect(info.NetplanEthernet.Addresses[0]).To(Equal(expectedCidrNotation))
					})
				})
			})
		})
	})
})

var _ = Describe("NetworkProvider utils", func() {
	Context("ToCidrNotation", func() {
		It("should work", func() {
			cidrNotation := network.ToCidrNotation("1.2.3.4", "255.255.255.0")
			Expect(cidrNotation).To(Equal("1.2.3.4/24"))
		})
	})
	Context("NormalizeNetplanMac", func() {
		It("empty string", func() {
			Expect(network.NormalizeNetplanMac("")).To(Equal(""))
		})
		It("lowercase mac", func() {
			Expect(network.NormalizeNetplanMac("12:ab:e4:99:c4")).To(Equal("12:ab:e4:99:c4"))
		})
		It("uppercase mac", func() {
			Expect(network.NormalizeNetplanMac("E4:99:C4:12:AB")).To(Equal("e4:99:c4:12:ab"))
		})
		It("mac with all numbers", func() {
			Expect(network.NormalizeNetplanMac("00:50:56:90:66")).To(Equal("00:50:56:90:66"))
		})
		It("mac with single quotes", func() {
			Expect(network.NormalizeNetplanMac("E4:99:C4:12:AB")).To(Equal("e4:99:c4:12:ab"))
		})
		It("mac with dashes", func() {
			Expect(network.NormalizeNetplanMac("ab-e4-99-c4-12")).To(Equal("ab:e4:99:c4:12"))
		})
	})
})
