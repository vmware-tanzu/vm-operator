// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var (
	netOPNetworkTypeMeta = metav1.TypeMeta{
		APIVersion: netopv1alpha1.SchemeGroupVersion.String(),
		Kind:       "Network",
	}
	nsxtNetworkTypeMeta = metav1.TypeMeta{
		APIVersion: ncpv1alpha1.SchemeGroupVersion.String(),
		Kind:       "VirtualNetwork",
	}
	vpcNetworkTypeMeta = metav1.TypeMeta{
		APIVersion: vpcv1alpha1.SchemeGroupVersion.String(),
		Kind:       "SubnetSet",
	}
)

var _ = Describe("CreateAndWaitForNetworkInterfaces", Label(testlabels.VCSim), func() {

	const (
		macAddress = "01:02:03:04:05:06"
	)

	var (
		testConfig builder.VCSimTestConfig
		parentCtx  context.Context
		ctx        *builder.TestContextForVCSim

		vmCtx       pkgctx.VirtualMachineContext
		vm          *vmopv1.VirtualMachine
		networkSpec *vmopv1.VirtualMachineNetworkSpec

		results     network.NetworkInterfaceResults
		err         error
		initObjects []client.Object
	)

	BeforeEach(func() {
		network.RetryTimeout = 1 * time.Millisecond

		testConfig = builder.VCSimTestConfig{}
		parentCtx = pkgcfg.NewContextWithDefaultConfig()

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "network-test-vm",
				Namespace: "network-test-ns",
				UID:       "network-test-uid",
			},
		}

		networkSpec = &vmopv1.VirtualMachineNetworkSpec{}
		vm.Spec.Network = networkSpec
	})

	JustBeforeEach(func() {
		ctx = builder.NewTestContextForVCSim(parentCtx, testConfig, initObjects...)

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
			testConfig.NumNetworks = 0
			testConfig.WithNetworkEnv = builder.NetworkEnvNamed
		})

		Context("network exists", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name:    "eth0",
						Network: &common.PartialObjectRef{Name: networkName},
						DHCP6:   ptr.To(true),
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
					dvpg, err := ctx.Finder.Network(ctx, networkName)
					Expect(err).ToNot(HaveOccurred())
					Expect(backingInfo.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
				})

				Expect(result.DHCP4).To(BeFalse())
				Expect(result.NoIPAM).To(BeFalse())
				Expect(result.DHCP6).To(BeTrue()) // Only enabled if explicitly requested (which it is above).
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
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							TypeMeta: netOPNetworkTypeMeta,
							Name:     networkName,
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.GetNetwork(0).Backing.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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
					netInterface.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
					netInterface.Status.MacAddress = "" // NetOP doesn't set this.
					netInterface.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
					netInterface.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
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
				Expect(result.ObjectProviderType).To(Equal(pkgcfg.NetworkProviderTypeVDS))
				Expect(result.MacAddress).To(BeEmpty())
				Expect(result.ExternalID).To(Equal(externalID))
				Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
				Expect(result.Backing).ToNot(BeNil())
				Expect(result.Backing.Reference()).To(Equal(ctx.GetNetwork(0).Backing.Reference()))
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
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NetOP reconcile", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						// No NetworkInterface Spec.MacAddress field to set.
						netInterface.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
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
					Expect(ctx.GetNetwork(0).Backing.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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

						netInterface.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
						netInterface.Status.MacAddress = "" // NetOP doesn't set this.
						netInterface.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
						netInterface.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
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
					Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
					Expect(result.Backing).ToNot(BeNil())
					Expect(result.Backing.Reference()).To(Equal(ctx.GetNetwork(0).Backing.Reference()))
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

		Context("Standard Portgroup", func() {
			BeforeEach(func() {
				testConfig.WithNetworkEnv = ""
				testConfig.WithNetworkConfig = []builder.VCSimNetworkConfig{
					{Provider: pkgcfg.NetworkProviderTypeVDS, StandardPortGroup: true},
				}

				pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
					config.Features.PerNamespaceNetworkProvider = true
				})

				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							TypeMeta: netOPNetworkTypeMeta,
							Name:     networkName,
						},
					},
				}
			})

			simulateReady := func() {
				netInterface := &netopv1alpha1.NetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
						Namespace: vm.Namespace,
					},
				}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
				Expect(netInterface.Spec.NetworkName).To(Equal(networkName))

				netInterface.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
				netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
					{
						Type:   netopv1alpha1.NetworkInterfaceReady,
						Status: corev1.ConditionTrue,
					},
				}
				Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
			}

			When("PerNamespaceNetworkProvider is enabled", func() {
				It("resolves to a standard Network backing", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NetOP reconcile", simulateReady)

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
					Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
					Expect(result.Backing).ToNot(BeNil())

					backing, err := result.Backing.EthernetCardBackingInfo(ctx)
					Expect(err).ToNot(HaveOccurred())
					backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
					Expect(ok).To(BeTrue())

					expectedBacking, err := ctx.GetNetwork(0).Backing.EthernetCardBackingInfo(ctx)
					Expect(err).ToNot(HaveOccurred())
					expectedBackingInfo, ok := expectedBacking.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
					Expect(ok).To(BeTrue())
					Expect(backingInfo.DeviceName).To(Equal(expectedBackingInfo.DeviceName))
				})
			})

			When("PerNamespaceNetworkProvider is disabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.Features.PerNamespaceNetworkProvider = false
					})
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NetOP reconcile", simulateReady)

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceBackingNotSupported))
					Expect(results.Results).To(BeEmpty())
				})
			})
		})

		Context("IPAMModes", func() {
			// IPFamilyPolicy is a WorkloadIPv6 capability feature; enable it for all
			// sub-contexts that verify the field is properly propagated to NetOP.
			BeforeEach(func() {
				testConfig.WithWorkloadIPv6 = true
			})

			Context("WorkloadIPv6 capability disabled", func() {
				BeforeEach(func() {
					testConfig.WithWorkloadIPv6 = false
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: interfaceName,
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol},
						},
					}
				})

				It("does not set IPFamilyPolicy on NetworkInterface CR", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify NetworkInterface CR does not have IPFamilyPolicy set", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")))
					})
				})
			})

			Context("IPv4Only policy", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: interfaceName,
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol},
						},
					}
				})

				It("creates NetworkInterface CR with IPv4Only policy", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify NetworkInterface CR has IPv4Only policy", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only))
					})
				})
			})

			Context("IPv6Only policy", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: interfaceName,
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
						},
					}
				})

				It("creates NetworkInterface CR with IPv6Only policy", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify NetworkInterface CR has IPv6Only policy", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only))
					})
				})
			})

			Context("DualStack policy", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: interfaceName,
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{
								corev1.IPv4Protocol,
								corev1.IPv6Protocol,
							},
						},
					}
				})

				It("creates NetworkInterface CR with DualStack policy", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify NetworkInterface CR has DualStack policy", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack))
					})
				})
			})

			Context("optional field not specified", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: interfaceName,
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							// IPAMModes not set
						},
					}
				})

				It("creates NetworkInterface CR without IPFamilyPolicy set", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify NetworkInterface CR does not have NetOP IPFamilyPolicy set", func() {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, interfaceName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						// IPFamilyPolicy should be empty string when not specified
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")))
					})
				})
			})

			Context("multiple interfaces with different policies", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol},
						},
						{
							Name: "eth1",
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
						},
						{
							Name: "eth2",
							Network: &common.PartialObjectRef{
								TypeMeta: netOPNetworkTypeMeta,
								Name:     networkName,
							},
							IPAMModes: []corev1.IPFamily{
								corev1.IPv4Protocol,
								corev1.IPv6Protocol,
							},
						},
					}
				})

				It("creates NetworkInterface CRs with correct policies for each interface", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					expectedPolicies := map[string]netopv1alpha1.NetworkInterfaceIPFamilyPolicy{
						"eth0": netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only,
						"eth1": netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only,
						"eth2": netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack,
					}

					By("simulate successful NetOP reconcile for each interface", func() {
						for ifName := range expectedPolicies {
							netInterface := &netopv1alpha1.NetworkInterface{
								ObjectMeta: metav1.ObjectMeta{
									Name:      network.NetOPCRName(vm.Name, networkName, ifName, false),
									Namespace: vm.Namespace,
								},
							}
							err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)
							if apierrors.IsNotFound(err) {
								// First call bailed before creating this one; create it now.
								netInterface.Spec.NetworkName = networkName
								Expect(ctx.Client.Create(ctx, netInterface)).To(Succeed())
							} else {
								Expect(err).ToNot(HaveOccurred())
							}
							netInterface.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
							netInterface.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
								{
									Type:   netopv1alpha1.NetworkInterfaceReady,
									Status: corev1.ConditionTrue,
								},
							}
							Expect(ctx.Client.Status().Update(ctx, netInterface)).To(Succeed())
						}
					})

					results, err = network.CreateAndWaitForNetworkInterfaces(
						vmCtx,
						ctx.Client,
						ctx.VCClient.Client,
						ctx.Finder,
						nil,
						networkSpec)
					Expect(err).ToNot(HaveOccurred())
					Expect(results.Results).To(HaveLen(3))

					for ifName, expectedPolicy := range expectedPolicies {
						netInterface := &netopv1alpha1.NetworkInterface{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.NetOPCRName(vm.Name, networkName, ifName, false),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(netInterface), netInterface)).To(Succeed())
						Expect(netInterface.Spec.IPFamilyPolicy).To(Equal(expectedPolicy), "interface %q", ifName)
					}
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
			testConfig.WithNetworkEnv = builder.NetworkEnvNSXT
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							TypeMeta: nsxtNetworkTypeMeta,
							Name:     networkName,
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.GetNetwork(0).Backing.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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
						NsxLogicalSwitchID: ctx.GetNetwork(0).LogicalSwitchUUID,
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
				Expect(result.ObjectProviderType).To(Equal(pkgcfg.NetworkProviderTypeNSXT))
				Expect(result.MacAddress).To(Equal(macAddress))
				Expect(result.ExternalID).To(Equal(interfaceID))
				Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))
				Expect(result.Name).To(Equal(interfaceName))

				Expect(result.Backing).ToNot(BeNil())
				backing, err := result.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))

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
					Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.GetNetwork(0).Backing.Reference()))
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
					Expect(ctx.GetNetwork(0).Backing.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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
							NsxLogicalSwitchID: ctx.GetNetwork(0).LogicalSwitchUUID,
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
					Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))
					Expect(result.Name).To(Equal(interfaceName))

					Expect(result.Backing).ToNot(BeNil())
					backing, err := result.Backing.EthernetCardBackingInfo(ctx)
					Expect(err).ToNot(HaveOccurred())
					opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
					Expect(ok).To(BeTrue())
					Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))

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
						Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.GetNetwork(0).Backing.Reference()))
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
			testConfig.WithNetworkEnv = builder.NetworkEnvVPC
		})

		Context("Simulate workflow", func() {
			BeforeEach(func() {
				networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName,
						Network: &common.PartialObjectRef{
							TypeMeta: vpcNetworkTypeMeta,
							Name:     networkName,
						},
					},
				}
			})

			It("returns success", func() {
				// Assert test env is what we expect.
				Expect(ctx.GetNetwork(0).Backing.Reference().Type).To(Equal("DistributedVirtualPortgroup"))

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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
					subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = ctx.GetNetwork(0).LogicalSwitchUUID
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
				Expect(result.ObjectProviderType).To(Equal(pkgcfg.NetworkProviderTypeVPC))
				Expect(result.MacAddress).To(Equal(macAddress))
				Expect(result.ExternalID).To(Equal(interfaceID))
				Expect(result.NetworkID).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))
				Expect(result.Name).To(Equal(interfaceName))

				Expect(result.Backing).ToNot(BeNil())
				backing, err := result.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork, ok := backing.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork.OpaqueNetworkId).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))

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
					Expect(results.Results[0].Backing.Reference()).To(Equal(ctx.GetNetwork(0).Backing.Reference()))
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
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
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
						subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = ctx.GetNetwork(0).LogicalSwitchUUID
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

			When("interfaceSpec provides multiple IP addresses", func() {
				const (
					ipAddr1 = "192.168.1.220"
					ipAddr2 = "192.168.1.221"
					ipAddr3 = "fd1a:6c85:79fe:7c98::1"
					ipCIDR1 = ipAddr1 + "/24"
					ipCIDR2 = ipAddr2 + "/24"
					ipCIDR3 = ipAddr3 + "/64"
				)

				BeforeEach(func() {
					networkSpec.Interfaces[0].MACAddr = macAddress
					networkSpec.Interfaces[0].Addresses = []string{ipCIDR1, ipCIDR2, ipCIDR3}
				})

				It("returns success with multiple IP addresses", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NSX Operator reconcile", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())

						// NSX currently only supports 1 IP in the SubnetPort.
						Expect(subnetPort.Spec.AddressBindings).To(HaveLen(1))
						Expect(subnetPort.Spec.AddressBindings[0].IPAddress).To(Equal(ipAddr1))
						Expect(subnetPort.Spec.AddressBindings[0].MACAddress).To(Equal(macAddress))
						/*
							Expect(subnetPort.Spec.AddressBindings[1].IPAddress).To(Equal(ipAddr2))
							Expect(subnetPort.Spec.AddressBindings[1].MACAddress).To(Equal(macAddress))
							Expect(subnetPort.Spec.AddressBindings[2].IPAddress).To(Equal(ipAddr3))
							Expect(subnetPort.Spec.AddressBindings[2].MACAddress).To(Equal(macAddress))
						*/

						subnetPort.Status.Attachment.ID = interfaceID
						subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = ctx.GetNetwork(0).LogicalSwitchUUID
						subnetPort.Status.NetworkInterfaceConfig.MACAddress = macAddress
						subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
							{
								IPAddress: ipCIDR1,
								Gateway:   "192.168.1.1",
							},
							{
								IPAddress: ipCIDR2,
								Gateway:   "192.168.1.1",
							},
							{
								IPAddress: ipCIDR3,
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
					Expect(result.IPConfigs).To(HaveLen(3))
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal(ipCIDR1))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[1]
					Expect(ipConfig.IPCIDR).To(Equal(ipCIDR2))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[2]
					Expect(ipConfig.IPCIDR).To(Equal(ipCIDR3))
					Expect(ipConfig.IsIPv4).To(BeFalse())
					Expect(ipConfig.Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
				})
			})

			When("interfaceSpec provides only invalid IP addresses", func() {
				BeforeEach(func() {
					networkSpec.Interfaces[0].MACAddr = macAddress
					networkSpec.Interfaces[0].Gateway4 = "192.168.200.1"
					networkSpec.Interfaces[0].Addresses = []string{
						"256.256.256.256/24", // Invalid IP - out of range (logged as "invalid CIDR address")
						"0.0.0.0/24",         // Invalid IP - unspecified (logged as "unspecified")
						"127.0.0.1/24",       // Invalid IP - loopback (logged as "loopback")
						"169.254.1.1/24",     // Invalid IP - link local unicast (logged as "link local unicast")
						"224.0.0.1/24",       // Invalid IP - link local multicast (logged as "link local multicast")
						"192.168.1.300/24",   // Invalid IP - malformed (logged as "invalid CIDR address")
					}
				})

				It("creates SubnetPort with only MAC address when all IPs are invalid", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NSX Operator reconcile", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())

						// Should have no address bindings since all IPs were invalid
						Expect(subnetPort.Spec.AddressBindings).To(BeEmpty())

						subnetPort.Status.Attachment.ID = interfaceID
						subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = ctx.GetNetwork(0).LogicalSwitchUUID
						subnetPort.Status.NetworkInterfaceConfig.MACAddress = macAddress
						subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
							{
								Gateway: "192.168.1.1",
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
					Expect(result.IPConfigs).To(HaveLen(4))

					// NOTE: Since SubnetPort does not have an IP, we will ignore the
					// Gateway. We need to do that for NoIPAM.
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal("0.0.0.0/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.200.1"))
					ipConfig = result.IPConfigs[1]
					Expect(ipConfig.IPCIDR).To(Equal("127.0.0.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.200.1"))
					ipConfig = result.IPConfigs[2]
					Expect(ipConfig.IPCIDR).To(Equal("169.254.1.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.200.1"))
					ipConfig = result.IPConfigs[3]
					Expect(ipConfig.IPCIDR).To(Equal("224.0.0.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.200.1"))
				})
			})

			When("interfaceSpec provides mixed valid and invalid IP addresses with logging", func() {
				BeforeEach(func() {
					networkSpec.Interfaces[0].MACAddr = macAddress
					networkSpec.Interfaces[0].Addresses = []string{
						"256.256.256.256/24", // Invalid IP - out of range (logged as "invalid CIDR address")
						"0.0.0.0/24",         // Invalid IP - unspecified (logged as "unspecified")
						"127.0.0.1/24",       // Invalid IP - loopback (logged as "loopback")
						"169.254.1.1/24",     // Invalid IP - link local unicast (logged as "link local unicast")
						"224.0.0.1/24",       // Invalid IP - link local multicast (logged as "link local multicast")
						"192.168.1.300/24",   // Invalid IP - malformed (logged as "invalid CIDR address")
						"192.168.1.100/24",   // Valid IP
						"192.168.1.200/24",   // Valid IP
						"192.168.1.250/24",   // Valid IP
					}
				})

				It("processes valid IPs and logs skipped invalid ones", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(results.Results).To(BeEmpty())

					By("simulate successful NSX Operator reconcile", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())

						// Should only have 3 valid IP addresses (192.168.1.100, 192.168.1.200, 192.168.1.250)
						// BUT NSX currently only supports 1 IP in the SubnetPort.
						Expect(subnetPort.Spec.AddressBindings).To(HaveLen(1))
						Expect(subnetPort.Spec.AddressBindings[0].IPAddress).To(Equal("192.168.1.100"))
						Expect(subnetPort.Spec.AddressBindings[0].MACAddress).To(Equal(macAddress))
						/*
							Expect(subnetPort.Spec.AddressBindings[1].IPAddress).To(Equal("192.168.1.200"))
							Expect(subnetPort.Spec.AddressBindings[1].MACAddress).To(Equal(macAddress))
							Expect(subnetPort.Spec.AddressBindings[2].IPAddress).To(Equal("192.168.1.250"))
							Expect(subnetPort.Spec.AddressBindings[2].MACAddress).To(Equal(macAddress))
						*/

						subnetPort.Status.Attachment.ID = interfaceID
						subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID = ctx.GetNetwork(0).LogicalSwitchUUID
						subnetPort.Status.NetworkInterfaceConfig.MACAddress = macAddress
						subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
							{
								IPAddress: "192.168.1.100/24",
								Gateway:   "192.168.1.1",
							},
							/*
								{
									IPAddress: "192.168.1.200/24",
									Gateway:   "192.168.1.1",
								},
								{
									IPAddress: "192.168.1.250/24",
									Gateway:   "192.168.1.1",
								},

							*/
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
					Expect(result.IPConfigs).To(HaveLen(7))
					ipConfig := result.IPConfigs[0]
					Expect(ipConfig.IPCIDR).To(Equal("0.0.0.0/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[1]
					Expect(ipConfig.IPCIDR).To(Equal("127.0.0.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[2]
					Expect(ipConfig.IPCIDR).To(Equal("169.254.1.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[3]
					Expect(ipConfig.IPCIDR).To(Equal("224.0.0.1/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[4]
					Expect(ipConfig.IPCIDR).To(Equal("192.168.1.100/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[5]
					Expect(ipConfig.IPCIDR).To(Equal("192.168.1.200/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
					ipConfig = result.IPConfigs[6]
					Expect(ipConfig.IPCIDR).To(Equal("192.168.1.250/24"))
					Expect(ipConfig.IsIPv4).To(BeTrue())
					Expect(ipConfig.Gateway).To(Equal("192.168.1.1"))
				})
			})
		})

		Context("InterfaceIPType and StaticIPAllocationType", func() {
			// InterfaceIPType and StaticIPAllocationType are WorkloadIPv6 capability
			// features; enable them for all sub-contexts that verify proper propagation.
			BeforeEach(func() {
				testConfig.WithWorkloadIPv6 = true
			})

			Context("WorkloadIPv6 capability disabled", func() {
				BeforeEach(func() {
					testConfig.WithWorkloadIPv6 = false
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:      interfaceName,
							Network:   &common.PartialObjectRef{Name: networkName, TypeMeta: vpcNetworkTypeMeta},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol},
						},
					}
				})

				It("does not set InterfaceIPType on SubnetPort", func() {
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify SubnetPort does not have InterfaceIPType set", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
						Expect(subnetPort.Spec.InterfaceIPType).To(Equal(vpcv1alpha1.IPAddressType("")))
						Expect(subnetPort.Spec.StaticIPAllocationType).To(Equal(vpcv1alpha1.StaticIPAllocationType("")))
					})
				})
			})

			Context("IPv4 only", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:      interfaceName,
							Network:   &common.PartialObjectRef{Name: networkName, TypeMeta: vpcNetworkTypeMeta},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol},
						},
					}
				})

				It("sets InterfaceIPType to IPv4", func() {
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify SubnetPort has InterfaceIPType IPv4", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
						Expect(subnetPort.Spec.InterfaceIPType).To(Equal(vpcv1alpha1.IPAddressTypeIPv4))
						Expect(subnetPort.Spec.StaticIPAllocationType).To(Equal(vpcv1alpha1.StaticIPAllocationType("")))
					})
				})
			})

			Context("IPv6 only", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:      interfaceName,
							Network:   &common.PartialObjectRef{Name: networkName, TypeMeta: vpcNetworkTypeMeta},
							IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
						},
					}
				})

				It("sets InterfaceIPType to IPv6", func() {
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify SubnetPort has InterfaceIPType IPv6", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
						Expect(subnetPort.Spec.InterfaceIPType).To(Equal(vpcv1alpha1.IPAddressTypeIPv6))
						Expect(subnetPort.Spec.StaticIPAllocationType).To(Equal(vpcv1alpha1.StaticIPAllocationType("")))
					})
				})
			})

			Context("dual stack", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:      interfaceName,
							Network:   &common.PartialObjectRef{Name: networkName, TypeMeta: vpcNetworkTypeMeta},
							IPAMModes: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
						},
					}
				})

				It("sets InterfaceIPType to IPv4IPv6", func() {
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify SubnetPort has InterfaceIPType IPv4IPv6", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
						Expect(subnetPort.Spec.InterfaceIPType).To(Equal(vpcv1alpha1.IPAddressTypeIPv4IPv6))
						Expect(subnetPort.Spec.StaticIPAllocationType).To(Equal(vpcv1alpha1.StaticIPAllocationType("")))
					})
				})
			})

			Context("DHCP6=false with IPv6 mode", func() {
				BeforeEach(func() {
					networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name:      interfaceName,
							Network:   &common.PartialObjectRef{Name: networkName, TypeMeta: vpcNetworkTypeMeta},
							IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
							DHCP6:     ptr.To(false),
						},
					}
				})

				It("sets StaticIPAllocationType to IPv6", func() {
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))

					By("verify SubnetPort has StaticIPAllocationType IPv6", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{
							ObjectMeta: metav1.ObjectMeta{
								Name:      network.VPCCRName(vm.Name, networkName, interfaceName),
								Namespace: vm.Namespace,
							},
						}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(subnetPort), subnetPort)).To(Succeed())
						Expect(subnetPort.Spec.InterfaceIPType).To(Equal(vpcv1alpha1.IPAddressTypeIPv6))
						Expect(subnetPort.Spec.StaticIPAllocationType).To(Equal(vpcv1alpha1.StaticIPAllocationTypeIPv6))
					})
				})
			})
		})
	})

	Context("Mixed VDS and VPC", func() {
		const (
			ifaceName0 = "eth0"
			ifaceName1 = "eth1"
			netName0   = "my-vds-net"
			netName1   = "my-vpc-net"
			ifaceID0   = "my-vds-external-id"
			ifaceID1   = "my-vpc-external-id"
			macAddr0   = "01:02:03:04:05:06"
			macAddr1   = "aa:bb:cc:dd:ee:ff"
		)

		BeforeEach(func() {
			testConfig.NumNetworks = 0
			testConfig.WithNetworkConfig = []builder.VCSimNetworkConfig{
				{Provider: pkgcfg.NetworkProviderTypeVDS},
				{Provider: pkgcfg.NetworkProviderTypeVPC},
			}
			networkSpec.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name: ifaceName0,
					Network: &common.PartialObjectRef{
						TypeMeta: netOPNetworkTypeMeta,
						Name:     netName0,
					},
				},
				{
					Name: ifaceName1,
					Network: &common.PartialObjectRef{
						TypeMeta: vpcNetworkTypeMeta,
						Name:     netName1,
					},
				},
			}
		})

		Context("Simulate workflow", func() {
			It("returns results with correct ObjectProviderType for each interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(results.Results).To(BeEmpty())

				netopKey := client.ObjectKey{
					Name:      network.NetOPCRName(vm.Name, netName0, ifaceName0, false),
					Namespace: vm.Namespace,
				}
				subnetKey := client.ObjectKey{
					Name:      network.VPCCRName(vm.Name, netName1, ifaceName1),
					Namespace: vm.Namespace,
				}

				By("verify network interface CRs created", func() {
					By("NetworkInterface", func() {
						netIf := &netopv1alpha1.NetworkInterface{}
						Expect(ctx.Client.Get(ctx, netopKey, netIf)).To(Succeed())
						Expect(metav1.IsControlledBy(netIf, vm)).To(BeTrue())
						Expect(netIf.Spec.NetworkName).To(Equal(netName0))
					})

					By("SubnetPort", func() {
						subnetPort := &vpcv1alpha1.SubnetPort{}
						Expect(ctx.Client.Get(ctx, subnetKey, subnetPort)).To(Succeed())
						Expect(metav1.IsControlledBy(subnetPort, vm)).To(BeTrue())
						Expect(subnetPort.Spec.SubnetSet).To(Equal(netName1))
					})
				})

				By("simulate VDS interface ready", func() {
					netIf := &netopv1alpha1.NetworkInterface{}
					Expect(ctx.Client.Get(ctx, netopKey, netIf)).To(Succeed())
					netIf.Status.NetworkID = ctx.GetNetwork(0).Backing.Reference().Value
					netIf.Status.MacAddress = macAddr0
					netIf.Status.ExternalID = ifaceID0
					netIf.Status.Conditions = []netopv1alpha1.NetworkInterfaceCondition{
						{
							Type:   netopv1alpha1.NetworkInterfaceReady,
							Status: corev1.ConditionTrue,
						},
					}
					Expect(ctx.Client.Status().Update(ctx, netIf)).To(Succeed())
				})

				By("simulate VPC interface ready", func() {
					subnetPort := &vpcv1alpha1.SubnetPort{}
					Expect(ctx.Client.Get(ctx, subnetKey, subnetPort)).To(Succeed())
					subnetPort.Status = vpcv1alpha1.SubnetPortStatus{
						Attachment: vpcv1alpha1.PortAttachment{ID: ifaceID1},
						NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
							LogicalSwitchUUID: ctx.GetNetwork(1).LogicalSwitchUUID,
							MACAddress:        macAddr1,
						},
						Conditions: []vpcv1alpha1.Condition{
							{
								Type:   vpcv1alpha1.Ready,
								Status: corev1.ConditionTrue,
							},
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
				Expect(results.Results).To(HaveLen(2))

				result0 := results.Results[0]
				Expect(result0.ObjectProviderType).To(Equal(pkgcfg.NetworkProviderTypeVDS))
				Expect(result0.Name).To(Equal(ifaceName0))
				Expect(result0.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
				Expect(result0.MacAddress).To(Equal(macAddr0))
				Expect(result0.ExternalID).To(Equal(ifaceID0))
				Expect(result0.Backing).ToNot(BeNil())

				backing0, err := result0.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				dvpBacking0, ok := backing0.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(dvpBacking0.Port.PortgroupKey).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))

				result1 := results.Results[1]
				Expect(result1.ObjectProviderType).To(Equal(pkgcfg.NetworkProviderTypeVPC))
				Expect(result1.Name).To(Equal(ifaceName1))
				Expect(result1.NetworkID).To(Equal(ctx.GetNetwork(1).LogicalSwitchUUID))
				Expect(result1.MacAddress).To(Equal(macAddr1))
				Expect(result1.ExternalID).To(Equal(ifaceID1))
				Expect(result1.Backing).ToNot(BeNil())

				backing1, err := result1.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueBacking, ok := backing1.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue(), "VPC device should have opaque network backing")
				Expect(opaqueBacking.OpaqueNetworkId).To(Equal(result1.NetworkID))
			})
		})
	})
})

