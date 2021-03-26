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
	"k8s.io/apimachinery/pkg/util/wait"
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

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
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
		ctx       context.Context
		name      string
		namespace string

		c       *govmomi.Client
		finder  *find.Finder
		cluster *object.ClusterComputeResource
		network object.NetworkReference

		vmNif *v1alpha1.VirtualMachineNetworkInterface
		vm    *v1alpha1.VirtualMachine
		vmCtx vsphere.VMContext

		np vsphere.NetworkProvider
	)

	BeforeEach(func() {
		var err error
		ctx = context.TODO()
		name = dummyObjectName
		namespace = dummyNamespace

		c, err = govmomi.NewClient(ctx, server.URL, true)
		Expect(err).ToNot(HaveOccurred())
		finder = find.NewFinder(c.Client)
		dc, err := finder.DefaultDatacenter(ctx)
		Expect(err).ToNot(HaveOccurred())
		finder.SetDatacenter(dc)

		cluster, err = finder.DefaultClusterComputeResource(ctx)
		Expect(err).ToNot(HaveOccurred())

		network, err = finder.Network(ctx, vcsimNetworkName)
		Expect(err).ToNot(HaveOccurred())

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

		vmCtx = vsphere.VMContext{
			Context: ctx,
			Logger:  integration.Log,
			VM:      vm,
		}
	})

	Context("Named Network Provider", func() {
		BeforeEach(func() {
			np = vsphere.NewNetworkProvider(nil, nil, finder, nil, nil)
		})

		Context("ensure interface", func() {

			It("create expected virtual device", func() {
				info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
				Expect(err).ToNot(HaveOccurred())
				Expect(info).NotTo(BeNil())

				Expect(info.Device).NotTo(BeNil())
				backing := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard().Backing
				Expect(backing).NotTo(BeNil())
				backingInfo, ok := backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(backingInfo.Port.PortgroupKey).To(Equal(network.Reference().Value))
			})

			It("create expected interface customization", func() {
				info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Customization).ToNot(BeNil())
				Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
			})

			It("should return an error if network does not exist", func() {
				_, err := np.EnsureNetworkInterface(vmCtx, &v1alpha1.VirtualMachineNetworkInterface{
					NetworkName: doesNotExist,
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("unable to find network \"%s\": network '%s' not found", doesNotExist, doesNotExist)))
			})
		})
	})

	Context("NetOP Network Provider", func() {
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
			_ = v1alpha1.AddToScheme(scheme)
			_ = netopv1alpha1.AddToScheme(scheme)

			k8sClient = clientfake.NewFakeClientWithScheme(scheme, netIf)
			np = vsphere.NewNetworkProvider(k8sClient, c.Client, finder, cluster, scheme)
		})

		Context("ensure interface", func() {

			// Long test due to poll timeout.
			It("create netop network interface object", func() {
				Expect(k8sClient.Delete(ctx, netIf)).To(Succeed())

				_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(wait.ErrWaitTimeout))

				instance := &netopv1alpha1.NetworkInterface{}
				err = k8sClient.Get(ctx, ctrlruntime.ObjectKey{Name: netIf.Name, Namespace: netIf.Namespace}, instance)
				Expect(err).ToNot(HaveOccurred())
				Expect(instance.Spec.NetworkName).To(Equal(vcsimNetworkName))

				Expect(instance.OwnerReferences).To(HaveLen(1))
				Expect(instance.OwnerReferences[0].Name).To(Equal(vm.Name))
			})

			Context("when interface has no provider status defined", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = ""
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("unable to get ethernet card backing info for network DistributedVirtualPortgroup::"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					netIf.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).To(MatchError("timed out waiting for the condition"))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					netIf.Status.NetworkID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to get ethernet card backing info for network DistributedVirtualPortgroup:" + doesNotExist))
				})
			})

			It("should succeed", func() {
				info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
				Expect(err).ToNot(HaveOccurred())
				Expect(info).NotTo(BeNil())

				Expect(info.Device).ToNot(BeNil())
				nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
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
					info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					Expect(info.Device).ToNot(BeNil())
					nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
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
					info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					Expect(info.Device).ToNot(BeNil())
					nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
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
						_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("networkinterfaces.netoperator.vmware.com \"does-not-exist\" not found"))
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
						_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("unsupported NetworkInterface ProviderRef"))
					})
				})
			})

			Context("with NSX-T NetworkType in ProviderRef", func() {
				BeforeEach(func() {
					vmNif.NetworkType = vsphere.NsxtNetworkType
					netIf.Name = dummyNetIfName
					netIf.Status.NetworkID = dummyNsxSwitchId
					netIf.Status.IPConfigs = nil

					vmNif.ProviderRef = &v1alpha1.NetworkInterfaceProviderReference{
						APIGroup:   "netoperator.vmware.com",
						APIVersion: "v1alpha1",
						Kind:       "NetworkInterface",
						Name:       dummyNetIfName,
					}
					vm.Spec.NetworkInterfaces[0] = *vmNif
				})

				Context("should succeed", func() {

					createInterface := func(ctx context.Context, c *vim25.Client) {
						finder := find.NewFinder(c)
						cluster, err := finder.DefaultClusterComputeResource(ctx)
						Expect(err).ToNot(HaveOccurred())
						scheme := runtime.NewScheme()
						_ = clientgoscheme.AddToScheme(scheme)
						_ = v1alpha1.AddToScheme(scheme)
						_ = netopv1alpha1.AddToScheme(scheme)

						net, err := finder.Network(ctx, "DC0_DVPG0")
						Expect(err).ToNot(HaveOccurred())
						dvpg := simulator.Map.Get(net.Reference()).(*simulator.DistributedVirtualPortgroup)
						dvpg.Config.LogicalSwitchUuid = dummyNsxSwitchId // Convert to an NSX backed PG
						dvpg.Config.BackingType = "nsx"

						np = vsphere.NewNetworkProvider(k8sClient, c, finder, cluster, scheme)

						info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(info).NotTo(BeNil())

						nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
						Expect(nic).NotTo(BeNil())
						Expect(nic.ExternalId).To(Equal(interfaceId))
						Expect(nic.MacAddress).To(Equal(macAddress))
						Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))
					}

					It("with no provider IP configuration", func() {
						res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
							createInterface(ctx, c)

							info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
							Expect(err).ToNot(HaveOccurred())
							Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))

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
						info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
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
						info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationFixedIp{}))
						fixedIp := info.Customization.Adapter.Ip.(*types.CustomizationFixedIp)
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
						info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.IpV6Spec).To(BeAssignableToTypeOf(&types.CustomizationIPSettingsIpV6AddressSpec{}))
						Expect(info.Customization.Adapter.IpV6Spec.Ip).To(HaveLen(1))
						fixedIp := info.Customization.Adapter.IpV6Spec.Ip[0].(*types.CustomizationFixedIpV6)
						Expect(fixedIp.IpAddress).To(Equal(ip))
					})
				})
			})
		})
	})

	Context("NSX-T Network Provider", func() {
		var (
			k8sClient ctrlruntime.Client
			ncpVif    *ncpv1alpha1.VirtualNetworkInterface
			scheme    *runtime.Scheme
		)

		BeforeEach(func() {
			vmNif.NetworkType = vsphere.NsxtNetworkType
			vm.Spec.NetworkInterfaces[0].NetworkType = vsphere.NsxtNetworkType

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

		JustBeforeEach(func() {
			scheme = runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			_ = v1alpha1.AddToScheme(scheme)
			_ = ncpv1alpha1.AddToScheme(scheme)

			k8sClient = clientfake.NewFakeClientWithScheme(scheme, ncpVif)
			np = vsphere.NewNetworkProvider(k8sClient, c.Client, finder, cluster, scheme)
		})

		Context("ensure interface", func() {

			// Long test due to poll timeout.
			It("create ncp virtual network interface object", func() {
				Expect(k8sClient.Delete(ctx, ncpVif)).To(Succeed())

				_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(wait.ErrWaitTimeout))

				instance := &ncpv1alpha1.VirtualNetworkInterface{}
				err = k8sClient.Get(ctx, ctrlruntime.ObjectKey{Name: ncpVif.Name, Namespace: ncpVif.Namespace}, instance)
				Expect(err).ToNot(HaveOccurred())
				Expect(instance.Spec.VirtualNetwork).To(Equal(vcsimNetworkName))

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
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetIf '"))
				})
			})

			Context("when interface has no opaque network id defined in the provider status", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).NotTo(BeNil())
					Expect(err.Error()).To(ContainSubstring("failed to get for nsx-t opaque network ID for vnetIf '"))
				})
			})

			Context("when the interface is not ready", func() {
				BeforeEach(func() {
					ncpVif.Status.Conditions = nil
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).To(MatchError("timed out waiting for the condition"))
				})
			})

			Context("when the referenced network is not found", func() {
				BeforeEach(func() {
					ncpVif.Status.ProviderStatus.NsxLogicalSwitchID = doesNotExist
				})

				It("should return an error", func() {
					_, err := np.EnsureNetworkInterface(vmCtx, vmNif)
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

					np = vsphere.NewNetworkProvider(k8sClient, c, finder, cluster, scheme)

					info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
					Expect(err).ToNot(HaveOccurred())
					Expect(info).NotTo(BeNil())

					nic := info.Device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
					Expect(nic).NotTo(BeNil())
					Expect(nic.ExternalId).To(Equal(interfaceId))
					Expect(nic.MacAddress).To(Equal(macAddress))
					Expect(nic.AddressType).To(Equal(string(types.VirtualEthernetCardMacTypeManual)))
				}

				It("with no provider IP configuration", func() {
					res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
						createInterface(ctx, c)

						info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
						Expect(err).ToNot(HaveOccurred())
						Expect(info.Customization.Adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))

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

							info, err := np.EnsureNetworkInterface(vmCtx, vmNif)
							Expect(err).ToNot(HaveOccurred())
							fixedIp := info.Customization.Adapter.Ip.(*types.CustomizationFixedIp)
							Expect(fixedIp.IpAddress).To(Equal(ip))
							return nil
						})
						Expect(res).To(BeNil())
					})
				})
			})
		})
	})
})
