// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

var _ = Describe("NetworkProvider", func() {

	const (
		dummyObjectName     = "dummy-object"
		dummyNamespace      = "dummy-ns"
		dummyNsxSwitchId    = "dummy-opaque-network-id"
		macAddress          = "01-23-45-67-89-AB-CD-EF"
		interfaceId         = "interface-id"
		dummyVirtualNetwork = "dummy-virtual-net"
		vcsimPortGroup      = "dvportgroup-11"
		vcsimNetworkName    = "DC0_DVPG0"
		dummyNetIfName      = "dummy-netIf-name"
		doesNotExist        = "does-not-exist"
	)

	var (
		c      *govmomi.Client
		finder *find.Finder

		cluster *object.ClusterComputeResource
		network object.NetworkReference

		ctx   context.Context
		vmNif *v1alpha1.VirtualMachineNetworkInterface
		vm    *v1alpha1.VirtualMachine

		np vsphere.NetworkProvider

		name      string
		namespace string
	)

	BeforeEach(func() {
		ctx = context.TODO()
		c, _ = govmomi.NewClient(ctx, server.URL, true)
		finder = find.NewFinder(c.Client)

		dc, err := finder.DefaultDatacenter(ctx)
		Expect(err).To(BeNil())
		finder.SetDatacenter(dc)

		cluster, err = finder.DefaultClusterComputeResource(ctx)
		Expect(err).To(BeNil())

		network, err = finder.Network(ctx, vcsimNetworkName)
		Expect(err).To(BeNil())

		name = dummyObjectName
		namespace = dummyNamespace

		vmNif = &v1alpha1.VirtualMachineNetworkInterface{
			NetworkName: vcsimNetworkName,
		}

		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.VirtualMachineSpec{
				NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
					*vmNif,
				},
			},
		}
	})

	Context("when getting the network by type", func() {
		It("should find NSX-T", func() {
			expectedProvider := vsphere.NsxtNetworkProvider(nil, nil, nil)

			np, err := vsphere.NetworkProviderByType("nsx-t", nil, nil, nil, nil, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})

		It("should find VDS (NetOP)", func() {
			expectedProvider := vsphere.NetOpNetworkProvider(nil, nil, nil, nil, nil)

			np, err := vsphere.NetworkProviderByType("vsphere-distributed", nil, nil, nil, nil, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})

		It("should find the default", func() {
			expectedProvider := vsphere.DefaultNetworkProvider(nil)

			np, err := vsphere.NetworkProviderByType("", nil, nil, nil, nil, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(np).To(BeAssignableToTypeOf(expectedProvider))
		})
	})

	Context("when using default network provider", func() {
		BeforeEach(func() {
			np = vsphere.DefaultNetworkProvider(finder)
		})

		Context("when creating vnic", func() {
			It("create vnic should succeed", func() {
				dev, err := np.CreateVnic(ctx, vm, vmNif)
				Expect(err).ToNot(HaveOccurred())

				Expect(dev).NotTo(BeNil())
				vDev := dev.GetVirtualDevice()
				Expect(vDev).ToNot(BeNil())
				backing := vDev.Backing
				Expect(backing).NotTo(BeNil())

				backingInfo, ok := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(backingInfo.Port.PortgroupKey).To(Equal(network.Reference().Value))
			})

			It("Return expected interface customization", func() {
				_, err := np.CreateVnic(ctx, vm, vmNif)
				Expect(err).ToNot(HaveOccurred())

				cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
				Expect(err).ToNot(HaveOccurred())
				Expect(cust.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
			})

			It("should return an error if network does not exist", func() {
				_, err := np.CreateVnic(ctx, vm, &v1alpha1.VirtualMachineNetworkInterface{
					NetworkName: doesNotExist,
				})
				Expect(err).To(MatchError(fmt.Sprintf("unable to find network \"%s\": network '%s' not found", doesNotExist, doesNotExist)))
			})

			It("should ignore if vm is nil", func() {
				_, err := np.CreateVnic(ctx, nil, vmNif)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("when using NetOP network provider", func() {
		var (
			k8sClient ctrlruntime.Client
			netIf     *netopv1alpha1.NetworkInterface
			dummyIp   = "192.168.100.20"
		)

		BeforeEach(func() {
			vmNif.NetworkType = vsphere.VdsNetworkType
			vm.Spec.NetworkInterfaces[0].NetworkType = vsphere.VdsNetworkType

			netIf = &netopv1alpha1.NetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s", vmNif.NetworkName, vm.Name),
					Namespace: dummyNamespace,
				},
				Spec: netopv1alpha1.NetworkInterfaceSpec{
					NetworkName: dummyVirtualNetwork,
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
							IP:       dummyIp,
							IPFamily: netopv1alpha1.IPv4Protocol,
						},
					},
					MacAddress: macAddress,
					ExternalID: interfaceId,
					NetworkID:  vcsimPortGroup,
				},
			}
		})

		JustBeforeEach(func() {
			scheme := runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			_ = netopv1alpha1.AddToScheme(scheme)

			k8sClient = clientfake.NewFakeClientWithScheme(scheme, netIf)
			np = vsphere.NetOpNetworkProvider(k8sClient, c.Client, finder, cluster, scheme)
		})

		Context("when creating vnic", func() {

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = ""
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get NetworkID for netIf"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					netIf.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(MatchError("timed out waiting for the condition"))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to get ethernet card backing info for network DistributedVirtualPortgroup:" + doesNotExist))
				})
			})

			It("should succeed", func() {
				dev, err := np.CreateVnic(ctx, vm, vmNif)
				Expect(err).ToNot(HaveOccurred())
				Expect(dev).NotTo(BeNil())

				nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
				Expect(nic).NotTo(BeNil())
				Expect(nic.ExternalId).To(Equal(interfaceId))
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
					dev, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(dev).NotTo(BeNil())

					nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceId))
					Expect(nic.MacAddress).To(BeEmpty())
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeGenerated)))
				})
			})

			Context("with NetworkInterfaceProvider Referenced in vmNif", func() {
				BeforeEach(func() {
					netIf.Name = dummyNetIfName
					vmNif.ProviderRef = &v1alpha1.NetworkInterfaceProviderReference{
						APIGroup:   "netoperator.vmware.com",
						APIVersion: "v1alpha1",
						Kind:       "NetworkInterface",
						Name:       dummyNetIfName,
					}
					vm.Spec.NetworkInterfaces[0] = *vmNif
				})

				It("should succeed", func() {
					dev, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(dev).NotTo(BeNil())

					nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceId))
					Expect(nic.MacAddress).To(Equal(macAddress))
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))

					backing := nic.Backing
					Expect(backing).To(BeAssignableToTypeOf(&types.VirtualEthernetCardDistributedVirtualPortBackingInfo{}))
					backingInfo := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(backingInfo.Port.PortgroupKey).To(Equal(vcsimPortGroup))
				})

				Context("referencing wrong netIf name", func() {
					BeforeEach(func() {
						vmNif.ProviderRef.Name = doesNotExist
						vm.Spec.NetworkInterfaces[0] = *vmNif
					})

					It("should return an error", func() {
						_, err := np.CreateVnic(ctx, vm, vmNif)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("cannot get NetworkInterface"))
					})
				})

				Context("referencing unsupported GVK", func() {
					BeforeEach(func() {
						vmNif.ProviderRef.APIVersion = "unsupported-version"
						vmNif.ProviderRef.APIGroup = "unsupported-group"
						vmNif.ProviderRef.Kind = "unsupported-kind"
						vm.Spec.NetworkInterfaces[0] = *vmNif
					})

					It("should return an error", func() {
						_, err := np.CreateVnic(ctx, vm, vmNif)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("unsupported NetworkInterface ProviderRef"))
					})
				})
			})

			Context("with NSX-T NetworkType", func() {

				BeforeEach(func() {
					vmNif.NetworkType = vsphere.NsxtNetworkType
					vm.Spec.NetworkInterfaces[0] = *vmNif
					netIf.Status.NetworkID = dummyNsxSwitchId
					netIf.Status.IPConfigs = nil
				})

				Context("should succeed", func() {

					createInterface := func(ctx context.Context, c *vim25.Client) {
						finder := find.NewFinder(c)
						cluster, err := finder.DefaultClusterComputeResource(ctx)
						Expect(err).ToNot(HaveOccurred())
						scheme := runtime.NewScheme()
						_ = clientgoscheme.AddToScheme(scheme)
						_ = netopv1alpha1.AddToScheme(scheme)

						net, err := finder.Network(ctx, "DC0_DVPG0")
						Expect(err).ToNot(HaveOccurred())
						dvpg := simulator.Map.Get(net.Reference()).(*simulator.DistributedVirtualPortgroup)
						dvpg.Config.LogicalSwitchUuid = dummyNsxSwitchId // Convert to an NSX backed PG
						dvpg.Config.BackingType = "nsx"

						np = vsphere.NetOpNetworkProvider(k8sClient, c, finder, cluster, scheme)

						dev, err := np.CreateVnic(ctx, vm, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(dev).NotTo(BeNil())

						nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
						Expect(nic).NotTo(BeNil())
						Expect(nic.ExternalId).To(Equal(interfaceId))
						Expect(nic.MacAddress).To(Equal(macAddress))
						Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))
					}

					It("with no provider IP configuration", func() {
						res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
							createInterface(ctx, c)

							cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
							Expect(err).ToNot(HaveOccurred())
							Expect(cust.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))

							return nil
						})
						Expect(res).To(BeNil())
					})
				})
			})

			Context("expected guest customization", func() {

				Context("no IPConfigs", func() {
					BeforeEach(func() { netIf.Status.IPConfigs = nil })

					It("dhcp customization", func() {
						cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(cust.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
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
						cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(cust.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationFixedIp{}))
						fixedIp := cust.Adapter.Ip.(*types.CustomizationFixedIp)
						Expect(fixedIp.IpAddress).To(Equal(ip))
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
						cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(cust.Adapter.IpV6Spec).To(BeAssignableToTypeOf(&types.CustomizationIPSettingsIpV6AddressSpec{}))
						Expect(len(cust.Adapter.IpV6Spec.Ip)).To(Equal(1))
						fixedIp := cust.Adapter.IpV6Spec.Ip[0].(*types.CustomizationFixedIpV6)
						Expect(fixedIp.IpAddress).To(Equal(ip))
					})
				})
			})
		})
	})

	Context("when using NSX-T network provider", func() {
		var (
			ncpClient clientset.Interface
			ncpVif    *ncpv1alpha1.VirtualNetworkInterface
		)

		BeforeEach(func() {
			ncpClient = ncpfake.NewSimpleClientset()
			np = vsphere.NsxtNetworkProvider(ncpClient, finder, cluster)

			ncpVif = &ncpv1alpha1.VirtualNetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-lsp", vmNif.NetworkName, vm.Name),
					Namespace: dummyNamespace,
				},
				Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
					VirtualNetwork: dummyVirtualNetwork,
				},
				Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
					MacAddress:  macAddress,
					InterfaceID: interfaceId,
					Conditions:  []ncpv1alpha1.VirtualNetworkCondition{{Type: "Ready", Status: "True"}},
					ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: dummyNsxSwitchId,
					},
				},
			}
		})

		// Creates the vnetif and other objects in the system the way we want to test them
		// This runs after all BeforeEach()
		JustBeforeEach(func() {
			_, err := ncpClient.VmwareV1alpha1().VirtualNetworkInterfaces(dummyNamespace).Create(ncpVif)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when creating vnic", func() {

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: "",
					}
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetif '"))
				})
			})

			Context("when interface has no opaque network id defined in the provider status", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetif '"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					ncpVif.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(MatchError("timed out waiting for the condition"))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus.NsxLogicalSwitchID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).To(MatchError(fmt.Sprintf("opaque network with ID '%s' not found", doesNotExist)))
				})
			})

			Context("should succeed", func() {

				createInterface := func(ctx context.Context, c *vim25.Client) {
					finder := find.NewFinder(c)
					cluster, err := finder.DefaultClusterComputeResource(ctx)
					Expect(err).ToNot(HaveOccurred())

					net, err := finder.Network(ctx, "DC0_DVPG0")
					Expect(err).ToNot(HaveOccurred())
					dvpg := simulator.Map.Get(net.Reference()).(*simulator.DistributedVirtualPortgroup)
					dvpg.Config.LogicalSwitchUuid = dummyNsxSwitchId // Convert to an NSX backed PG
					dvpg.Config.BackingType = "nsx"

					np := vsphere.NsxtNetworkProvider(ncpClient, finder, cluster)

					dev, err := np.CreateVnic(ctx, vm, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(dev).NotTo(BeNil())

					nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceId))
					Expect(nic.MacAddress).To(Equal(macAddress))
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))
				}

				It("with no provider IP configuration", func() {
					res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
						createInterface(ctx, c)

						cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(cust.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))

						return nil
					})
					Expect(res).To(BeNil())
				})

				Context("with provider IP configuration", func() {
					ip := "192.168.100.10"

					BeforeEach(func() {
						ncpVif.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
							{
								IP: ip,
							},
						}
					})

					It("should work", func() {
						res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
							createInterface(ctx, c)

							cust, err := np.GetInterfaceGuestCustomization(vm, vmNif)
							Expect(err).ToNot(HaveOccurred())
							fixedIp := cust.Adapter.Ip.(*types.CustomizationFixedIp)
							Expect(fixedIp.IpAddress).To(Equal(ip))
							return nil
						})
						Expect(res).To(BeNil())
					})
				})
			})

			It("should update owner reference if already exist", func() {
				otherVmWithDifferentUid := &v1alpha1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						UID:       "another-uid",
					},
					Spec: v1alpha1.VirtualMachineSpec{
						NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
							*vmNif,
						},
					},
				}
				// TODO: we can't test the owner reference is correct
				// But this test exercises the path that there is an existing network interface owned by a different VM
				// and this interface should be updated with the new VM.
				_, err := np.CreateVnic(ctx, otherVmWithDifferentUid, vmNif)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