var _ = Describe("SetNetworkInterfaceOwnerRef", func() {
	var (
		vm     *vmopv1.VirtualMachine
		netIf  *netopv1alpha1.NetworkInterface
		client client.Client
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-vm",
				Namespace: "my-ns",
				UID:       "my-vm-uid",
			},
		}
		netIf = &netopv1alpha1.NetworkInterface{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-interface",
				Namespace: vm.Namespace,
			},
		}

		client = builder.NewFakeClient()
	})

	When("Network interface has VM owner ref", func() {
		It("owner ref gets upgraded to controller ref", func() {
			Expect(controllerutil.SetOwnerReference(vm, netIf, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netIf, client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			Expect(netIf.OwnerReferences).To(HaveLen(1))
			Expect(metav1.IsControlledBy(netIf, vm)).To(BeTrue())
		})
	})

	When("Network interface already has a controller ref", func() {
		It("VM is added as owner ref", func() {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: vm.Namespace, UID: "my-cm-uid"}}
			Expect(controllerutil.SetControllerReference(cm, netIf, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netIf, client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			Expect(netIf.OwnerReferences).To(HaveLen(2))
			Expect(netIf.OwnerReferences[0].Name).To(Equal(cm.Name))
			Expect(netIf.OwnerReferences[1].Name).To(Equal(vm.Name))
			Expect(metav1.IsControlledBy(netIf, cm)).To(BeTrue())
		})
	})

	When("Network interface already has a controller ref that is a VM", func() {
		It("New VM is not added as owner ref", func() {
			ownerVM := &vmopv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "my-vm-1", Namespace: vm.Namespace}}
			Expect(controllerutil.SetControllerReference(ownerVM, netIf, client.Scheme())).To(Succeed())

			err := network.SetNetworkInterfaceOwnerRef(vm, netIf, client.Scheme())
			Expect(err).To(HaveOccurred())
			Expect(netIf.OwnerReferences).To(HaveLen(1))
			Expect(metav1.IsControlledBy(netIf, ownerVM)).To(BeTrue())
		})
	})
})

