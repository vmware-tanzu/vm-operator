// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CreateAndWaitForNetworkInterfaces", Label(testlabels.VCSim), func() {

	const (
		macAddress = "01:02:03:04:05:06"
	)

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim

		vmCtx       pkgctx.VirtualMachineContext
		vm          *vmopv1.VirtualMachine
		networkSpec *vmopv1.VirtualMachineNetworkSpec

		results     network.NetworkInterfaceResults
		err         error
		initObjects []client.Object
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "network-test-vm",
				Namespace: "network-test-ns",
				UID:       "network-test-uid",
			},
		}

		networkSpec = &vmopv1.VirtualMachineNetworkSpec{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithName("network_test"),
			VM:      vm,
		}

		results, err = network.CreateAndWaitForNetworkInterfaces(
			vmCtx,
			ctx.Client,
			ctx.VCClient.Client,
			ctx.Finder,
			nil,
			networkSpec)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Context("Named Network", func() {
		// Use network vcsim automatically creates.
		const networkName = "DC0_DVPG0"

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNamed
		})

		Context("network exists", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name:    "eth0",
						Network: &common.PartialObjectRef{Name: networkName},
						DHCP6:   true,
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(results.Results).To(HaveLen(1))

				result := results.Results[0]
				By("has expected backing", func() {
					Expect(result.Backing).ToNot(BeNil())
					backing, err := result.Backing.EthernetCardBackingInfo(ctx)
					Expect(err).ToNot(HaveOccurred())
					backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).To(BeTrue())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
				})

				Expect(result.DHCP4).To(BeTrue())
				Expect(result.DHCP6).To(BeTrue()) // Only enabled if explicitly requested (which it is above).
			})

			Context("Overrides with provided InterfaceSpec", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:            "my-network-interface",
							GuestDeviceName: "eth42",
							Network:         &common.PartialObjectRef{Name: networkName},
							MACAddr:         macAddress,
							Addresses: []string{
								"172.42.1.100/24",
								"fd1a:6c85:79fe:7c98:0000:0000:0000:000f/56",
							},
							Gateway4:    "172.42.1.1",
							Gateway6:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
							MTU:         ptr.To[int64](9000),
							Nameservers: []string{"9.9.9.9"},
							Routes: []vmopv1.VirtualMachineNetworkRouteSpec{
								{
									To:     "10.10.10.10",
									Via:    "5.5.5.5",
									Metric: 42,
								},
								{
									To:  "default",
									Via: "1.2.3.4",
								},
							},
							SearchDomains: []string{"vmware.com"},
						},
					}
				})

				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(results.Results).To(HaveLen(1))

					result := results.Results[0]
					By("has expected backing", func() {
						Expect(result.Backing).ToNot(BeNil())
						backing, err := result.Backing.EthernetCardBackingInfo(ctx)
						Expect(err).ToNot(HaveOccurred())
						backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
						Expect(ok).To(BeTrue())
						Expect(backingInfo.Port.PortgroupKey).To(Equal(ctx.NetworkRef.Reference().Value))
					})

					By("has expected names", func() {
						Expect(result.Name).To(Equal("my-network-interface"))
						Expect(result.GuestDeviceName).To(Equal("eth42"))
					})

					Expect(result.MacAddress).To(Equal(macAddress))
					Expect(result.DHCP4).To(BeFalse())
					Expect(result.DHCP6).To(BeFalse())

					By("has expected address", func() {
						Expect(result.IPConfigs).To(HaveLen(2))
						ipConfig := result.IPConfigs[0]
						Expect(ipConfig.IPCIDR).To(Equal("172.42.1.100/24"))
						Expect(ipConfig.IsIPv4).To(BeTrue())
						Expect(ipConfig.Gateway).To(Equal("172.42.1.1"))
						ipConfig = result.IPConfigs[1]
						Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:000f/56"))
						Expect(ipConfig.IsIPv4).To(BeFalse())
						Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
					})

					Expect(result.MTU).To(BeEquivalentTo(9000))
					Expect(result.Nameservers).To(HaveExactElements("9.9.9.9"))
					Expect(result.SearchDomains).To(HaveExactElements("vmware.com"))
					Expect(result.Routes).To(HaveLen(2))
					Expect(result.Routes[0].To).To(Equal("10.10.10.10"))
					Expect(result.Routes[0].Via).To(Equal("5.5.5.5"))
					Expect(result.Routes[0].Metric).To(BeEquivalentTo(42))
					Expect(result.Routes[1].To).To(Equal("default"))
					Expect(result.Routes[1].Via).To(Equal("1.2.3.4"))
					Expect(result.Routes[1].Metric).To(BeEquivalentTo(0))
				})

				Context("Gateway4/6 are disabled", func() {
					BeforeEach(func() {
						networkSpec.Interfaces[0].Gateway4 = "None"
						networkSpec.Interfaces[0].Gateway6 = "None"
					})

					It("returns success", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(results.Results).To(HaveLen(1))

						result := results.Results[0]

						By("has no gateways", func() {
							Expect(result.IPConfigs).To(HaveLen(2))
							ipConfig := result.IPConfigs[0]
							Expect(ipConfig.IPCIDR).To(Equal("172.42.1.100/24"))
							Expect(ipConfig.IsIPv4).To(BeTrue())
							Expect(ipConfig.Gateway).To(BeEmpty())
							ipConfig = result.IPConfigs[1]
							Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:000f/56"))
							Expect(ipConfig.IsIPv4).To(BeFalse())
							Expect(ipConfig.Gateway).To(BeEmpty())
						})
					})
				})

				Context("Bootstrap has use globals as defaults", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								UseGlobalNameserversAsDefault:   ptr.To(true),
								UseGlobalSearchDomainsAsDefault: ptr.To(true),
							},
						}

						networkSpec.Nameservers = []string{"149.112.112.112"}
						networkSpec.SearchDomains = []string{"broadcom.net"}
						networkSpec.Interfaces[0].Nameservers = nil
						networkSpec.Interfaces[0].SearchDomains = nil
					})

					It("returns success", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(results.Results).To(HaveLen(1))

						result := results.Results[0]
						By("has expected values", func() {
							Expect(result.Nameservers).To(HaveExactElements("149.112.112.112"))
							Expect(result.SearchDomains).To(HaveExactElements("broadcom.net"))
						})
					})
				})
			})
		})

		Context("network does not exist", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name:    "eth0",
						Network: &common.PartialObjectRef{Name: "bogus"},
					},
				}
			})

			It("returns error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to find named network"))
				Expect(results.Results).To(BeEmpty())
			})
		})
	})

	Context("VDS", func() {
		const (
			interfaceName = "eth0"
			networkName   = "my-vds-network"
		)

		BeforeEach(func() {
			network.RetryTimeout = 1 * time.Second
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.NetworkRef.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
				Expect(results.Results).To(BeEmpty())

				var externalID string
				By("simulate successful NetOP reconcile", func() {
					netInterface := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
					Expect(metav1.IsControlledBy(netInterface, vm)).To(BeTrue())
					Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
					Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName))
					Expect(netInterface.Spec.NetworkName).To(Equal(networkName))

					externalID = netInterface.Spec.ExternalID
					// Expect(externalID).ToNot(BeEmpty())
					Expect(externalID).To(BeEmpty())

					netInterface.Status.ExternalID = externalID
					netInterface.Status.NetworkID = ctx.NetworkRef.Reference().Value
					netInterface.Status.MacAddress = "" // NetOP doesn't set this.
					netInterface.Status.IPConfigs = []netopv1alpha1.IPConfig{
						{
							IP:         "192.168.1.110",
							IPFamily:   corev1.IPv4Protocol,
							Gateway:    "192.168.1.1",
							SubnetMask: "255.255.255.0",
						},
						{
							IP:         "fd1a:6c85:79fe:7c98:0000:0000:0000:000f",
							IPFamily:   corev1.IPv6Protocol,
							Gateway:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
							SubnetMask: "ffff:ffff:ffff:ff00:0000:0000:0000:0000",
						},
					}
					netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
						{
							Type:   netopv1alpha1.NetworkInterfaceReady,
							Status: corev1.ConditionTrue,
						},
					}
					Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
				})

				results, err = network.CreateAndWaitForNetworkInterfaces(
					vmCtx,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder,
					nil,
					networkSpec)
				Expect(err).ToNot(HaveOccurred())

				Expect(results.Results).To(HaveLen(1))
				result := results.Results[0]
				Expect(result.MacAddress).To(BeEmpty())
				Expect(result.ExternalID).To(Equal(externalID))
				Expect(result.NetworkID).To(Equal(ctx.NetworkRef.Reference().Value))
				Expect(result.Backing).ToNot(BeNil())
				Expect(result.Backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
				Expect(result.Name).To(Equal(interfaceName))
				Expect(result.GuestDeviceName).To(Equal(interfaceName))

				Expect(result.IPConfigs).To(HaveLen(2))
				ipConfig := result.IPConfigs[0]
				Expect(ipConfig.IPCIDR).To(Equal("192.168.1.110/24"))
				Expect(ipConfig.IsIPv4).To(BeTrue())
				Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
				ipConfig = result.IPConfigs[1]
				Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
				Expect(ipConfig.IsIPv4).To(BeFalse())
				Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
			})

			When("interfaceSpec provides MAC address", func() {
				BeforeEach(func() {
					networkSpec.Interfaces[0].MACAddr = macAddress
				})

				It("returns success with expected mac address", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NetOP reconcile", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
							{
								Type:   netopv1alpha1.NetworkInterfaceReady,
								Status: corev1.ConditionTrue,
							},
						}
						Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
					})

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					result := results.Results[0]
					Expect(result.MacAddress).To(Equal(macAddress))
				})
			})

			When("v1a1 network interface exists", func() {
				BeforeEach(func() {
					netIf := &netopv1alpha1.NetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, true),
							Namespace: vm.Namespace,
						},
						Spec: netopv1alpha1.NetworkInterfaceSpec{
							NetworkName: networkName,
							Type:        netopv1alpha1.NetworkInterfaceTypeVMXNet3,
						},
					}

					initObjects = append(initObjects, netIf)
				})

				It("returns success", func() {
					// Assert test env is what we expect.
					Expect(ctx.NetworkRef.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NetOP reconcile", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, true),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(metav1.IsControlledBy(netInterface, vm)).To(BeTrue())
						Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
						Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName))
						Expect(netInterface.Spec.NetworkName).To(Equal(networkName))

						netInterface.Status.NetworkID = ctx.NetworkRef.Reference().Value
						netInterface.Status.MacAddress = "" // NetOP doesn't set this.
						netInterface.Status.IPConfigs = []netopv1alpha1.IPConfig{
							{
								IP:         "192.168.1.110",
								IPFamily:   corev1.IPv4Protocol,
								Gateway:    "192.168.1.1",
								SubnetMask: "255.255.255.0",
							},
							{
								IP:         "fd1a:6c85:79fe:7c98:0000:0000:0000:000f",
								IPFamily:   corev1.IPv6Protocol,
								Gateway:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
								SubnetMask: "ffff:ffff:ffff:ff00:0000:0000:0000:0000",
							},
						}
						netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
							{
								Type:   netopv1alpha1.NetworkInterfaceReady,
								Status: corev1.ConditionTrue,
							},
						}
						Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
					})

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					result := results.Results[0]
					Expect(result.MacAddress).To(BeEmpty())
					Expect(result.ExternalID).To(BeEmpty())
					Expect(result.NetworkID).To(Equal(ctx.NetworkRef.Reference().Value))
					Expect(result.Backing).ToNot(BeNil())
					Expect(result.Backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
					Expect(result.Name).To(Equal(interfaceName))
					Expect(result.GuestDeviceName).To(Equal(interfaceName))

					Expect(result.IPConfigs).To(HaveLen(2))
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal("192.168.1.110/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[1]
					Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
					Expect(ipConfig.IsIPv4).To(BeFalse())
					Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
				})
			})
		})
	})

	Context("NCP", func() {
		const (
			interfaceName = "eth0"
			interfaceID   = "my-interface-id"
			networkName   = "my-ncp-network"
		)

		BeforeEach(func() {
			network.RetryTimeout = 1 * time.Second
			testConfig.WithNetworkEnv = builder.NetworkEnvNSXT
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.NetworkRef.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
				Expect(results.Results).To(BeEmpty())

				By("simulate successful NCP reconcile", func() {
					netInterface := &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NCPCRName(vm.Name, networkName, interfaceName, false),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
					Expect(metav1.IsControlledBy(netInterface, vm)).To(BeTrue())
					Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
					Expect(netInterface.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName))
					Expect(netInterface.Spec.VirtualNetwork).To(Equal(networkName))

					netInterface.Status.InterfaceID = interfaceID
					netInterface.Status.MacAddress = macAddress
					netInterface.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
						NsxLogicalSwitchID: builder.GetNsxTLogicalSwitchUUID(0),
					}
					netInterface.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
						{
							IP:         "192.168.1.110",
							Gateway:    "192.168.1.1",
							SubnetMask: "255.255.255.0",
						},
						{
							IP:         "fd1a:6c85:79fe:7c98:0000:0000:0000:000f",
							Gateway:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
							SubnetMask: "ffff:ffff:ffff:ff00:0000:0000:0000:0000",
						},
					}
					netInterface.Status.Conditions = []ncpv1alpha1.VirtualNetworkCondition{
						{
							Type:   "Ready",
							Status: "True",
						},
					}
					Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
				})

				results, err = network.CreateAndWaitForNetworkInterfaces(
					vmCtx,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder,
					nil,
					networkSpec)
				Expect(err).ToNot(HaveOccurred())

				Expect(results.Results).To(HaveLen(1))
				result := results.Results[0]
				Expect(result.MacAddress).To(Equal(macAddress))
				Expect(result.ExternalID).To(Equal(interfaceID))
				Expect(result.NetworkID).To(Equal(builder.GetNsxTLogicalSwitchUUID(0)))
				Expect(result.Name).To(Equal(interfaceName))

				Expect(result.Backing).ToNot(BeNil())
				backing, err := result.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(builder.NsxTLogicalSwitchUUID))

				Expect(result.IPConfigs).To(HaveLen(2))
				ipConfig := result.IPConfigs[0]
				Expect(ipConfig.IPCIDR).To(Equal("192.168.1.110/24"))
				Expect(ipConfig.IsIPv4).To(BeTrue())
				Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
				ipConfig = result.IPConfigs[1]
				Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
				Expect(ipConfig.IsIPv4).To(BeFalse())
				Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))

				By("Returns DVPG backing when CCR is provided", func() {
					clusterMoRef := ctx.GetFirstClusterFromFirstZone().Reference()
					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						&clusterMoRef,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					Expect(results.Results[0].Backing).ToNot(BeNil())
					Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
				})
			})

			When("v1a1 NCP network interface exists", func() {
				BeforeEach(func() {
					vnetIf := &ncpv1alpha1.VirtualNetworkInterface{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.NCPCRName(vm.Name, networkName, interfaceName, true),
							Namespace: vm.Namespace,
						},
						Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
							VirtualNetwork: networkName,
						},
					}

					initObjects = append(initObjects, vnetIf)
				})

				It("returns success", func() {
					// Assert test env is what we expect.
					Expect(ctx.NetworkRef.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NCP reconcile", func() {
						netInterface := &ncpv1alpha1.VirtualNetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NCPCRName(vm.Name, networkName, interfaceName, true),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(metav1.IsControlledBy(netInterface, vm)).To(BeTrue())
						Expect(netInterface.Spec.VirtualNetwork).To(Equal(networkName))

						netInterface.Status.InterfaceID = interfaceID
						netInterface.Status.MacAddress = macAddress
						netInterface.Status.ProviderStatus = &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
							NsxLogicalSwitchID: builder.GetNsxTLogicalSwitchUUID(0),
						}
						netInterface.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
							{
								IP:         "192.168.1.110",
								Gateway:    "192.168.1.1",
								SubnetMask: "255.255.255.0",
							},
							{
								IP:         "fd1a:6c85:79fe:7c98:0000:0000:0000:000f",
								Gateway:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
								SubnetMask: "ffff:ffff:ffff:ff00:0000:0000:0000:0000",
							},
						}
						netInterface.Status.Conditions = []ncpv1alpha1.VirtualNetworkCondition{
							{
								Type:   "Ready",
								Status: "True",
							},
						}
						Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
					})

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					result := results.Results[0]
					Expect(result.MacAddress).To(Equal(macAddress))
					Expect(result.ExternalID).To(Equal(interfaceID))
					Expect(result.NetworkID).To(Equal(builder.GetNsxTLogicalSwitchUUID(0)))
					Expect(result.Name).To(Equal(interfaceName))

					Expect(result.Backing).ToNot(BeNil())
					backing, err := result.Backing.EthernetCardBackingInfo(ctx)
					Expect(err).ToNot(HaveOccurred())
					opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
					Expect(ok).To(BeTrue())
					Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(builder.NsxTLogicalSwitchUUID))

					Expect(result.IPConfigs).To(HaveLen(2))
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal("192.168.1.110/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[1]
					Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
					Expect(ipConfig.IsIPv4).To(BeFalse())
					Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))

					By("Returns DVPG backing when CCR is provided", func() {
						clusterMoRef := ctx.GetFirstClusterFromFirstZone().Reference()
						results, err = network.CreateAndWaitForNetworkInterfaces(
							vmCtx,
							ctx.Client,
							ctx.VCClient.Client,
							ctx.Finder,
							&clusterMoRef,
							networkSpec)
						Expect(err).ToNot(HaveOccurred())

						Expect(results.Results).To(HaveLen(1))
						Expect(results.Results[0].Backing).ToNot(BeNil())
						Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
					})
				})
			})
		})
	})

	Context("VPC", func() {
		const (
			interfaceName = "eth0"
			interfaceID   = "my-interface-id"
			networkName   = "my-vpc-network"
			macAddress    = "01-23-45-67-89-ab-cd-ef"
		)

		BeforeEach(func() {
			network.RetryTimeout = 1 * time.Second
			testConfig.WithNetworkEnv = builder.NetworkEnvVPC
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							Name: networkName,
							TypeMeta: metav1.TypeMeta{
								Kind:       "SubnetSet",
								APIVersion: "crd.nsx.vmware.com/v1alpha1",
							},
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.NetworkRef.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
				Expect(results.Results).To(BeEmpty())

				By("simulate successful NSX Operator reconcile", func() {
					subnetPort := &vpcv1alpha1.SubnetPort{
						ObjectMeta: metav1.ObjectMeta{
							Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
					Expect(metav1.IsControlledBy(subnetPort, vm)).To(BeTrue())
					Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
					Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName))
					annotationVal := "virtualmachine/" + vm.Name + "/" + interfaceName
					Expect(subnetPort.Annotations).To(HaveKeyWithValue(constants.VPCAttachmentRef, annotationVal))
					Expect(subnetPort.Spec.SubnetSet).To(Equal(networkName))
					Expect(subnetPort.Spec.Subnet).To(BeEmpty())

					subnetPort.Status.Attachment.ID = interfaceID
					subnetPort.Status.NetworkInterfaceConfig.MACAddress = macAddress
					subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = builder.GetVPCTLogicalSwitchUUID(0)
					subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
						{
							IPAddress: "192.168.1.110/24",
							Gateway:   "192.168.1.1",
						},
						{
							IPAddress: "fd1a:6c85:79fe:7c98::f/56",
							Gateway:   "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
						},
					}
					subnetPort.Status.Conditions = []vpcv1alpha1.Condition{
						{
							Type:   vpcv1alpha1.Ready,
							Status: corev1.ConditionTrue,
						},
					}
					Expect(ctx.Client.Status().Update(ctx, subnetPort)).To(Succeed())
				})

				results, err = network.CreateAndWaitForNetworkInterfaces(
					vmCtx,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder,
					nil,
					networkSpec)
				Expect(err).ToNot(HaveOccurred())

				Expect(results.Results).To(HaveLen(1))
				result := results.Results[0]
				Expect(result.MacAddress).To(Equal(macAddress))
				Expect(result.ExternalID).To(Equal(interfaceID))
				Expect(result.NetworkID).To(Equal(builder.GetVPCTLogicalSwitchUUID(0)))
				Expect(result.Name).To(Equal(interfaceName))

				Expect(result.Backing).ToNot(BeNil())
				backing, err := result.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(builder.VPCLogicalSwitchUUID))

				Expect(result.IPConfigs).To(HaveLen(2))
				ipConfig := result.IPConfigs[0]
				Expect(ipConfig.IPCIDR).To(Equal("192.168.1.110/24"))
				Expect(ipConfig.IsIPv4).To(BeTrue())
				Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
				ipConfig = result.IPConfigs[1]
				Expect(ipConfig.IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
				Expect(ipConfig.IsIPv4).To(BeFalse())
				Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))

				By("Returns DVPG backing when CCR is provided", func() {
					clusterMoRef := ctx.GetFirstClusterFromFirstZone().Reference()
					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						&clusterMoRef,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					Expect(results.Results[0].Backing).ToNot(BeNil())
					Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
				})
			})

			When("interfaceSpec provides IP/MAC address", func() {
				const ipAddr = "192.168.1.220"
				const ipCIDR = ipAddr + "/24"

				BeforeEach(func() {
					networkSpec.Interfaces[0].MACAddr = macAddress
					networkSpec.Interfaces[0].Addresses = []string{ipCIDR}
				})

				It("returns success with expected MAC address", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("network interface is not ready yet"))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NSX Operator reconcile", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())

						Expect(subnetPort.Spec.AddressBindings).To(HaveLen(1))
						Expect(subnetPort.Spec.AddressBindings[0].IPAddress).To(Equal(ipAddr))
						Expect(subnetPort.Spec.AddressBindings[0].MACAddress).To(Equal(macAddress))

						subnetPort.Status.Attachment.ID = interfaceID
						subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = builder.GetVPCTLogicalSwitchUUID(0)
						subnetPort.Status.NetworkInterfaceConfig.MACAddress = macAddress
						subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
							{
								IPAddress: ipCIDR,
								Gateway:   "192.168.1.1",
							},
						}
						subnetPort.Status.Conditions = []vpcv1alpha1.Condition{
							{
								Type:   vpcv1alpha1.Ready,
								Status: corev1.ConditionTrue,
							},
						}
						Expect(ctx.Client.Status().Update(ctx, subnetPort)).To(Succeed())
					})

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())

					Expect(results.Results).To(HaveLen(1))
					result := results.Results[0]
					Expect(result.MacAddress).To(Equal(macAddress))
					Expect(result.IPConfigs).To(HaveLen(1))
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal(ipCIDR))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
				})
			})
		})
	})
})