var _ = Describe("CreateNetworkDevices", Label(testlabels.VCSim), func() {

	const (
		interfaceName0 = "eth0"
		interfaceName1 = "eth1"
		networkName0   = "my-network"
		networkName1   = "my-network-1"
		macAddress0    = "01:02:03:04:05:06"
		macAddress1    = "00:AA:BB:CC:DD:FF"
		externalID0    = "my-external-id"
		externalID1    = "my-external-id-1"
	)

	var (
		ctx         *builder.TestContextForVCSim
		testConfig  builder.VCSimTestConfig
		initObjects []client.Object

		vm            *vmopv1.VirtualMachine
		interfaceKey0 client.ObjectKey
		interfaceKey1 client.ObjectKey
		devices       []network.Device
		err           error
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "create-net-dev-vm",
				Namespace: "create-net-dev-ns",
				UID:       "create-net-dev-uid",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)

		devices, err = network.CreateNetworkDevices(
			ctx,
			vm,
			ctx.Client,
			ctx.VCClient.Client,
			ctx.Finder)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	netOPReconcile := func(objKey client.ObjectKey, r func(s *netopv1alpha1.NetworkInterfaceStatus)) {
		netIf := &netopv1alpha1.NetworkInterface{}
		Expect(ctx.Client.Get(ctx, objKey, netIf)).To(Succeed())
		r(&netIf.Status)
		Expect(ctx.Client.Status().Update(ctx, netIf)).To(Succeed())
	}

	ncpReconcile := func(k client.ObjectKey, r func(s *ncpv1alpha1.VirtualNetworkInterfaceStatus)) {
		vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
		Expect(ctx.Client.Get(ctx, k, vnetIf)).To(Succeed())
		r(&vnetIf.Status)
		Expect(ctx.Client.Status().Update(ctx, vnetIf)).To(Succeed())
	}

	vpcReconcile := func(objKey client.ObjectKey, r func(s *vpcv1alpha1.SubnetPortStatus)) {
		subnetPort := &vpcv1alpha1.SubnetPort{}
		Expect(ctx.Client.Get(ctx, objKey, subnetPort)).To(Succeed())
		r(&subnetPort.Status)
		Expect(ctx.Client.Status().Update(ctx, subnetPort)).To(Succeed())
	}

	Context("network spec is nil", func() {
		It("returns nil devices and no error", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(devices).To(BeNil())
		})
	})

	Context("network spec is disabled", func() {
		BeforeEach(func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: true,
			}
		})

		It("returns nil devices and no error", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(devices).To(BeNil())
		})
	})

	Context("Named Network", func() {
		// Use network vcsim automatically creates.
		const networkName = "DC0_DVPG0"

		BeforeEach(func() {
			testConfig.NumNetworks = 0
			testConfig.WithNetworkEnv = builder.NetworkEnvNamed

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name:    interfaceName0,
						Network: &common.PartialObjectRef{Name: networkName},
					},
				},
			}
		})

		It("returns success with backing", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(devices).To(HaveLen(1))

			dev := devices[0]
			Expect(dev.Backing).ToNot(BeNil())
			backing, err := dev.Backing.EthernetCardBackingInfo(ctx)
			Expect(err).ToNot(HaveOccurred())
			backingInfo, ok := backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
			Expect(ok).To(BeTrue())
			dvpg, err := ctx.Finder.Network(ctx, networkName)
			Expect(err).ToNot(HaveOccurred())
			Expect(backingInfo.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
		})

		Context("Named Network with MAC address", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].MACAddr = macAddress0
			})

			It("returns device with MAC address", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(devices).To(HaveLen(1))
				Expect(devices[0].MacAddress).To(Equal(macAddress0))
			})
		})
	})

	Context("VDS", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVDS
			testConfig.NumNetworks = 2

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName0,
						Network: &common.PartialObjectRef{
							TypeMeta: netOPNetworkTypeMeta,
							Name:     networkName0,
						},
					},
					{
						Name: interfaceName1,
						Network: &common.PartialObjectRef{
							TypeMeta: netOPNetworkTypeMeta,
							Name:     networkName1,
						},
					},
				},
			}

			interfaceKey0 = client.ObjectKey{
				Name:      network.NetOPCRName(vm.Name, networkName0, interfaceName0, false),
				Namespace: vm.Namespace,
			}

			interfaceKey1 = client.ObjectKey{
				Name:      network.NetOPCRName(vm.Name, networkName1, interfaceName1, false),
				Namespace: vm.Namespace,
			}
		})

		Context("interface is not ready", func() {
			It("returns not-ready error and creates the interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				netIf := &netopv1alpha1.NetworkInterface{}
				Expect(ctx.Client.Get(ctx, interfaceKey0, netIf)).To(Succeed())
				Expect(metav1.IsControlledBy(netIf, vm)).To(BeTrue())
				Expect(netIf.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(netIf.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName0))
				Expect(netIf.Spec.NetworkName).To(Equal(networkName0))

				Expect(ctx.Client.Get(ctx, interfaceKey1, netIf)).To(Succeed())
				Expect(metav1.IsControlledBy(netIf, vm)).To(BeTrue())
				Expect(netIf.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(netIf.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName1))
				Expect(netIf.Spec.NetworkName).To(Equal(networkName1))
			})

			DescribeTableSubtree("interface spec with IPAMModes",
				func(modes []corev1.IPFamily, expected netopv1alpha1.NetworkInterfaceIPFamilyPolicy) {
					BeforeEach(func() {
						testConfig.WithWorkloadIPv6 = true
						vm.Spec.Network.Interfaces[0].IPAMModes = modes
					})

					It("interface IPFamilyPolicy is set", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
						Expect(devices).To(BeEmpty())

						netIf := &netopv1alpha1.NetworkInterface{}
						Expect(ctx.Client.Get(ctx, interfaceKey0, netIf)).To(Succeed())
						Expect(netIf.Spec.IPFamilyPolicy).To(Equal(expected))
					})
				},
				Entry("None", []corev1.IPFamily{}, netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")),
				Entry("IPv4", []corev1.IPFamily{corev1.IPv4Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only),
				Entry("IPv6", []corev1.IPFamily{corev1.IPv6Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only),
				Entry("Dual", []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack),
			)
		})

		Context("interfaces become ready", func() {
			It("returns success with backing", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				By("Mark interfaces as ready", func() {
					netOPReconcile(interfaceKey0, func(s *netopv1alpha1.NetworkInterfaceStatus) {
						*s = netopv1alpha1.NetworkInterfaceStatus{
							NetworkID:  ctx.GetNetwork(0).Backing.Reference().Value,
							MacAddress: macAddress0,
							ExternalID: externalID0,
							Conditions: []netopv1alpha1.NetworkInterfaceCondition{
								{
									Type:   netopv1alpha1.NetworkInterfaceReady,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})

					netOPReconcile(interfaceKey1, func(s *netopv1alpha1.NetworkInterfaceStatus) {
						*s = netopv1alpha1.NetworkInterfaceStatus{
							NetworkID:  ctx.GetNetwork(1).Backing.Reference().Value,
							MacAddress: macAddress1,
							ExternalID: externalID1,
							Conditions: []netopv1alpha1.NetworkInterfaceCondition{
								{
									Type:   netopv1alpha1.NetworkInterfaceReady,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})
				})

				devices, err = network.CreateNetworkDevices(
					ctx,
					vm,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder)
				Expect(err).ToNot(HaveOccurred())
				Expect(devices).To(HaveLen(2))

				dev0 := devices[0]
				Expect(dev0.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVDS))
				Expect(dev0.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
				Expect(dev0.Backing).ToNot(BeNil())
				Expect(dev0.MacAddress).To(Equal(macAddress0))
				Expect(dev0.ExternalID).To(Equal(externalID0))

				backing0, err := dev0.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				dvpBacking0, ok := backing0.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(dvpBacking0.Port.PortgroupKey).To(Equal(dev0.NetworkID))

				dev1 := devices[1]
				Expect(dev1.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVDS))
				Expect(dev1.NetworkID).To(Equal(ctx.GetNetwork(1).Backing.Reference().Value))
				Expect(dev1.Backing).ToNot(BeNil())
				Expect(dev1.MacAddress).To(Equal(macAddress1))
				Expect(dev1.ExternalID).To(Equal(externalID1))

				backing1, err := dev1.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				dvpBacking1, ok := backing1.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(dvpBacking1.Port.PortgroupKey).To(Equal(dev1.NetworkID))
			})
		})

		Context("interface has failure condition", func() {
			BeforeEach(func() {
				netIf0 := &netopv1alpha1.NetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      interfaceKey0.Name,
						Namespace: interfaceKey0.Namespace,
					},
					Status: netopv1alpha1.NetworkInterfaceStatus{
						Conditions: []netopv1alpha1.NetworkInterfaceCondition{
							{
								Type:    netopv1alpha1.NetworkInterfaceFailure,
								Status:  corev1.ConditionTrue,
								Reason:  "SomeReason",
								Message: "something went wrong",
							},
						},
					},
				}

				netIf1 := &netopv1alpha1.NetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      interfaceKey1.Name,
						Namespace: interfaceKey1.Namespace,
					},
					Status: netopv1alpha1.NetworkInterfaceStatus{
						NetworkID:  "dvportgroup-42",
						MacAddress: macAddress1,
						ExternalID: externalID1,
						Conditions: []netopv1alpha1.NetworkInterfaceCondition{
							{
								Type:   netopv1alpha1.NetworkInterfaceReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}

				initObjects = append(initObjects, netIf0, netIf1)
			})

			It("returns failure error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(err).To(MatchError("error getting device for interface eth0: network interface is not ready: failure - SomeReason: something went wrong"))
			})
		})
	})

	Context("NSXT", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNSXT
			testConfig.NumNetworks = 2

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName0,
						Network: &common.PartialObjectRef{
							TypeMeta: nsxtNetworkTypeMeta,
							Name:     networkName0,
						},
					},
					{
						Name: interfaceName1,
						Network: &common.PartialObjectRef{
							TypeMeta: nsxtNetworkTypeMeta,
							Name:     networkName1,
						},
					},
				},
			}

			interfaceKey0 = client.ObjectKey{
				Name:      network.NCPCRName(vm.Name, networkName0, interfaceName0, false),
				Namespace: vm.Namespace,
			}

			interfaceKey1 = client.ObjectKey{
				Name:      network.NCPCRName(vm.Name, networkName1, interfaceName1, false),
				Namespace: vm.Namespace,
			}
		})

		Context("interface is not ready", func() {
			It("returns not-ready error and creates the interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
				Expect(ctx.Client.Get(ctx, interfaceKey0, vnetIf)).To(Succeed())
				Expect(metav1.IsControlledBy(vnetIf, vm)).To(BeTrue())
				Expect(vnetIf.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(vnetIf.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName0))
				Expect(vnetIf.Spec.VirtualNetwork).To(Equal(networkName0))

				Expect(ctx.Client.Get(ctx, interfaceKey1, vnetIf)).To(Succeed())
				Expect(metav1.IsControlledBy(vnetIf, vm)).To(BeTrue())
				Expect(vnetIf.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(vnetIf.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName1))
				Expect(vnetIf.Spec.VirtualNetwork).To(Equal(networkName1))
			})
		})

		Context("interfaces become ready", func() {
			It("returns success with backing and identifiers", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				By("Mark interfaces as ready", func() {
					ncpReconcile(interfaceKey0, func(s *ncpv1alpha1.VirtualNetworkInterfaceStatus) {
						*s = ncpv1alpha1.VirtualNetworkInterfaceStatus{
							InterfaceID: externalID0,
							MacAddress:  macAddress0,
							ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
								NsxLogicalSwitchID: ctx.GetNetwork(0).LogicalSwitchUUID,
							},
							Conditions: []ncpv1alpha1.VirtualNetworkCondition{
								{
									Type:   "Ready",
									Status: "True",
								},
							},
						}
					})

					ncpReconcile(interfaceKey1, func(s *ncpv1alpha1.VirtualNetworkInterfaceStatus) {
						*s = ncpv1alpha1.VirtualNetworkInterfaceStatus{
							InterfaceID: externalID1,
							MacAddress:  macAddress1,
							ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{
								NsxLogicalSwitchID: ctx.GetNetwork(1).LogicalSwitchUUID,
							},
							Conditions: []ncpv1alpha1.VirtualNetworkCondition{
								{
									Type:   "Ready",
									Status: "True",
								},
							},
						}
					})
				})

				devices, err = network.CreateNetworkDevices(
					ctx,
					vm,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder)
				Expect(err).ToNot(HaveOccurred())
				Expect(devices).To(HaveLen(2))

				dev0 := devices[0]
				Expect(dev0.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeNSXT))
				Expect(dev0.NetworkID).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))
				Expect(dev0.Backing).ToNot(BeNil())
				Expect(dev0.ExternalID).To(Equal(externalID0))
				Expect(dev0.MacAddress).To(Equal(macAddress0))

				backing0, err := dev0.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork0, ok := backing0.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork0.OpaqueNetworkId).To(Equal(dev0.NetworkID))

				dev1 := devices[1]
				Expect(dev1.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeNSXT))
				Expect(dev1.NetworkID).To(Equal(ctx.GetNetwork(1).LogicalSwitchUUID))
				Expect(dev1.Backing).ToNot(BeNil())
				Expect(dev1.ExternalID).To(Equal(externalID1))
				Expect(dev1.MacAddress).To(Equal(macAddress1))

				backing1, err := dev1.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork1, ok := backing1.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork1.OpaqueNetworkId).To(Equal(dev1.NetworkID))
			})
		})

		Context("interface is not ready", func() {
			BeforeEach(func() {
				vnetIf := &ncpv1alpha1.VirtualNetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      interfaceKey0.Name,
						Namespace: interfaceKey0.Namespace,
					},
					Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
						Conditions: []ncpv1alpha1.VirtualNetworkCondition{
							{
								Type:    "Ready",
								Status:  "False",
								Reason:  "ProvisioningFailed",
								Message: "subnet not found",
							},
						},
					},
				}
				initObjects = append(initObjects, vnetIf)
			})

			It("returns not-ready error with reason", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(err.Error()).To(ContainSubstring("network interface is not ready: ProvisioningFailed - subnet not found"))
			})
		})

		Context("interface is ready but missing provider status", func() {
			BeforeEach(func() {
				vnetIf := &ncpv1alpha1.VirtualNetworkInterface{
					ObjectMeta: metav1.ObjectMeta{
						Name:      interfaceKey0.Name,
						Namespace: interfaceKey0.Namespace,
					},
					Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
						Conditions: []ncpv1alpha1.VirtualNetworkCondition{
							{
								Type:   "Ready",
								Status: "True",
							},
						},
					},
				}
				initObjects = append(initObjects, vnetIf)
			})

			It("returns error about missing provider status", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("ready network interface does not have provider status"))
			})
		})
	})

	Context("VPC", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvVPC
			testConfig.NumNetworks = 2

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName0,
						Network: &common.PartialObjectRef{
							TypeMeta: vpcNetworkTypeMeta,
							Name:     networkName0,
						},
					},
					{
						Name: interfaceName1,
						Network: &common.PartialObjectRef{
							TypeMeta: vpcNetworkTypeMeta,
							Name:     networkName1,
						},
					},
				},
			}

			interfaceKey0 = client.ObjectKey{
				Name:      network.VPCCRName(vm.Name, networkName0, interfaceName0),
				Namespace: vm.Namespace,
			}

			interfaceKey1 = client.ObjectKey{
				Name:      network.VPCCRName(vm.Name, networkName1, interfaceName1),
				Namespace: vm.Namespace,
			}
		})

		Context("interface is not ready", func() {
			It("returns not-ready error and creates the interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				subnetPort := &vpcv1alpha1.SubnetPort{}
				Expect(ctx.Client.Get(ctx, interfaceKey0, subnetPort)).To(Succeed())
				Expect(metav1.IsControlledBy(subnetPort, vm)).To(BeTrue())
				Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName0))
				Expect(subnetPort.Spec.SubnetSet).To(Equal(networkName0))

				Expect(ctx.Client.Get(ctx, interfaceKey1, subnetPort)).To(Succeed())
				Expect(metav1.IsControlledBy(subnetPort, vm)).To(BeTrue())
				Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMInterfaceNameLabel, interfaceName1))
				Expect(subnetPort.Spec.SubnetSet).To(Equal(networkName1))
			})

			DescribeTableSubtree("interface spec with addresses",
				func(addresses []string, macAddress string, expectedIP string) {
					BeforeEach(func() {
						vm.Spec.Network.Interfaces[0].Addresses = addresses
						vm.Spec.Network.Interfaces[0].MACAddr = macAddress
					})

					It("SubnetPort AddressBindings are set correctly", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
						Expect(devices).To(BeEmpty())

						subnetPort := &vpcv1alpha1.SubnetPort{}
						Expect(ctx.Client.Get(ctx, interfaceKey0, subnetPort)).To(Succeed())

						if expectedIP != "" {
							Expect(subnetPort.Spec.AddressBindings).To(HaveLen(1))
							Expect(subnetPort.Spec.AddressBindings[0].IPAddress).To(Equal(expectedIP))
							Expect(subnetPort.Spec.AddressBindings[0].MACAddress).To(Equal(strings.ToLower(macAddress)))
						} else if macAddress != "" {
							Expect(subnetPort.Spec.AddressBindings).To(HaveLen(1))
							Expect(subnetPort.Spec.AddressBindings[0].IPAddress).To(BeEmpty())
							Expect(subnetPort.Spec.AddressBindings[0].MACAddress).To(Equal(strings.ToLower(macAddress)))
						} else {
							Expect(subnetPort.Spec.AddressBindings).To(BeEmpty())
						}
					})
				},
				Entry("No addresses or MAC", []string{}, "", ""),
				Entry("IPv4 address with MAC", []string{"192.168.1.100/24"}, macAddress0, "192.168.1.100"),
				Entry("IPv6 address with MAC", []string{"2001:db8::100/64"}, macAddress0, "2001:db8::100"),
				Entry("Multiple addresses (only first is used)", []string{"192.168.1.100/24", "192.168.1.101/24"}, macAddress0, "192.168.1.100"),
				Entry("Only MAC address", []string{}, macAddress0, ""),
				Entry("Invalid IP address skipped", []string{"256.256.256.256/24", "192.168.1.100/24"}, macAddress0, "192.168.1.100"),
			)

			Context("Default to SubnetSet", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Network.Kind = ""
				})

				It("interface is created with SubnetSet set", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(devices).To(BeEmpty())

					subnetPort := &vpcv1alpha1.SubnetPort{}
					Expect(ctx.Client.Get(ctx, interfaceKey0, subnetPort)).To(Succeed())
					Expect(subnetPort.Spec.SubnetSet).To(Equal(networkName0))
				})
			})

			Context("Subnet", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Network.Kind = "Subnet"
				})

				It("interface is created with Subnet assigned", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
					Expect(devices).To(BeEmpty())

					subnetPort := &vpcv1alpha1.SubnetPort{}
					Expect(ctx.Client.Get(ctx, interfaceKey0, subnetPort)).To(Succeed())
					Expect(subnetPort.Spec.Subnet).To(Equal(networkName0))
				})
			})
		})

		Context("interfaces become ready", func() {
			It("returns success with backing", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				By("Mark interfaces as ready", func() {
					vpcReconcile(interfaceKey0, func(s *vpcv1alpha1.SubnetPortStatus) {
						*s = vpcv1alpha1.SubnetPortStatus{
							Attachment: vpcv1alpha1.PortAttachment{
								ID: externalID0,
							},
							NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
								LogicalSwitchUUID: ctx.GetNetwork(0).LogicalSwitchUUID,
								MACAddress:        macAddress0,
							},
							Conditions: []vpcv1alpha1.Condition{
								{
									Type:   vpcv1alpha1.Ready,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})

					vpcReconcile(interfaceKey1, func(s *vpcv1alpha1.SubnetPortStatus) {
						*s = vpcv1alpha1.SubnetPortStatus{
							Attachment: vpcv1alpha1.PortAttachment{
								ID: externalID1,
							},
							NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
								LogicalSwitchUUID: ctx.GetNetwork(1).LogicalSwitchUUID,
								MACAddress:        macAddress1,
							},
							Conditions: []vpcv1alpha1.Condition{
								{
									Type:   vpcv1alpha1.Ready,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})
				})

				devices, err = network.CreateNetworkDevices(
					ctx,
					vm,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder)
				Expect(err).ToNot(HaveOccurred())
				Expect(devices).To(HaveLen(2))

				dev0 := devices[0]
				Expect(dev0.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVPC))
				Expect(dev0.NetworkID).To(Equal(ctx.GetNetwork(0).LogicalSwitchUUID))
				Expect(dev0.Backing).ToNot(BeNil())
				Expect(dev0.MacAddress).To(Equal(macAddress0))
				Expect(dev0.ExternalID).To(Equal(externalID0))

				backing0, err := dev0.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork0, ok := backing0.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork0.OpaqueNetworkId).To(Equal(dev0.NetworkID))

				dev1 := devices[1]
				Expect(dev1.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVPC))
				Expect(dev1.NetworkID).To(Equal(ctx.GetNetwork(1).LogicalSwitchUUID))
				Expect(dev1.Backing).ToNot(BeNil())
				Expect(dev1.MacAddress).To(Equal(macAddress1))
				Expect(dev1.ExternalID).To(Equal(externalID1))

				backing1, err := dev1.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueNetwork1, ok := backing1.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue())
				Expect(opaqueNetwork1.OpaqueNetworkId).To(Equal(dev1.NetworkID))
			})
		})
	})

	Context("Mixed VDS and VCP providers interfaces", func() {

		BeforeEach(func() {
			testConfig.NumNetworks = 0
			testConfig.WithNetworkConfig = []builder.VCSimNetworkConfig{
				{Provider: pkgcfg.NetworkProviderTypeVDS},
				{Provider: pkgcfg.NetworkProviderTypeVPC},
			}
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: interfaceName0,
						Network: &common.PartialObjectRef{
							TypeMeta: netOPNetworkTypeMeta,
							Name:     networkName0,
						},
					},
					{
						Name: interfaceName1,
						Network: &common.PartialObjectRef{
							TypeMeta: vpcNetworkTypeMeta,
							Name:     networkName1,
						},
					},
				},
			}

			interfaceKey0 = client.ObjectKey{
				Name:      network.NetOPCRName(vm.Name, networkName0, interfaceName0, false),
				Namespace: vm.Namespace,
			}

			interfaceKey1 = client.ObjectKey{
				Name:      network.VPCCRName(vm.Name, networkName1, interfaceName1),
				Namespace: vm.Namespace,
			}
		})

		Context("interfaces not ready", func() {
			It("creates a NetOP CR for the VDS interface and a SubnetPort for the VPC interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				netIf := &netopv1alpha1.NetworkInterface{}
				Expect(ctx.Client.Get(ctx, interfaceKey0, netIf)).To(Succeed())
				Expect(metav1.IsControlledBy(netIf, vm)).To(BeTrue())
				Expect(netIf.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(netIf.Spec.NetworkName).To(Equal(networkName0))

				subnetPort := &vpcv1alpha1.SubnetPort{}
				Expect(ctx.Client.Get(ctx, interfaceKey1, subnetPort)).To(Succeed())
				Expect(metav1.IsControlledBy(subnetPort, vm)).To(BeTrue())
				Expect(subnetPort.Labels).To(HaveKeyWithValue(network.VMNameLabel, vm.Name))
				Expect(subnetPort.Spec.SubnetSet).To(Equal(networkName1))
			})
		})

		Context("interfaces become ready", func() {
			It("returns devices with correct ProviderType for each interface", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				By("Mark VDS interface ready", func() {
					netOPReconcile(interfaceKey0, func(s *netopv1alpha1.NetworkInterfaceStatus) {
						*s = netopv1alpha1.NetworkInterfaceStatus{
							NetworkID:  ctx.GetNetwork(0).Backing.Reference().Value,
							MacAddress: macAddress0,
							ExternalID: externalID0,
							Conditions: []netopv1alpha1.NetworkInterfaceCondition{
								{
									Type:   netopv1alpha1.NetworkInterfaceReady,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})
				})

				By("Mark VPC interface ready", func() {
					vpcReconcile(interfaceKey1, func(s *vpcv1alpha1.SubnetPortStatus) {
						*s = vpcv1alpha1.SubnetPortStatus{
							Attachment: vpcv1alpha1.PortAttachment{
								ID: externalID1,
							},
							NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
								LogicalSwitchUUID: ctx.GetNetwork(1).LogicalSwitchUUID,
								MACAddress:        macAddress1,
							},
							Conditions: []vpcv1alpha1.Condition{
								{
									Type:   vpcv1alpha1.Ready,
									Status: corev1.ConditionTrue,
								},
							},
						}
					})
				})

				devices, err = network.CreateNetworkDevices(
					ctx,
					vm,
					ctx.Client,
					ctx.VCClient.Client,
					ctx.Finder)
				Expect(err).ToNot(HaveOccurred())
				Expect(devices).To(HaveLen(2))

				dev0 := devices[0]
				Expect(dev0.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVDS))
				Expect(dev0.NetworkID).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))
				Expect(dev0.Backing).ToNot(BeNil())
				Expect(dev0.MacAddress).To(Equal(macAddress0))
				Expect(dev0.ExternalID).To(Equal(externalID0))

				backing0, err := dev0.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				dvpBacking, ok := backing0.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				Expect(ok).To(BeTrue(), "VDS device should have DVPort backing")
				Expect(dvpBacking.Port.PortgroupKey).To(Equal(ctx.GetNetwork(0).Backing.Reference().Value))

				dev1 := devices[1]
				Expect(dev1.ProviderType).To(Equal(pkgcfg.NetworkProviderTypeVPC))
				Expect(dev1.NetworkID).To(Equal(ctx.GetNetwork(1).LogicalSwitchUUID))
				Expect(dev1.Backing).ToNot(BeNil())
				Expect(dev1.MacAddress).To(Equal(macAddress1))
				Expect(dev1.ExternalID).To(Equal(externalID1))

				backing1, err := dev1.Backing.EthernetCardBackingInfo(ctx)
				Expect(err).ToNot(HaveOccurred())
				opaqueBacking, ok := backing1.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				Expect(ok).To(BeTrue(), "VPC device should have opaque network backing")
				Expect(opaqueBacking.OpaqueNetworkId).To(Equal(dev1.NetworkID))
			})
		})

		Context("partial readiness", func() {
			markVDSReady := func() {
				netIf := &netopv1alpha1.NetworkInterface{}
				Expect(ctx.Client.Get(ctx, interfaceKey0, netIf)).To(Succeed())
				netIf.Status = netopv1alpha1.NetworkInterfaceStatus{
					NetworkID:  ctx.GetNetwork(0).Backing.Reference().Value,
					MacAddress: macAddress0,
					ExternalID: externalID0,
					Conditions: []netopv1alpha1.NetworkInterfaceCondition{
						{
							Type:   netopv1alpha1.NetworkInterfaceReady,
							Status: corev1.ConditionTrue,
						},
					},
				}
				Expect(ctx.Client.Status().Update(ctx, netIf)).To(Succeed())
			}

			markVPCReady := func() {
				subnetPort := &vpcv1alpha1.SubnetPort{}
				Expect(ctx.Client.Get(ctx, interfaceKey1, subnetPort)).To(Succeed())
				subnetPort.Status = vpcv1alpha1.SubnetPortStatus{
					Attachment: vpcv1alpha1.PortAttachment{ID: externalID1},
					NetworkInterfaceConfig: vpcv1alpha1.NetworkInterfaceConfig{
						LogicalSwitchUUID: ctx.GetNetwork(1).LogicalSwitchUUID,
						MACAddress:        macAddress1,
					},
					Conditions: []vpcv1alpha1.Condition{
						{
							Type:   vpcv1alpha1.Ready,
							Status: corev1.ConditionTrue,
						},
					},
				}
				Expect(ctx.Client.Status().Update(ctx, subnetPort)).To(Succeed())
			}

			It("returns ErrNetworkInterfaceNotReady when only the VDS interface is ready", func() {
				// First call creates both CRs but neither is ready yet.
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				markVDSReady()

				// VPC SubnetPort still unready — the call must report not-ready.
				devices, err = network.CreateNetworkDevices(
					ctx, vm, ctx.Client, ctx.VCClient.Client, ctx.Finder)
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())
			})

			It("returns ErrNetworkInterfaceNotReady when only the VPC interface is ready", func() {
				// First call creates both CRs but neither is ready yet.
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())

				markVPCReady()

				// NetOP NetworkInterface still unready — the call must report not-ready.
				devices, err = network.CreateNetworkDevices(
					ctx, vm, ctx.Client, ctx.VCClient.Client, ctx.Finder)
				Expect(err).To(MatchError(network.ErrNetworkInterfaceNotReady))
				Expect(devices).To(BeEmpty())
			})
		})
	})
})