var _ = Describe("SetNetworkInterfaceOwnerRef", func() {
	var (
		vm           *vmopv1.VirtualMachine
		netInterface *netopv1alpha1.NetworkInterface
		client       client.Client
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-vm",
				Namespace: "my-ns",
				UID:       "my-vm-uid",
			},
		}
		netInterface = &netopv1alpha1.NetworkInterface{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-interface",
				Namespace: vm.Namespace,
			},
		}

		client = builder.NewFakeClient()
	})

	When("Network interface has VM owner ref", func() {
		It("owner ref gets upgraded to controller ref", func() {
			Expect(controllerutil.SetOwnerReference(vm, netInterface, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netInterface, client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			Expect(netInterface.OwnerReferences).To(HaveLen(1))
			Expect(metav1.IsControlledBy(netInterface, vm)).To(BeTrue())
		})
	})

	When("Network interface already has a controller ref", func() {
		It("VM is added as owner ref", func() {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: vm.Namespace, UID: "my-cm-uid"}}
			Expect(controllerutil.SetControllerReference(cm, netInterface, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netInterface, client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			Expect(netInterface.OwnerReferences).To(HaveLen(2))
			Expect(netInterface.OwnerReferences[0].Name).To(Equal(cm.Name))
			Expect(netInterface.OwnerReferences[1].Name).To(Equal(vm.Name))
			Expect(metav1.IsControlledBy(netInterface, cm)).To(BeTrue())
		})
	})

	When("Network interface already has a controller ref that is a VM", func() {
		It("New VM is not added as owner ref", func() {
			ownerVM := &vmopv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "my-vm-1", Namespace: vm.Namespace}}
			Expect(controllerutil.SetControllerReference(ownerVM, netInterface, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netInterface, client.Scheme())
			Expect(err).To(HaveOccurred())
			Expect(netInterface.OwnerReferences).To(HaveLen(1))
			Expect(metav1.IsControlledBy(netInterface, ownerVM)).To(BeTrue())
		})
	})
})
