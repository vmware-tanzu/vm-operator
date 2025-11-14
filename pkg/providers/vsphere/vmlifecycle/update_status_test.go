// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apirecord "k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("UpdateStatus", func() {

	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		vcVM  *object.VirtualMachine
		data  vmlifecycle.ReconcileStatusData
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		vm := builder.DummyVirtualMachine()
		vm.Name = "update-status-test"

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		// Initialize with the expected properties. Tests can overwrite this if needed.
		Expect(vcVM.Properties(
			ctx,
			vcVM.Reference(),
			vsphere.VMUpdatePropertiesSelector,
			&vmCtx.MoVM)).To(Succeed())

		data = vmlifecycle.ReconcileStatusData{
			NetworkDeviceKeysToSpecIdx: map[int32]int{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
		Expect(err).ToNot(HaveOccurred())
	})

	When("properties are refetched", func() {
		BeforeEach(func() {
			vmCtx.MoVM = mo.VirtualMachine{}
			Expect(vcVM.Properties(
				ctx,
				vcVM.Reference(),
				vsphere.VMUpdatePropertiesSelector,
				&vmCtx.MoVM)).To(Succeed())
		})
		Specify("the status is created from the properties fetched from vsphere", func() {
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), vsphere.VMUpdatePropertiesSelector, &moVM)).To(Succeed())

			Expect(vmCtx.VM.Status.BiosUUID).To(Equal(moVM.Summary.Config.Uuid))
			Expect(vmCtx.VM.Status.InstanceUUID).To(Equal(moVM.Summary.Config.InstanceUuid))
			Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
			Expect(vmCtx.VM.Status.Network.HostName).To(Equal(moVM.Summary.Guest.HostName))
			Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(moVM.Summary.Guest.IpAddress))
		})
	})

	Context("Annotations to Conditions", func() {
		const (
			testAnnotationKey = "condition.vmoperator.vmware.com.protected/MyCondition"
			testConditionType = "MyConditionType"
			testReason        = "MyReason"
			testMessage       = "My message"
		)

		Context("When annotation matches the protected condition pattern", func() {
			When("annotation value has type and status=True", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType + ";True",
					}
				})
				It("should set condition to True", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				})
			})

			When("annotation value has type and status=False with reason and message", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType + ";False;" + testReason + ";" + testMessage,
					}
				})
				It("should set condition to False with reason and message", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					Expect(cond.Reason).To(Equal(testReason))
					Expect(cond.Message).To(Equal(testMessage))
				})
			})

			When("annotation value has type and status=Unknown with reason and message", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType + ";Unknown;UnknownReason;Unknown message",
					}
				})
				It("should set condition to Unknown with reason and message", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
					Expect(cond.Reason).To(Equal("UnknownReason"))
					Expect(cond.Message).To(Equal("Unknown message"))
				})
			})

			When("annotation value has type and unrecognized status", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType + ";InvalidStatus;" + testReason + ";" + testMessage,
					}
				})
				It("should set condition to Unknown", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
				})
			})

			When("annotation value has type but empty status", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType + ";;" + testReason + ";" + testMessage,
					}
				})
				It("should set condition to Unknown", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
				})
			})

			When("annotation value has only type", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: testConditionType,
					}
				})
				It("should set condition to Unknown", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
				})
			})

			When("annotation value is empty", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: "",
					}
				})
				It("should not set any condition", func() {
					cond := conditions.Get(vmCtx.VM, "")
					Expect(cond).To(BeNil())
				})
			})

			When("annotation value has no type (empty type)", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						testAnnotationKey: ";True;" + testReason + ";" + testMessage,
					}
				})
				It("should not set a condition for empty type", func() {
					// Since type is empty, no condition with empty type should be set
					// The function checks if t != "" before setting conditions
					cond := conditions.Get(vmCtx.VM, "")
					Expect(cond).To(BeNil())
					// But Created condition should still exist
					cond = conditions.Get(vmCtx.VM, vmopv1.VirtualMachineConditionCreated)
					Expect(cond).ToNot(BeNil())
				})
			})

			When("multiple protected condition annotations exist", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						"condition.vmoperator.vmware.com.protected/Condition1": "Type1;True",
						"condition.vmoperator.vmware.com.protected/Condition2": "Type2;False;Reason2;Message2",
						"condition.vmoperator.vmware.com.protected/Condition3": "Type3;Unknown;Reason3;Message3",
					}
				})
				It("should set all conditions appropriately", func() {
					cond1 := conditions.Get(vmCtx.VM, "Type1")
					Expect(cond1).ToNot(BeNil())
					Expect(cond1.Status).To(Equal(metav1.ConditionTrue))

					cond2 := conditions.Get(vmCtx.VM, "Type2")
					Expect(cond2).ToNot(BeNil())
					Expect(cond2.Status).To(Equal(metav1.ConditionFalse))
					Expect(cond2.Reason).To(Equal("Reason2"))
					Expect(cond2.Message).To(Equal("Message2"))

					cond3 := conditions.Get(vmCtx.VM, "Type3")
					Expect(cond3).ToNot(BeNil())
					Expect(cond3.Status).To(Equal(metav1.ConditionUnknown))
					Expect(cond3.Reason).To(Equal("Reason3"))
					Expect(cond3.Message).To(Equal("Message3"))
				})
			})
		})

		Context("When annotation does not match the protected condition pattern", func() {
			When("annotation has different prefix", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = map[string]string{
						"other.annotation/MyCondition": testConditionType + ";True",
					}
				})
				It("should not set any condition from this annotation", func() {
					cond := conditions.Get(vmCtx.VM, testConditionType)
					Expect(cond).To(BeNil())
				})
			})

			When("no annotations exist", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations = nil
				})
				It("should have default conditions set by ReconcileStatus", func() {
					// ReconcileStatus sets multiple conditions including Created and ReconcileReady
					// Just verify the Created condition exists and no annotation-based conditions
					cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineConditionCreated)
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				})
			})
		})
	})

	Context("Network", func() {

		Context("PrimaryIP", func() {
			const (
				invalid  = "abc"
				validIP4 = "192.168.0.2"
				validIP6 = "FD00:F53B:82E4::54"
			)
			BeforeEach(func() {
				vmCtx.MoVM.Guest = &vimtypes.GuestInfo{}
				vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
					},
				}
				vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
			})

			Context("IP4", func() {
				When("address is invalid", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = invalid
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is unspecified", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "0.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local multicast", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "224.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local unicast", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "169.254.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is loopback", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "127.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is ip6", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = validIP6
					})
					Specify("status.network.primaryIP4 should be empty", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(BeEmpty())
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(Equal(validIP6))
					})
				})
				When("address is valid", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = validIP4
					})
					Specify("status.network.primaryIP4 should be expected value", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(validIP4))
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(BeEmpty())
					})
				})
			})

			Context("IP6", func() {
				When("address is invalid", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = invalid
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is unspecified", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "::0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local multicast", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "FF02:::"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local unicast", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "FE80:::"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is loopback", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = "::1"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is ip4", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = validIP4
					})
					Specify("status.network.primaryIP6 should be empty", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(validIP4))
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(BeEmpty())
					})
				})
				When("address is valid", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Guest.IpAddress = validIP6
					})
					Specify("status.network.primaryIP6 should be expected value", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(BeEmpty())
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(Equal(validIP6))
					})
				})
			})

			Context("CloudInit", func() {
				const (
					localIP4 = "192.168.0.200"
					localIP6 = "FD00:F53B:82E4::5F"
				)

				BeforeEach(func() {
					vmCtx.MoVM.Guest.IpAddress = validIP4
					vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{}
					vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
					}
				})

				Context("ExtraConfig contains local-ip values with valid IPs", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   constants.CloudInitGuestInfoLocalIPv4Key,
								Value: localIP4,
							},
							&vimtypes.OptionValue{
								Key:   constants.CloudInitGuestInfoLocalIPv6Key,
								Value: localIP6,
							},
						}
					})
					It("status.network.primaryIP4/6 should be expected values", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(localIP4))
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(Equal(localIP6))
					})
				})

				Context("ExtraConfig contains local-ip values with invalid IPs", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   constants.CloudInitGuestInfoLocalIPv4Key,
								Value: invalid,
							},
							&vimtypes.OptionValue{
								Key:   constants.CloudInitGuestInfoLocalIPv6Key,
								Value: invalid,
							},
						}
					})
					It("status.network.primaryIP4/6 should be expected values", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(validIP4))
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(BeEmpty())
					})
				})
			})
		})

		Context("Interfaces", func() {
			Context("VM has pseudo devices", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{
						Net: []vimtypes.GuestNicInfo{
							{
								DeviceConfigId: -1,
								MacAddress:     "mac-1",
							},
							{
								DeviceConfigId: 4000,
								MacAddress:     "mac-4000",
							},
						},
					}

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth42",
						},
					}

					data.NetworkDeviceKeysToSpecIdx[4000] = 0
				})

				It("Skips pseudo devices", func() {
					network := vmCtx.VM.Status.Network
					Expect(network).ToNot(BeNil())

					Expect(network.Interfaces).To(HaveLen(1))
					Expect(network.Interfaces[0].Name).To(Equal("eth42"))
					Expect(network.Interfaces[0].IP).ToNot(BeNil())
					Expect(network.Interfaces[0].IP.MACAddr).To(Equal("mac-4000"))
				})
			})

			Context("VM has more interfaces than expected", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{
						Net: []vimtypes.GuestNicInfo{
							{
								DeviceConfigId: 4000,
								MacAddress:     "mac-4000",
							},
							{
								DeviceConfigId: 4001,
								MacAddress:     "mac-4001",
							},
						},
					}

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth42",
						},
					}

					data.NetworkDeviceKeysToSpecIdx[4000] = 0
				})

				It("Reports all interfaces", func() {
					network := vmCtx.VM.Status.Network
					Expect(network).ToNot(BeNil())

					Expect(network.Interfaces).To(HaveLen(2))
					Expect(network.Interfaces[0].Name).To(Equal("eth42"))
					Expect(network.Interfaces[0].IP).ToNot(BeNil())
					Expect(network.Interfaces[0].IP.MACAddr).To(Equal("mac-4000"))
					Expect(network.Interfaces[1].Name).To(BeEmpty())
					Expect(network.Interfaces[1].IP).ToNot(BeNil())
					Expect(network.Interfaces[1].IP.MACAddr).To(Equal("mac-4001"))
				})
			})

			Context("VM has multiple interfaces", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{
						Net: []vimtypes.GuestNicInfo{
							{
								DeviceConfigId: 4000,
								MacAddress:     "mac-4000",
							},
							{
								DeviceConfigId: 4001,
								MacAddress:     "mac-4001",
							},
						},
					}

					vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth42",
						},
						{
							Name: "eth99",
						},
					}

					data.NetworkDeviceKeysToSpecIdx[4000] = 1
					data.NetworkDeviceKeysToSpecIdx[4001] = 0
				})

				It("Reports all interfaces", func() {
					network := vmCtx.VM.Status.Network
					Expect(network).ToNot(BeNil())

					Expect(network.Interfaces).To(HaveLen(2))
					Expect(network.Interfaces[0].Name).To(Equal("eth99"))
					Expect(network.Interfaces[0].IP).ToNot(BeNil())
					Expect(network.Interfaces[0].IP.MACAddr).To(Equal("mac-4000"))
					Expect(network.Interfaces[1].Name).To(Equal("eth42"))
					Expect(network.Interfaces[1].IP).ToNot(BeNil())
					Expect(network.Interfaces[1].IP.MACAddr).To(Equal("mac-4001"))
				})
			})
		})

		Context("DNS", func() {
			When("DNSConfig has duplicate IpAddress and duplicate SearchDomain", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{
						IpStack: []vimtypes.GuestStackInfo{
							{
								DnsConfig: &vimtypes.NetDnsConfigInfo{
									HostName:   "my-vm",
									DomainName: "local.domain",
									IpAddress: []string{
										"10.211.0.1",
										"10.211.0.2",
										"10.211.0.1",
										"10.211.0.2",
									},
									SearchDomain: []string{
										"foo.local", "bar.local",
										"foo.local", "bar.local",
									},
								},
							},
						},
					}
				})

				It("Skips duplicate Nameservers and duplicate SearchDomain", func() {
					network := vmCtx.VM.Status.Network
					Expect(network).ToNot(BeNil())
					Expect(network.IPStacks).To(HaveLen(1))
					Expect(network.IPStacks[0].DNS).ToNot(BeNil())
					Expect(network.IPStacks[0].DNS.HostName).To(Equal("my-vm"))
					Expect(network.IPStacks[0].DNS.DomainName).To(Equal("local.domain"))
					Expect(network.IPStacks[0].DNS.Nameservers).To(HaveLen(2))
					Expect(network.IPStacks[0].DNS.Nameservers).To(Equal([]string{"10.211.0.1", "10.211.0.2"}))
					Expect(network.IPStacks[0].DNS.SearchDomains).To(HaveLen(2))
					Expect(network.IPStacks[0].DNS.SearchDomains).To(Equal([]string{"foo.local", "bar.local"}))
				})
			})
		})

		Context("IPRoutes", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Guest = &vimtypes.GuestInfo{
					IpStack: []vimtypes.GuestStackInfo{
						{
							IpRouteConfig: &vimtypes.NetIpRouteConfigInfo{
								IpRoute: []vimtypes.NetIpRouteConfigInfoIpRoute{
									{
										Network:      "192.168.1.0",
										PrefixLength: 24,
									},
									{
										Network:      "192.168.1.100",
										PrefixLength: 32,
									},
									{
										Network:      "fe80::",
										PrefixLength: 64,
									},
									{
										Network:      "ff00::",
										PrefixLength: 8,
									},
									{
										Network:      "e9ef:6df5:eb14:42e2:5c09:9982:a9b5:8c2b",
										PrefixLength: 48,
									},
								},
							},
						},
					},
				}
			})

			It("Skips IPs", func() {
				network := vmCtx.VM.Status.Network
				Expect(network).ToNot(BeNil())
				Expect(network.IPStacks).To(HaveLen(1))
				Expect(network.IPStacks[0].IPRoutes).To(HaveLen(2))
				Expect(network.IPStacks[0].IPRoutes[0].NetworkAddress).To(Equal("192.168.1.0/24"))
				Expect(network.IPStacks[0].IPRoutes[1].NetworkAddress).To(Equal("e9ef:6df5:eb14:42e2:5c09:9982:a9b5:8c2b/48"))
			})
		})

		Context("Status.Network", func() {

			When("nil Guest property", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = nil
					vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
				})

				Specify("status.network should be nil", func() {
					Expect(vmCtx.VM.Status.Network).To(BeNil())
				})
			})

			When("Guest property is empty", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{}
					vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
				})

				Specify("status.network should be nil", func() {
					Expect(vmCtx.VM.Status.Network).To(BeNil())
				})
			})

			When("Empty guest property but has existing status.network.config", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Guest = &vimtypes.GuestInfo{}
					vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
						PrimaryIP4: "my-ipv4",
						PrimaryIP6: "my-ipv6",
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceStatus{
							{},
						},
						IPStacks: []vmopv1.VirtualMachineNetworkIPStackStatus{
							{},
						},
						Config: &vmopv1.VirtualMachineNetworkConfigStatus{
							Interfaces: []vmopv1.VirtualMachineNetworkConfigInterfaceStatus{
								{
									Name: "my-interface",
								},
							},
						},
					}
				})

				Specify("only status.network.config should be preserved", func() {
					network := vmCtx.VM.Status.Network
					Expect(network).ToNot(BeNil())

					Expect(network.PrimaryIP4).To(BeEmpty())
					Expect(network.PrimaryIP6).To(BeEmpty())
					Expect(network.Interfaces).To(BeNil())
					Expect(network.IPStacks).To(BeNil())
					Expect(network.Config).ToNot(BeNil())
					Expect(network.Config.Interfaces).To(HaveLen(1))
					Expect(network.Config.Interfaces[0].Name).To(Equal("my-interface"))
				})
			})
		})

	})

	Context("Storage", func() {
		const oneGiBInBytes = 1 /* B */ * 1024 /* KiB */ * 1024 /* MiB */ * 1024 /* GiB */

		Context("status.changeBlockTracking", func() {
			When("moVM.config is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config = nil
				})
				Specify("status.changeBlockTracking is nil", func() {
					Expect(vmCtx.VM.Status.ChangeBlockTracking).To(BeNil())
				})
			})
			When("moVM.config is not nil", func() {
				When("moVM.config.changeTrackingEnabled is nil", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
							ChangeTrackingEnabled: nil,
						}
					})
					Specify("status.changeBlockTracking is nil", func() {
						Expect(vmCtx.VM.Status.ChangeBlockTracking).To(BeNil())
					})
				})
				When("moVM.config.changeTrackingEnabled is true", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
							ChangeTrackingEnabled: ptr.To(true),
						}
					})
					Specify("status.changeBlockTracking is true", func() {
						Expect(vmCtx.VM.Status.ChangeBlockTracking).To(Equal(ptr.To(true)))
					})
				})
				When("moVM.config.changeTrackingEnabled is false", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
							ChangeTrackingEnabled: ptr.To(false),
						}
					})
					Specify("status.changeBlockTracking is false", func() {
						Expect(vmCtx.VM.Status.ChangeBlockTracking).To(Equal(ptr.To(false)))
					})
				})
			})
		})

		Context("status.storage", func() {
			When("status.volumes and moVM.layoutEx are nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.LayoutEx = nil
					vmCtx.VM.Status.Volumes = nil
				})
				When("status.storage is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = nil
					})
					Specify("status.storage to be unchanged", func() {
						Expect(vmCtx.VM.Status.Storage).To(BeNil())
					})
				})
				When("status.storage is not nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = &vmopv1.VirtualMachineStorageStatus{}
					})
					Specify("status.storage to be unchanged", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{}))
					})
				})
			})
			When("status.volumes is not nil but moVM.layoutEx is", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Type:      vmopv1.VolumeTypeClassic,
							Requested: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Limit:     kubeutil.BytesToResource(50 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(10 * oneGiBInBytes),
						},
						{
							Type:      vmopv1.VolumeTypeManaged,
							Requested: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Limit:     kubeutil.BytesToResource(50 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(20 * oneGiBInBytes),
						},
					}
					vmCtx.MoVM.LayoutEx = nil
				})
				When("status.storage is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = nil
					})
					Specify("status.storage to be initialized", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
							},
						}))
					})
				})
				When("status.storage is not nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = &vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(1024 + (20 * oneGiBInBytes)),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(20 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Other: kubeutil.BytesToResource(1 * oneGiBInBytes),
							},
						}
					})
					Specify("status.storage to be updated", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
							},
						}))
					})
				})
			})
			When("status.volumes and moVM.layoutEx are not nil", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Type:      vmopv1.VolumeTypeClassic,
							Requested: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Limit:     kubeutil.BytesToResource(50 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(10 * oneGiBInBytes),
						},
						{
							Type:      vmopv1.VolumeTypeManaged,
							Requested: kubeutil.BytesToResource(50 * oneGiBInBytes),
							Limit:     kubeutil.BytesToResource(50 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(20 * oneGiBInBytes),
						},
					}
					vmCtx.MoVM.LayoutEx = &vimtypes.VirtualMachineFileLayoutEx{
						File: []vimtypes.VirtualMachineFileLayoutExFileInfo{
							{
								Type:       string(vimtypes.VirtualMachineFileLayoutExFileTypeDiskExtent),
								UniqueSize: 10 * oneGiBInBytes,
							},
							{
								Type:       string(vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotData),
								UniqueSize: 1 * oneGiBInBytes,
							},
							{
								Type:       string(vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotList),
								UniqueSize: 10 * oneGiBInBytes,
							},
							{
								Type:       string(vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotMemory),
								UniqueSize: 10 * oneGiBInBytes,
							},
						},
					}
				})
				When("status.storage is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = nil
					})
					Specify("status.storage to be initialized", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(71 * oneGiBInBytes),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
								Other: kubeutil.BytesToResource(21 * oneGiBInBytes),
							},
						}))
					})
					When("VMSnapshot feature is enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMSnapshots = true
							})
						})
						Specify("status.storage to be initialized, snapshot related files are not included", func() {
							Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
								Total: kubeutil.BytesToResource(50 * oneGiBInBytes),
								Requested: &vmopv1.VirtualMachineStorageStatusRequested{
									Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
								},
								Used: &vmopv1.VirtualMachineStorageStatusUsed{
									Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
								},
							}))
						})
					})
				})
				When("status.storage is not nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Status.Storage = &vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(1024 + (5 * oneGiBInBytes)),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(5 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Other: kubeutil.BytesToResource(1024),
							},
						}
					})
					Specify("status.storage to be updated", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(71 * oneGiBInBytes),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
								Other: kubeutil.BytesToResource(21 * oneGiBInBytes),
							},
						}))
					})

					When("VMSnapshot feature is enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
								config.Features.VMSnapshots = true
							})
						})
						Specify("status.storage to be updated, snapshot related files are not included", func() {
							Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
								Total: kubeutil.BytesToResource(50 * oneGiBInBytes),
								Requested: &vmopv1.VirtualMachineStorageStatusRequested{
									Disks: kubeutil.BytesToResource(50 * oneGiBInBytes),
								},
								Used: &vmopv1.VirtualMachineStorageStatusUsed{
									Disks: kubeutil.BytesToResource(10 * oneGiBInBytes),
								},
							}))
						})
					})
				})
			})
		})

		Context("status.volumes", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							&vimtypes.ParaVirtualSCSIController{
								VirtualSCSIController: vimtypes.VirtualSCSIController{
									VirtualController: vimtypes.VirtualController{
										BusNumber: 0,
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 200,
										},
									},
								},
							},

							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-100.vmdk",
										},
										Uuid: "100",
										KeyId: &vimtypes.CryptoKeyId{
											KeyId: "my-key-id",
											ProviderId: &vimtypes.KeyProviderId{
												Id: "my-provider-id",
											},
										},
									},
									Key:           100,
									ControllerKey: 200,
									UnitNumber:    ptr.To[int32](3),
								},
								CapacityInBytes: 10 * oneGiBInBytes,
							},
							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-101.vmdk",
										},
										Uuid: "101",
									},
									Key:        101,
									UnitNumber: ptr.To[int32](0),
								},
								CapacityInBytes: 1 * oneGiBInBytes,
							},
							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-102.vmdk",
										},
										Uuid: "102",
									},
									Key:        102,
									UnitNumber: ptr.To[int32](0),
								},
								CapacityInBytes: 2 * oneGiBInBytes,
							},
							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-103.vmdk",
										},
										Uuid: "103",
									},
									Key:        103,
									UnitNumber: ptr.To[int32](0),
								},
								CapacityInBytes: 3 * oneGiBInBytes,
							},
							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{
										DescriptorFileName: "[datastore] vm/my-disk-104.vmdk",
										Uuid:               "104",
									},
									Key:        104,
									UnitNumber: ptr.To[int32](0),
								},
								CapacityInBytes: 4 * oneGiBInBytes,
							},
							// managed
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-105.vmdk",
										},
										Uuid: "105",
										KeyId: &vimtypes.CryptoKeyId{
											KeyId: "my-key-id",
											ProviderId: &vimtypes.KeyProviderId{
												Id: "my-provider-id",
											},
										},
									},
									Key:        105,
									UnitNumber: ptr.To[int32](0),
								},
								CapacityInBytes: 5 * oneGiBInBytes,
								VDiskId: &vimtypes.ID{
									Id: "my-fcd-1",
								},
							},
						},
					},
				}
				vmCtx.MoVM.LayoutEx = &vimtypes.VirtualMachineFileLayoutEx{
					Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
						{
							Key: 100,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{0, 10},
								},
							},
						},
						{
							Key: 101,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{1, 11},
								},
							},
						},
						{
							Key: 102,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{2, 12},
								},
							},
						},
						{
							Key: 103,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{3, 13},
								},
							},
						},
						{
							Key: 104,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{4, 14},
								},
							},
						},
						{
							Key: 105,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{5, 15},
								},
							},
						},
					},
					File: []vimtypes.VirtualMachineFileLayoutExFileInfo{
						{
							Key:        0,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        10,
							Size:       2 * oneGiBInBytes,
							UniqueSize: 1 * oneGiBInBytes,
						},

						{
							Key:        1,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        11,
							Size:       0.5 * oneGiBInBytes,
							UniqueSize: 0.25 * oneGiBInBytes,
						},

						{
							Key:        2,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        12,
							Size:       1 * oneGiBInBytes,
							UniqueSize: 0.5 * oneGiBInBytes,
						},

						{
							Key:        3,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        13,
							Size:       2 * oneGiBInBytes,
							UniqueSize: 1 * oneGiBInBytes,
						},

						{
							Key:        4,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        14,
							Size:       3 * oneGiBInBytes,
							UniqueSize: 2 * oneGiBInBytes,
						},

						{
							Key:        5,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        15,
							Size:       50 * oneGiBInBytes,
							UniqueSize: 50 * oneGiBInBytes,
						},
					},
				}
				vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{}
			})
			When("moVM.config is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config = nil
				})
				Specify("status.volumes is unchanged", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{}))
				})
			})
			When("moVM.config.hardware.device is empty", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = nil
				})
				Specify("status.volumes is unchanged", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{}))
				})
			})
			When("moVM.layoutEx is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.LayoutEx = nil
				})
				Specify("status.volumes is unchanged", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{}))
				})
			})
			When("moVM.layoutEx.disk is empty", func() {
				BeforeEach(func() {
					vmCtx.MoVM.LayoutEx.Disk = nil
				})
				Specify("status.volumes is unchanged", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{}))
				})
			})
			When("moVM.layoutEx.file is empty", func() {
				BeforeEach(func() {
					vmCtx.MoVM.LayoutEx.Disk = nil
				})
				Specify("status.volumes is unchanged", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{}))
				})
			})
			When("vm.status.volumes does not have pvcs", func() {
				Specify("status.volumes includes the classic disks", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
					}))
				})
			})

			When("vm.status.volumes has a pvc", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      "my-disk-105",
							DiskUUID:  "105",
							Type:      vmopv1.VolumeTypeManaged,
							Attached:  false,
							Limit:     kubeutil.BytesToResource(100 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(100 * oneGiBInBytes),
						},
					}
				})
				Specify("status.volumes includes the pvc and classic disks", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
						{
							Name:     "my-disk-105",
							DiskUUID: "105",
							Type:     vmopv1.VolumeTypeManaged,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached:  false,
							Limit:     kubeutil.BytesToResource(100 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(100 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (50 * oneGiBInBytes)),
						},
					}))
				})
			})

			When("vm.status.volumes has a stale (no longer exists) classic disk", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      pkgutil.GeneratePVCName("disk", "106"),
							DiskUUID:  "106",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
						},
					}
				})
				Specify("status.volumes no longer includes the stale classic disk", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								ProviderID: "my-provider-id",
								KeyID:      "my-key-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
					}))
				})
			})

			When("vm.status.volumes has a stale (not in spec) classic disk", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: pkgutil.GeneratePVCName("disk", "100"),
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									ControllerType:      vmopv1.VirtualControllerTypeSCSI,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](3),
								},
							},
						},
					}
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      "my-old-name",
							DiskUUID:  "100",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
						},
					}
				})
				Specify("status.volumes no longer includes the stale classic disk", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								ProviderID: "my-provider-id",
								KeyID:      "my-key-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
					}))
				})
			})

			When("there is a classic disk w an invalid path", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.
						Device[1].(*vimtypes.VirtualDisk).
						Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).
						FileName = "invalid"
				})
				Specify("status.volumes omits the classic disk w invalid path", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
					}))
				})
			})

			When("brownfield vm was upgraded from v1alpha3 VM, it has a classic volume which doesn't have 'requested'", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID: "104",
							Type:     vmopv1.VolumeTypeClassic, // requested type
							Attached: true,
							Limit:    kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:     kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
							// No requested.
						},
					}
				})
				Specify("status.volumes includes this volume and its requested is patched", func() {
					Expect(vmCtx.VM.Status.Volumes).To(HaveLen(5))
					Expect(vmCtx.VM.Status.Volumes[4]).To(Equal(
						vmopv1.VirtualMachineVolumeStatus{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes), // Patched.
							Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
						},
					))
				})
			})

			When("brownfield vm was upgraded from v1alpha3 VM, it has a managed volume which doesn't has 'requested'", func() {
				BeforeEach(func() {
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     "my-disk-105",
							DiskUUID: "105",
							Type:     vmopv1.VolumeTypeManaged,
							Attached: false,
							Limit:    kubeutil.BytesToResource(100 * oneGiBInBytes),
							// No requested.
						},
					}
				})
				Specify("status.volumes includes this volume but skip patching its used since it should be patched by volume controller", func() {
					Expect(vmCtx.VM.Status.Volumes).To(HaveLen(6))
					Expect(vmCtx.VM.Status.Volumes[5]).To(Equal(
						vmopv1.VirtualMachineVolumeStatus{
							Name:     "my-disk-105",
							DiskUUID: "105",
							Type:     vmopv1.VolumeTypeManaged,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached: false,
							Limit:    kubeutil.BytesToResource(100 * oneGiBInBytes),
							Used:     kubeutil.BytesToResource(500 + (50 * oneGiBInBytes)),
							// No requested.
						},
					))
				})
			})

			When("disk has multiple chains", func() {
				BeforeEach(func() {
					vmCtx.MoVM.LayoutEx.Disk = []vimtypes.VirtualMachineFileLayoutExDiskLayout{
						{
							Key: 100,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{1, 11},
								},
								{
									FileKey: []int32{0, 10},
								},
							},
						},
						{
							Key: 101,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{0, 10},
								},
								{
									FileKey: []int32{1, 11},
								},
							},
						},
						{
							Key: 102,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{1, 11},
								},
								{
									FileKey: []int32{2, 12},
								},
							},
						},
						{
							Key: 103,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{2, 12},
								},
								{
									FileKey: []int32{3, 13},
								},
							},
						},
						{
							Key: 104,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{3, 13},
								},
								{
									FileKey: []int32{4, 14},
								},
							},
						},
						{
							Key: 105,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{4, 14},
								},
								{
									FileKey: []int32{5, 15},
								},
							},
						},
					}
				})
				Specify("status.volumes is calculated", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + 1*oneGiBInBytes + 500 + 0.25*oneGiBInBytes),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + 0.25*oneGiBInBytes + 500 + 1*oneGiBInBytes),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + 0.5*oneGiBInBytes + 500 + 0.25*oneGiBInBytes),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + 1*oneGiBInBytes + 500 + 0.5*oneGiBInBytes),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(500 + 2*oneGiBInBytes + 500 + 1*oneGiBInBytes),
						},
					}))
				})
				When("VMSnapshot feature is enabled", func() {
					BeforeEach(func() {
						pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
							config.Features.VMSnapshots = true
						})

						vmCtx.MoVM.LayoutEx.Snapshot = []vimtypes.VirtualMachineFileLayoutExSnapshotLayout{
							{
								Key: vimtypes.ManagedObjectReference{
									Type:  "Snapshot",
									Value: "Snapshot-1",
								},
								Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
									{
										// classic disk
										Key: 100,
										Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
											{
												FileKey: []int32{3, 4}, // 500 + 500
											},
										},
									},
									{
										// managed disk
										Key: 105,
										Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
											{
												FileKey: []int32{13, 14}, // 5 Gib
											},
										},
									},
								},
								DataKey:   10, // 2 Gib
								MemoryKey: -1,
							},
							{
								Key: vimtypes.ManagedObjectReference{
									Type:  "Snapshot",
									Value: "Snapshot-2",
								},
								Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
									{
										// classic disk
										Key: 101,
										Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
											{
												FileKey: []int32{1, 2}, // 500 + 500
											},
										},
									},
									{
										// managed disk
										Key: 105,
										Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
											{
												FileKey: []int32{11, 12}, // 1.5 Gib
											},
										},
									},
								},
								DataKey:   5, // 500
								MemoryKey: 4, // 500
							},
						}
					})
					Specify("status.volumes is calculated, and value of 'used' only includes the files in the last chain", func() {
						Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     pkgutil.GeneratePVCName("disk", "100"),
								DiskUUID: "100",
								Type:     vmopv1.VolumeTypeClassic,
								Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
									KeyID:      "my-key-id",
									ProviderID: "my-provider-id",
								},
								Attached:  true,
								Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
								Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
								Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
							},
							{
								Name:      pkgutil.GeneratePVCName("disk", "101"),
								DiskUUID:  "101",
								Type:      vmopv1.VolumeTypeClassic,
								Attached:  true,
								Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
								Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
								Used:      kubeutil.BytesToResource(500 + (0.25 * oneGiBInBytes)),
							},
							{
								Name:      pkgutil.GeneratePVCName("disk", "102"),
								DiskUUID:  "102",
								Type:      vmopv1.VolumeTypeClassic,
								Attached:  true,
								Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
								Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
								Used:      kubeutil.BytesToResource(500 + (0.5 * oneGiBInBytes)),
							},
							{
								Name:      pkgutil.GeneratePVCName("disk", "103"),
								DiskUUID:  "103",
								Type:      vmopv1.VolumeTypeClassic,
								Attached:  true,
								Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
								Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
								Used:      kubeutil.BytesToResource(500 + (1 * oneGiBInBytes)),
							},
							{
								Name:      pkgutil.GeneratePVCName("disk", "104"),
								DiskUUID:  "104",
								Type:      vmopv1.VolumeTypeClassic,
								Attached:  true,
								Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
								Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
								Used:      kubeutil.BytesToResource(500 + (2 * oneGiBInBytes)),
							},
						}))
					})
					Specify("status.storage is calculated, it contains snapshot related usage in Snapshot field", func() {
						Expect(vmCtx.VM.Status.Storage).To(Equal(&vmopv1.VirtualMachineStorageStatus{
							Total: kubeutil.BytesToResource(74.75*oneGiBInBytes + 3000),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: kubeutil.BytesToResource(20 * oneGiBInBytes),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: kubeutil.BytesToResource(4.75*oneGiBInBytes + 2500),
								Snapshots: &vmopv1.VirtualMachineStorageStatusUsedSnapshotDetails{
									// 1, 2, 3, 4, 4, 5, 10
									VM: kubeutil.BytesToResource(3000 + 2*oneGiBInBytes),
									// 11, 12, 13, 14
									Volume: kubeutil.BytesToResource(6.5 * oneGiBInBytes),
								},
								Other: kubeutil.BytesToResource(54.75*oneGiBInBytes + 3000),
							},
						}))
					})
				})
			})

			When("moVM.layoutEx.disk.chain is empty", func() {
				BeforeEach(func() {
					for i := range vmCtx.MoVM.LayoutEx.Disk {
						vmCtx.MoVM.LayoutEx.Disk[i].Chain = nil
					}
				})
				Specify("status.volumes is calculated, and value of 'used' is 0", func() {
					Expect(vmCtx.VM.Status.Volumes).To(Equal([]vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     pkgutil.GeneratePVCName("disk", "100"),
							DiskUUID: "100",
							Type:     vmopv1.VolumeTypeClassic,
							Crypto: &vmopv1.VirtualMachineVolumeCryptoStatus{
								KeyID:      "my-key-id",
								ProviderID: "my-provider-id",
							},
							Attached:  true,
							Limit:     kubeutil.BytesToResource(10 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(10 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(0),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "101"),
							DiskUUID:  "101",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(1 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(1 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(0),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "102"),
							DiskUUID:  "102",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(2 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(2 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(0),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "103"),
							DiskUUID:  "103",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(3 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(3 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(0),
						},
						{
							Name:      pkgutil.GeneratePVCName("disk", "104"),
							DiskUUID:  "104",
							Type:      vmopv1.VolumeTypeClassic,
							Attached:  true,
							Limit:     kubeutil.BytesToResource(4 * oneGiBInBytes),
							Requested: kubeutil.BytesToResource(4 * oneGiBInBytes),
							Used:      kubeutil.BytesToResource(0),
						},
					}))
				})
			})
		})
	})

	Context("PowerState", func() {
		Context("status.powerState", func() {
			When("VM runtime power state is PoweredOn", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				})

				When("spec.powerState is PoweredOn (synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})

					It("should set status.powerState to PoweredOn", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					})

					It("should set VirtualMachinePowerStateSynced condition to True", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						Expect(cond.Reason).To(Equal(string(vmopv1.VirtualMachinePowerStateOn)))
						Expect(cond.Message).To(BeEmpty())
					})
				})

				When("spec.powerState is PoweredOff (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})

					It("should set status.powerState to PoweredOn", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=PoweredOff"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=PoweredOn"))
					})
				})

				When("spec.powerState is Suspended (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})

					It("should set status.powerState to PoweredOn", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=Suspended"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=PoweredOn"))
					})
				})
			})

			When("VM runtime power state is PoweredOff", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOff
				})

				When("spec.powerState is PoweredOff (synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})

					It("should set status.powerState to PoweredOff", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})

					It("should set VirtualMachinePowerStateSynced condition to True", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						Expect(cond.Reason).To(Equal(string(vmopv1.VirtualMachinePowerStateOff)))
						Expect(cond.Message).To(BeEmpty())
					})
				})

				When("spec.powerState is PoweredOn (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})

					It("should set status.powerState to PoweredOff", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=PoweredOn"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=PoweredOff"))
					})
				})

				When("spec.powerState is Suspended (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})

					It("should set status.powerState to PoweredOff", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=Suspended"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=PoweredOff"))
					})
				})
			})

			When("VM runtime power state is Suspended", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStateSuspended
				})

				When("spec.powerState is Suspended (synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})

					It("should set status.powerState to Suspended", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
					})

					It("should set VirtualMachinePowerStateSynced condition to True", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						Expect(cond.Reason).To(Equal(string(vmopv1.VirtualMachinePowerStateSuspended)))
						Expect(cond.Message).To(BeEmpty())
					})
				})

				When("spec.powerState is PoweredOn (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})

					It("should set status.powerState to Suspended", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=PoweredOn"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=Suspended"))
					})
				})

				When("spec.powerState is PoweredOff (not synced)", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})

					It("should set status.powerState to Suspended", func() {
						Expect(vmCtx.VM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
					})

					It("should set VirtualMachinePowerStateSynced condition to False", func() {
						cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachinePowerStateSynced)
						Expect(cond).ToNot(BeNil())
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						Expect(cond.Reason).To(Equal("NotSynced"))
						Expect(cond.Message).To(ContainSubstring("spec.powerState=PoweredOff"))
						Expect(cond.Message).To(ContainSubstring("status.powerState=Suspended"))
					})
				})
			})
		})
	})

	Context("Controllers", func() {
		When("moVM.Config is nil", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = nil
			})
			Specify("status.hardware.controllers should be empty", func() {
				if vmCtx.VM.Status.Hardware != nil {
					Expect(vmCtx.VM.Status.Hardware.Controllers).To(BeEmpty())
				}
			})
		})

		When("moVM.Config.Hardware.Device is empty", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{},
					},
				}
			})
			Specify("status.hardware.controllers should be empty", func() {
				if vmCtx.VM.Status.Hardware != nil {
					Expect(vmCtx.VM.Status.Hardware.Controllers).To(BeEmpty())
				}
			})
		})

		When("VM has various controller types", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
					builder.DummyIDEController(200, 0, nil),
					builder.DummySCSIController(1000, 0),
					builder.DummySATAController(15000, 0, nil),
					builder.DummyNVMEController(20000, 0),
				)
			})
			Specify("status.hardware.controllers should contain all controller types", func() {
				Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Hardware.Controllers).To(HaveLen(4))

				controllerTypes := make([]vmopv1.VirtualControllerType, 0, 4)
				controllerKeys := make([]int32, 0, 4)
				for _, controller := range vmCtx.VM.Status.Hardware.Controllers {
					controllerTypes = append(controllerTypes, controller.Type)
					controllerKeys = append(controllerKeys, controller.DeviceKey)
					Expect(controller.BusNumber).To(Equal(int32(0)))
				}

				Expect(controllerTypes).To(ContainElements(
					vmopv1.VirtualControllerTypeIDE,
					vmopv1.VirtualControllerTypeSCSI,
					vmopv1.VirtualControllerTypeSATA,
					vmopv1.VirtualControllerTypeNVME,
				))

				Expect(controllerKeys).To(ContainElements(
					int32(200),
					int32(1000),
					int32(15000),
					int32(20000),
				))
			})
		})

		When("VM has controllers with attached devices", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							&vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 1000,
									},
									BusNumber: 0,
								},
							},
							builder.DummyVirtualDisk(2000, 1000, ptr.To(int32(0)), "test-uuid-123", ""),
							&vimtypes.VirtualIDEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 200,
									},
									BusNumber: 0,
								},
							},
							builder.DummyCdromDevice(3000, 200, 0, "/vmfs/volumes/datastore1/vm1/ubuntu.iso"),
						},
					},
				}
			})
			Specify("status.hardware.controllers should contain controllers with their devices", func() {
				Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Hardware.Controllers).To(HaveLen(2))

				var scsiController, ideController *vmopv1.VirtualControllerStatus
				for i := range vmCtx.VM.Status.Hardware.Controllers {
					controller := &vmCtx.VM.Status.Hardware.Controllers[i]
					switch controller.Type {
					case vmopv1.VirtualControllerTypeSCSI:
						scsiController = controller
					case vmopv1.VirtualControllerTypeIDE:
						ideController = controller
					}
				}

				Expect(scsiController).ToNot(BeNil())
				Expect(scsiController.DeviceKey).To(Equal(int32(1000)))
				Expect(scsiController.BusNumber).To(Equal(int32(0)))
				Expect(scsiController.Devices).To(HaveLen(1))
				Expect(scsiController.Devices[0].UnitNumber).To(Equal(int32(0)))
				Expect(scsiController.Devices[0].Type).To(Equal(vmopv1.VirtualDeviceTypeDisk))

				Expect(ideController).ToNot(BeNil())
				Expect(ideController.DeviceKey).To(Equal(int32(200)))
				Expect(ideController.BusNumber).To(Equal(int32(0)))
				Expect(ideController.Devices).To(HaveLen(1))
				Expect(ideController.Devices[0].UnitNumber).To(Equal(int32(0)))
				Expect(ideController.Devices[0].Type).To(Equal(vmopv1.VirtualDeviceTypeCDROM))
			})
		})

		Context("HardwareCondition", func() {
			// Constants for common condition messages
			const (
				aggregateMessagePrefix = "Hardware configuration issues detected. See"
				aggregateMessageSuffix = "conditions for details."
			)

			// Child condition metadata for verification
			childConditionInfo := map[string]struct {
				conditionType string
				falseReason   string
			}{
				vmopv1.VirtualMachineHardwareControllersVerified: {
					conditionType: vmopv1.VirtualMachineHardwareControllersVerified,
					falseReason:   vmopv1.VirtualMachineHardwareControllersMismatchReason,
				},
				vmopv1.VirtualMachineHardwareVolumesVerified: {
					conditionType: vmopv1.VirtualMachineHardwareVolumesVerified,
					falseReason:   vmopv1.VirtualMachineHardwareVolumesMismatchReason,
				},
				vmopv1.VirtualMachineHardwareCDROMVerified: {
					conditionType: vmopv1.VirtualMachineHardwareCDROMVerified,
					falseReason:   vmopv1.VirtualMachineHardwareCDROMMismatchReason,
				},
			}

			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMSharedDisks = true
				})
				// Clear MoVM.Config so tests can set it up as needed.
				// reconcileStatusController() will populate status from MoVM.Config.Hardware.Device.
				vmCtx.MoVM.Config = nil
				// Clear default volumes, CD-ROMs, and controllers from DummyVirtualMachine.
				vmCtx.VM.Spec.Volumes = nil
				vmCtx.VM.Spec.Hardware = nil
			})

			// Helper function to build aggregate condition message from condition types.
			buildAggregateMessage := func(conditionTypes ...string) string {
				if len(conditionTypes) == 0 {
					return ""
				}
				return fmt.Sprintf("%s %s %s", aggregateMessagePrefix, strings.Join(conditionTypes, ", "), aggregateMessageSuffix)
			}

			// Helper function to verify a child condition with expected message or True status.
			verifyChildCondition := func(
				conditionType string,
				expectedFalseReason string,
				expectedMessage string,
				shouldHaveIssues bool) {
				childCond := conditions.Get(vmCtx.VM, conditionType)
				if shouldHaveIssues {
					Expect(childCond).ToNot(BeNil())
					Expect(childCond.Status).To(Equal(metav1.ConditionFalse))
					Expect(childCond.Reason).To(Equal(expectedFalseReason))
					Expect(childCond.Message).To(Equal(expectedMessage))
				} else {
					// Verify that condition without issues is True.
					if childCond != nil {
						Expect(childCond.Status).To(Equal(metav1.ConditionTrue))
					}
				}
			}

			// Helper function to assert condition is false with expected message.
			// Also verifies that child conditions are set correctly with exact detailed messages.
			// expectedChildMessages is a map from condition type to expected message string.
			assertConditionFalse := func(expectedMessage string, expectedChildMessages map[string]string) {
				cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineHardwareDeviceConfigVerified)
				Expect(cond).ToNot(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(vmopv1.VirtualMachineHardwareDeviceConfigMismatchReason))
				Expect(cond.Message).To(Equal(expectedMessage))

				// Verify all child conditions using table-driven approach.
				for conditionType, info := range childConditionInfo {
					if expectedMsg, ok := expectedChildMessages[conditionType]; ok {
						verifyChildCondition(info.conditionType, info.falseReason, expectedMsg, true)
					} else {
						verifyChildCondition(info.conditionType, "", "", false)
					}
				}
			}

			// Helper function to assert condition is true and all child conditions are true.
			assertConditionTrue := func() {
				cond := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineHardwareDeviceConfigVerified)
				Expect(cond).To(HaveValue(HaveField("Status", Equal(metav1.ConditionTrue))))

				// Verify all child conditions are True using table-driven approach.
				for _, info := range childConditionInfo {
					childCond := conditions.Get(vmCtx.VM, info.conditionType)
					if childCond != nil {
						Expect(childCond.Status).To(Equal(metav1.ConditionTrue))
					}
				}
			}

			// Helper function to set up SCSI controller in spec.
			setupSCSIControllerInSpec := func(busNumber int32) {
				if vmCtx.VM.Spec.Hardware == nil {
					vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				vmCtx.VM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{BusNumber: busNumber},
				}
			}

			// Helper function to set up IDE controller in spec.
			setupIDEControllerInSpec := func(busNumber int32) {
				if vmCtx.VM.Spec.Hardware == nil {
					vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				vmCtx.VM.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: busNumber},
				}
			}

			// Helper function to create a basic PVC volume setup.
			setupPVCVolume := func(volumeName, pvcName string, controllerType vmopv1.VirtualControllerType, busNumber, unitNumber int32) {
				vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					builder.DummyPVCVirtualMachineVolume(
						volumeName,
						pvcName,
						controllerType,
						ptr.To(busNumber),
						ptr.To(unitNumber),
					),
				}
			}

			// Helper function to set up CD-ROM image objects.
			setupCDROMImage := func(vmiName, vmiFileName string) {
				vmCtx.VM.Namespace = ctx.PodNamespace
				k8sObjs := builder.DummyImageAndItemObjectsForCdromBacking(
					vmiName, ctx.PodNamespace, "VirtualMachineImage", vmiFileName,
					"lib-item-uuid", true, true,
					resource.MustParse("100Mi"),
					true, true, imgregv1a1.ContentLibraryItemTypeIso)
				for _, obj := range k8sObjs {
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				}
			}

			Context("when all checks pass", func() {
				When("SCSI and IDE controllers match", func() {
					BeforeEach(func() {
						// Set up matching controllers in spec and MoVM so status is populated correctly.
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{{BusNumber: 0}},
							IDEControllers:  []vmopv1.IDEControllerSpec{{BusNumber: 0}},
						}

						// Set up MoVM.Config.Hardware.Device to match the spec.
						// reconcileStatusController() will populate status from this.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
							builder.DummyIDEController(200, 0, nil),
						)
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("SATA controller matches", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SATAControllers: []vmopv1.SATAControllerSpec{
								{BusNumber: 0},
							},
						}

						// Set up MoVM.Config.Hardware.Device to have a SATA controller.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySATAController(15000, 0, nil),
						)
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("NVME controller matches", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							NVMEControllers: []vmopv1.NVMEControllerSpec{
								{BusNumber: 0},
							},
						}

						// Set up MoVM.Config.Hardware.Device to have an NVME controller.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyNVMEController(20000, 0),
						)
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("when there are no controllers in spec or status", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("when hardware is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = nil
						// MoVM.Config.Hardware.Device is already cleared in parent BeforeEach.
						// reconcileStatusController() will return early without setting status.
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})
			})

			Context("controller mismatches", func() {
				When("spec has controllers but moVM.Config is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{BusNumber: 0},
							},
						}
						vmCtx.MoVM.Config = nil
					})

					It("should mark condition as false with concise summary", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: SCSI:0",
							})
					})
				})

				When("status has controllers but spec hardware is nil", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = nil
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
						)
					})

					It("should mark the condition as false with concise summary", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "unexpected controllers: SCSI:0",
							})
					})
				})

				When("controller type mismatch", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{BusNumber: 0},
							},
						}
						// Set up MoVM.Config.Hardware.Device to have an IDE controller instead of SCSI.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
						)
					})

					It("should mark the condition as false", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: SCSI:0\nunexpected controllers: IDE:0",
							})
					})
				})

				When("controller bus number mismatch", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{BusNumber: 0},
							},
						}
						// Set up MoVM.Config.Hardware.Device to have a SCSI controller with bus number 1.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 1),
						)
					})

					It("should mark the condition as false", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: SCSI:0\nunexpected controllers: SCSI:1",
							})
					})
				})

				When("NVME controller mismatch", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							NVMEControllers: []vmopv1.NVMEControllerSpec{
								{BusNumber: 0},
							},
						}
						// Set up MoVM.Config.Hardware.Device to have a SCSI controller instead of NVME.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
						)
					})

					It("should mark the condition as false", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: NVME:0\nunexpected controllers: SCSI:0",
							})
					})
				})
			})

			Context("PVC volume mismatches", func() {
				When("spec has PVC volume but it's not attached", func() {
					BeforeEach(func() {
						setupPVCVolume("pvc-volume-1", "test-pvc", vmopv1.VirtualControllerTypeSCSI, 0, 0)
						setupSCSIControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have a SCSI controller but no disk.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{}
					})

					It("should mark the condition as false with concise summary", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareVolumesVerified: "missing PVC volumes: pvc-volume-1 (SCSI:0:0)",
							})
					})
				})

				When("PVC volume has incomplete placement information", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "pvc-volume-incomplete",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "test-pvc",
										},
										// Missing ControllerType, ControllerBusNumber, or UnitNumber.
									},
								},
							},
						}
					})

					It("should mark the condition as false with incomplete placement error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareVolumesVerified: "PVC volumes with incomplete placement: pvc-volume-incomplete",
							})
					})
				})

				When("PVC volume is correctly attached", func() {
					BeforeEach(func() {
						setupPVCVolume("pvc-volume-1", "test-pvc", vmopv1.VirtualControllerTypeSCSI, 0, 0)
						setupSCSIControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have a SCSI controller with a disk.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
							builder.DummyVirtualDisk(2000, 1000, ptr.To(int32(0)), "disk-uuid-123", ""),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     "pvc-volume-1",
								DiskUUID: "disk-uuid-123",
								Type:     vmopv1.VolumeTypeManaged,
							},
						}
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("unexpected PVC volume is attached", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{}
						setupSCSIControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have a SCSI controller with a disk.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
							builder.DummyVirtualDisk(2000, 1000, ptr.To(int32(0)), "disk-uuid-123", ""),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     "pvc-volume-1",
								DiskUUID: "disk-uuid-123",
								Type:     vmopv1.VolumeTypeManaged,
							},
						}
					})

					It("should mark the condition as false with unexpected PVC volumes error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareVolumesVerified: "unexpected PVC volumes: pvc-volume-1 (SCSI:0:0)",
							})
					})
				})
			})

			Context("CD-ROM device mismatches", func() {
				const (
					testVMIName     = "test-vmi"
					testVMIFileName = "/vmfs/volumes/datastore1/test.iso"
				)

				BeforeEach(func() {
					setupCDROMImage(testVMIName, testVMIFileName)

					vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						Cdrom: []vmopv1.VirtualMachineCdromSpec{
							builder.DummyCdromSpec(
								"cdrom1",
								testVMIName,
								"VirtualMachineImage",
								vmopv1.VirtualControllerTypeIDE,
								ptr.To(int32(0)),
								ptr.To(int32(0)),
								nil,
								nil,
							),
						},
					}
				})

				When("spec has CD-ROM but it's not attached", func() {
					BeforeEach(func() {
						setupIDEControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have an IDE controller but no CD-ROM.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
						)
					})

					It("should mark the condition as false with missing CD-ROM devices error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareCDROMVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareCDROMVerified: fmt.Sprintf("missing CD-ROM devices: %s (IDE:0:0)", testVMIFileName),
							})
					})
				})

				When("CD-ROM device has incomplete placement information", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware.Cdrom[0].ControllerType = ""
						vmCtx.VM.Spec.Hardware.Cdrom[0].ControllerBusNumber = nil
						vmCtx.VM.Spec.Hardware.Cdrom[0].UnitNumber = nil
					})

					It("should mark the condition as false with incomplete placement error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareCDROMVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareCDROMVerified: "CD-ROM devices with incomplete placement: cdrom1",
							})
					})
				})

				When("CD-ROM image reference cannot be resolved", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware.Cdrom[0].Image.Name = "non-existent-image"
					})

					It("should mark the condition as false with resolution error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareCDROMVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareCDROMVerified: "CD-ROM devices with failed resolution: cdrom1",
							})
					})
				})

				When("CD-ROM device is correctly attached", func() {
					BeforeEach(func() {
						setupIDEControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have an IDE controller with a CD-ROM.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
							builder.DummyCdromDevice(3000, 200, 0, testVMIFileName),
						)
					})

					It("should mark the condition as true", func() {
						assertConditionTrue()
					})
				})

				When("unexpected CD-ROM device is attached", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{}
						setupIDEControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have an IDE controller with a CD-ROM.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
							builder.DummyCdromDevice(3000, 200, 0, testVMIFileName),
						)
					})

					It("should mark the condition as false with unexpected CD-ROM devices error", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareCDROMVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareCDROMVerified: fmt.Sprintf("unexpected CD-ROM devices: %s (IDE:0:0)", testVMIFileName),
							})
					})
				})
			})

			Context("multiple items in same category", func() {
				When("multiple missing controllers", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{BusNumber: 0},
								{BusNumber: 1},
							},
							IDEControllers: []vmopv1.IDEControllerSpec{
								{BusNumber: 0},
							},
						}
					})

					It("should format multiple controllers as comma-separated list", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: IDE:0, SCSI:0, SCSI:1",
							})
					})
				})

				When("multiple unexpected controllers", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
							builder.DummyIDEController(200, 0, nil),
							builder.DummyNVMEController(20000, 0),
						)
					})

					It("should format multiple controllers as comma-separated list", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareControllersVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "unexpected controllers: IDE:0, NVME:0, SCSI:0",
							})
					})
				})

				When("multiple missing PVC volumes", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							builder.DummyPVCVirtualMachineVolume(
								"pvc-volume-1",
								"test-pvc-1",
								vmopv1.VirtualControllerTypeSCSI,
								ptr.To(int32(0)),
								ptr.To(int32(0)),
							),
							builder.DummyPVCVirtualMachineVolume(
								"pvc-volume-2",
								"test-pvc-2",
								vmopv1.VirtualControllerTypeSCSI,
								ptr.To(int32(0)),
								ptr.To(int32(1)),
							),
						}
						setupSCSIControllerInSpec(0)

						// Set up MoVM.Config.Hardware.Device to have a SCSI controller but no disks.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummySCSIController(1000, 0),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{}
					})

					It("should format multiple PVC volumes as comma-separated list", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareVolumesVerified: "missing PVC volumes: pvc-volume-1 (SCSI:0:0), pvc-volume-2 (SCSI:0:1)",
							})
					})
				})

				When("multiple incomplete placement PVC volumes", func() {
					BeforeEach(func() {
						vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "pvc-volume-incomplete-1",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "test-pvc-1",
										},
									},
								},
							},
							{
								Name: "pvc-volume-incomplete-2",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "test-pvc-2",
										},
									},
								},
							},
						}
					})

					It("should format multiple incomplete placement errors as comma-separated list", func() {
						assertConditionFalse(
							buildAggregateMessage(vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareVolumesVerified: "PVC volumes with incomplete placement: pvc-volume-incomplete-1, pvc-volume-incomplete-2",
							})
					})
				})
			})

			Context("multiple error scenarios", func() {
				When("controllers and volumes have issues", func() {
					BeforeEach(func() {
						// Set up mismatches in controllers and PVC volumes.
						setupSCSIControllerInSpec(0)
						setupPVCVolume("pvc-volume-1", "test-pvc", vmopv1.VirtualControllerTypeSCSI, 0, 0)

						// Set up MoVM.Config.Hardware.Device to have an IDE controller but no disk.
						// This simulates the mismatch: spec wants SCSI but status has IDE.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{}
					})

					It("should provide a concise summary of device types with issues", func() {
						assertConditionFalse(
							buildAggregateMessage(
								vmopv1.VirtualMachineHardwareControllersVerified,
								vmopv1.VirtualMachineHardwareVolumesVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: SCSI:0\nunexpected controllers: IDE:0",
								vmopv1.VirtualMachineHardwareVolumesVerified:     "missing PVC volumes: pvc-volume-1 (SCSI:0:0)",
							})
					})
				})

				When("controllers, volumes, and CD-ROM all have issues", func() {
					const (
						testVMIName     = "test-vmi"
						testVMIFileName = "/vmfs/volumes/datastore1/test.iso"
					)

					BeforeEach(func() {
						setupCDROMImage(testVMIName, testVMIFileName)

						// Set up mismatches in controllers, PVC volumes, and CD-ROM devices.
						setupSCSIControllerInSpec(0)
						setupPVCVolume("pvc-volume-1", "test-pvc", vmopv1.VirtualControllerTypeSCSI, 0, 0)

						vmCtx.VM.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
							builder.DummyCdromSpec(
								"cdrom1",
								testVMIName,
								"VirtualMachineImage",
								vmopv1.VirtualControllerTypeIDE,
								ptr.To(int32(0)),
								ptr.To(int32(0)),
								nil,
								nil,
							),
						}

						// Set up MoVM.Config.Hardware.Device to have an IDE controller but no disk or CD-ROM.
						// This simulates mismatches: spec wants SCSI controller, PVC volume, and CD-ROM but status has IDE controller only.
						vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
							builder.DummyIDEController(200, 0, nil),
						)
						vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{}
					})

					It("should provide a concise summary of all device types with issues", func() {
						assertConditionFalse(
							buildAggregateMessage(
								vmopv1.VirtualMachineHardwareControllersVerified,
								vmopv1.VirtualMachineHardwareVolumesVerified,
								vmopv1.VirtualMachineHardwareCDROMVerified),
							map[string]string{
								vmopv1.VirtualMachineHardwareControllersVerified: "missing controllers: SCSI:0\nunexpected controllers: IDE:0",
								vmopv1.VirtualMachineHardwareVolumesVerified:     "missing PVC volumes: pvc-volume-1 (SCSI:0:0)",
								vmopv1.VirtualMachineHardwareCDROMVerified:       fmt.Sprintf("missing CD-ROM devices: %s (IDE:0:0)", testVMIFileName),
							})
					})
				})
			})

			Context("excluded volumes", func() {
				BeforeEach(func() {
					// Set up unmanaged PVC volume in spec (excluded from checks).
					vmCtx.VM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						builder.DummyUnmanagedPVCVirtualMachineVolume(
							"unmanaged-pvc-volume",
							"test-pvc",
							"unmanaged-volume",
							vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
							vmopv1.VirtualControllerTypeSCSI,
							ptr.To(int32(0)),
							ptr.To(int32(0)),
						),
					}

					setupSCSIControllerInSpec(0)

					// Set up MoVM.Config.Hardware.Device to have a SCSI controller with a disk.
					vmCtx.MoVM.Config = builder.DummyVirtualMachineConfigInfo(
						builder.DummySCSIController(1000, 0),
						builder.DummyVirtualDisk(2000, 1000, ptr.To(int32(0)), "classic-disk-uuid", ""),
					)

					// Classic volume in status should be excluded from PVC volume checks.
					vmCtx.VM.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     "classic-volume",
							DiskUUID: "classic-disk-uuid",
							Type:     vmopv1.VolumeTypeClassic,
						},
					}
				})

				It("should exclude unmanaged PVC and classic volumes from checks and mark condition as true", func() {
					assertConditionTrue()
				})
			})
		})
	})

	Context("ReadinessProbe", func() {

		var (
			chanRecord chan string
		)

		BeforeEach(func() {
			chanRecord = make(chan string, 10)

			vmCtx.Context = record.WithContext(
				vmCtx.Context,
				record.New(&apirecord.FakeRecorder{Events: chanRecord}))

			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.AsyncSignalEnabled = true
			})
		})

		assertEvent := func(msg string) {
			var e string
			EventuallyWithOffset(1, chanRecord).Should(Receive(&e, Equal(msg)))
		}

		When("there is no probe", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.ReadinessProbe = nil
			})
			It("should not update status", func() {
				Expect(conditions.Has(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeFalse())
			})
		})

		When("there is a TCP probe", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
					TCPSocket: &vmopv1.TCPSocketAction{},
				}
			})
			It("should not update status", func() {
				Expect(conditions.Has(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeFalse())
			})
		})

		When("there is a GuestHeartbeat probe", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
					GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
				}
			})
			When("threshold is green", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus = vmopv1.GreenHeartbeatStatus
				})
				When("vm is green", func() {
					BeforeEach(func() {
						vmCtx.MoVM.GuestHeartbeatStatus = vimtypes.ManagedEntityStatusGreen
					})
					It("should mark ready=true", func() {
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeTrue())
						assertEvent("Normal Ready ")
					})
				})
				When("vm is red", func() {
					BeforeEach(func() {
						vmCtx.MoVM.GuestHeartbeatStatus = vimtypes.ManagedEntityStatusRed
					})
					It("should mark ready=false", func() {
						c := conditions.Get(vmCtx.VM, vmopv1.ReadyConditionType)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("NotReady"))
						Expect(c.Message).To(Equal("heartbeat status \"red\" is below threshold"))
						assertEvent("Normal NotReady heartbeat status \"red\" is below threshold")
					})
				})
				When("vm is unknown color", func() {
					BeforeEach(func() {
						vmCtx.MoVM.GuestHeartbeatStatus = "unknown"
					})
					It("should mark ready=Unknown", func() {
						c := conditions.Get(vmCtx.VM, vmopv1.ReadyConditionType)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionUnknown))
						assertEvent("Normal Unknown ")
					})
				})
			})
		})

		When("there is a GuestInfo probe", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{}
			})
			When("specific match", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						GuestInfo: []vmopv1.GuestInfoAction{
							{
								Key:   "hello",
								Value: "^world$",
							},
						},
					}
				})
				When("match exists", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.hello",
								Value: "world",
							},
						}
					})
					It("should mark ready=true", func() {
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeTrue())
						assertEvent("Normal Ready ")
					})
				})

				When("match does not exist", func() {
					It("should mark ready=false", func() {
						c := conditions.Get(vmCtx.VM, vmopv1.ReadyConditionType)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("NotReady"))
						assertEvent("Normal NotReady ")
					})
				})
			})
			When("wildcard match", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						GuestInfo: []vmopv1.GuestInfoAction{
							{
								Key: "hello",
							},
						},
					}
				})
				When("match exists", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.hello",
								Value: "there",
							},
						}
					})
					It("should mark ready=true", func() {
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeTrue())
						assertEvent("Normal Ready ")
					})
				})

				When("match does not exist", func() {
					It("should mark ready=false", func() {
						c := conditions.Get(vmCtx.VM, vmopv1.ReadyConditionType)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("NotReady"))
						assertEvent("Normal NotReady ")
					})
				})
			})

			When("multiple actions", func() {
				BeforeEach(func() {
					vmCtx.VM.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
						GuestInfo: []vmopv1.GuestInfoAction{
							{
								Key:   "hello",
								Value: "world|there",
							},
							{
								Key:   "fu",
								Value: "bar",
							},
						},
					}
				})
				When("match exists", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.hello",
								Value: "world",
							},
							&vimtypes.OptionValue{
								Key:   "guestinfo.fu",
								Value: "high bar",
							},
						}
					})
					It("should mark ready=true", func() {
						Expect(conditions.IsTrue(vmCtx.VM, vmopv1.ReadyConditionType)).To(BeTrue())
						assertEvent("Normal Ready ")
					})
				})

				When("match does not exist", func() {
					BeforeEach(func() {
						vmCtx.MoVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "guestinfo.hello",
								Value: "there",
							},
						}
					})
					It("should mark ready=false", func() {
						c := conditions.Get(vmCtx.VM, vmopv1.ReadyConditionType)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("NotReady"))
						assertEvent("Normal NotReady ")
					})
				})
			})
		})
	})

	Context("Copies values to the VM status", func() {
		biosUUID, instanceUUID := "f7c371d6-2003-5a48-9859-3bc9a8b0890", "6132d223-1566-5921-bc3b-df91ece09a4d"
		BeforeEach(func() {
			vmCtx.MoVM.Summary = vimtypes.VirtualMachineSummary{
				Config: vimtypes.VirtualMachineConfigSummary{
					Uuid:         biosUUID,
					InstanceUuid: instanceUUID,
					HwVersion:    "vmx-19",
				},
			}
		})

		It("sets the summary config values in the status", func() {
			status := vmCtx.VM.Status
			Expect(status).NotTo(BeNil())
			Expect(status.BiosUUID).To(Equal(biosUUID))
			Expect(status.InstanceUUID).To(Equal(instanceUUID))
			Expect(status.HardwareVersion).To(Equal(int32(19)))
		})
	})

	Context("Misc Status fields", func() {
		It("Back fills created Condition", func() {
			Expect(conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
		})

		Context("When FSS_WCP_VMSERVICE_RESIZE is not enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMResize = false
				})
			})

			Context("Has Class", func() {
				It("Back fills Status.Class", func() {
					Expect(vmCtx.VM.Status.Class).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Class.Name).To(Equal(builder.DummyClassName))
				})
			})
		})

		Context("When FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY is not enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMResizeCPUMemory = false
				})
			})

			Context("Has Class", func() {
				It("Back fills Status.Class", func() {
					Expect(vmCtx.VM.Status.Class).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Class.Name).To(Equal(builder.DummyClassName))
				})
			})
		})

		Context("Does not have Class", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.ClassName = ""
				vmCtx.VM.Status.Class = &vmopv1common.LocalObjectRef{Name: "foo"}
			})

			It("VM Status.Class is cleared", func() {
				Expect(vmCtx.VM.Status.Class).To(BeNil())
			})
		})
	})
})

var _ = Describe("VirtualMachineTools Status to VM Status Condition", func() {
	Context("markVMToolsRunningStatusCondition", func() {
		var (
			vm        *vmopv1.VirtualMachine
			guestInfo *vimtypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &vimtypes.GuestInfo{
				ToolsRunningStatus: "",
			}
		})

		JustBeforeEach(func() {
			vmlifecycle.MarkVMToolsRunningStatusCondition(vm, guestInfo)
		})

		Context("guestInfo is nil", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := []metav1.Condition{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "NoGuestInfo", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("ToolsRunningStatus is empty", func() {
			It("sets condition unknown", func() {
				expectedConditions := []metav1.Condition{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "NoGuestInfo", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is not running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning)
			})
			It("sets condition to false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason, "VMware Tools is not running"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsRunning)
			})
			It("sets condition true", func() {
				expectedConditions := []metav1.Condition{
					*conditions.TrueCondition(vmopv1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is starting", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts)
			})
			It("sets condition true", func() {
				expectedConditions := []metav1.Condition{
					*conditions.TrueCondition(vmopv1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("Unexpected vmtools running status", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = "blah"
			})
			It("sets condition unknown", func() {
				expectedConditions := []metav1.Condition{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "Unknown", "Unexpected VMware Tools running status"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})

var _ = Describe("VSphere Customization Status to VM Status Condition", func() {
	Context("markCustomizationInfoCondition", func() {
		var (
			vm        *vmopv1.VirtualMachine
			guestInfo *vimtypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &vimtypes.GuestInfo{
				CustomizationInfo: &vimtypes.GuestInfoCustomizationInfo{},
			}
		})

		JustBeforeEach(func() {
			vmlifecycle.MarkCustomizationInfoCondition(vm, guestInfo)
		})

		Context("guestInfo unset", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := []metav1.Condition{
					*conditions.UnknownCondition(vmopv1.GuestCustomizationCondition, "NoGuestInfo", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo unset", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := []metav1.Condition{
					*conditions.UnknownCondition(vmopv1.GuestCustomizationCondition, "NoGuestInfo", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo idle", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE)
			})
			It("sets condition true", func() {
				expectedConditions := []metav1.Condition{
					*conditions.TrueCondition(vmopv1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo pending", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
			})
			It("sets condition false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo running", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
			})
			It("sets condition false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo succeeded", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED)
			})
			It("sets condition true", func() {
				expectedConditions := []metav1.Condition{
					*conditions.TrueCondition(vmopv1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo failed", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED)
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, "%s", guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo invalid", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = "asdf"
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, "Unknown", "%s", guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})

var _ = Describe("VSphere Bootstrap Status to VM Status Condition", func() {
	Context("MarkBootstrapCondition", func() {
		var (
			vm          *vmopv1.VirtualMachine
			extraConfig map[string]string
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
		})

		JustBeforeEach(func() {
			vmlifecycle.MarkBootstrapCondition(vm, extraConfig)
		})

		AfterEach(func() {
			extraConfig = nil
		})

		Context("no bootstrap condition in extra config", func() {
			When("extraConfig unset", func() {
				BeforeEach(func() {
					extraConfig = nil
					conditions.MarkTrue(vm, vmopv1.GuestBootstrapCondition)
				})
				It("removes condition", func() {
					Expect(conditions.Get(vm, vmopv1.GuestBootstrapCondition)).To(BeNil())
				})
			})
			When("no bootstrap status", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1": "val1",
					}
					conditions.MarkTrue(vm, vmopv1.GuestBootstrapCondition)
				})
				It("removes condition", func() {
					Expect(conditions.Get(vm, vmopv1.GuestBootstrapCondition)).To(BeNil())
				})
			})
		})
		Context("successful condition", func() {
			When("status is 1", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "1",
					}
				})
				It("sets condition true", func() {
					expectedConditions := []metav1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true and there is a reason", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "true,my-reason",
					}
				})
				It("sets condition true", func() {
					expectedConditions := []metav1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					expectedConditions[0].Reason = "my-reason"
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true and there is a reason and message", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "true,my-reason,my,comma,delimited,message",
					}
				})
				It("sets condition true", func() {
					expectedConditions := []metav1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					expectedConditions[0].Reason = "my-reason"
					expectedConditions[0].Message = "my,comma,delimited,message"
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
		Context("failed condition", func() {
			When("status is 0", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "0",
					}
				})
				It("sets condition false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition, "", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is non-truthy", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "not a boolean value",
					}
				})
				It("sets condition false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition, "", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is false and there is a reason", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "false,my-reason",
					}
				})
				It("sets condition false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition, "my-reason", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is false and there is a reason and message", func() {
				BeforeEach(func() {
					extraConfig = map[string]string{
						"key1":                              "val1",
						pkgutil.GuestInfoBootstrapCondition: "false,my-reason,my,comma,delimited,message",
					}
				})
				It("sets condition false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"my-reason",
							"my,comma,delimited,message"),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
	})
})

var _ = Describe("VirtualMachineReconcileReady Status to VM Status Condition", func() {
	Context("MarkReconciliationCondition", func() {
		var (
			vm *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			}
		})

		JustBeforeEach(func() {
			vmlifecycle.MarkReconciliationCondition(vm)
		})

		Context("PausedVMLabel is non-existent", func() {
			It("sets VirtualMachineReconcileReady condition to true", func() {
				expectedConditions := []metav1.Condition{
					*conditions.TrueCondition(vmopv1.VirtualMachineReconcileReady),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))

			})
		})
		Context("PausedVMLabel exists", func() {
			When("PausedVMLabel value is devops", func() {
				BeforeEach(func() {
					vm.Labels[vmopv1.PausedVMLabelKey] = "devops"
				})
				It("sets VirtualMachineReconcileReady condition to false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason, "Virtual Machine reconciliation paused by DevOps"),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("PausedVMLabel value is admin", func() {
				BeforeEach(func() {
					vm.Labels[vmopv1.PausedVMLabelKey] = "admin"
				})
				It("sets VirtualMachineReconcileReady condition to false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason, "Virtual Machine reconciliation paused by Admin"),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("PausedVMLabel value is both", func() {
				BeforeEach(func() {
					vm.Labels[vmopv1.PausedVMLabelKey] = "both"
				})
				It("sets VirtualMachineReconcileReady condition to false", func() {
					expectedConditions := []metav1.Condition{
						*conditions.FalseCondition(
							vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason, "Virtual Machine reconciliation paused by Admin, DevOps"),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
	})
})

var _ = Describe("UpdateNetworkStatusConfig", func() {
	var (
		vm   *vmopv1.VirtualMachine
		args vmlifecycle.BootstrapArgs
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
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
				},
			},
		}

		args = vmlifecycle.BootstrapArgs{
			HostName:       "my-vm",
			DomainName:     "local.domain",
			DNSServers:     []string{"1.2.3.4", "5.6.7.8"},
			SearchSuffixes: []string{"fu.bar", "hello.world"},
			NetworkResults: network.NetworkInterfaceResults{
				Results: []network.NetworkInterfaceResult{
					{
						Name: "eth0",
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "192.168.0.2/24",
								IsIPv4:  true,
								Gateway: "192.168.0.1",
							},
							{
								IPCIDR:  "FD00:F53B:82E4::53/24",
								IsIPv4:  false,
								Gateway: "FD00:F500::::",
							},
						},
					},
				},
			},
		}
	})

	assertExpectedNetworkInterfaces := func(
		c *vmopv1.VirtualMachineNetworkConfigStatus,
		expectedNumber int) {

		ExpectWithOffset(1, c).ToNot(BeNil())
		ExpectWithOffset(1, c.Interfaces).To(HaveLen(expectedNumber))
		ic := c.Interfaces[0]
		ExpectWithOffset(1, ic.Name).To(Equal("eth0"))
		ExpectWithOffset(1, ic.IP).ToNot(BeNil())
		ExpectWithOffset(1, ic.IP.Addresses).To(HaveLen(2))
		ExpectWithOffset(1, ic.IP.Addresses[0]).To(Equal("192.168.0.2/24"))
		ExpectWithOffset(1, ic.IP.Addresses[1]).To(Equal("FD00:F53B:82E4::53/24"))
		ExpectWithOffset(1, ic.IP.DHCP).To(BeNil())
		ExpectWithOffset(1, ic.IP.Gateway4).To(Equal("192.168.0.1"))
		ExpectWithOffset(1, ic.IP.Gateway6).To(Equal("FD00:F500::::"))
	}

	assertExpectedDNS := func(c *vmopv1.VirtualMachineNetworkConfigStatus) {
		ExpectWithOffset(1, c.DNS).ToNot(BeNil())
		ExpectWithOffset(1, c.DNS.HostName).To(Equal("my-vm"))
		ExpectWithOffset(1, c.DNS.DomainName).To(Equal("local.domain"))
		ExpectWithOffset(1, c.DNS.Nameservers).To(Equal([]string{"1.2.3.4", "5.6.7.8"}))
		ExpectWithOffset(1, c.DNS.SearchDomains).To(Equal([]string{"fu.bar", "hello.world"}))
	}

	AfterEach(func() {
		args = vmlifecycle.BootstrapArgs{}
	})

	When("vm is nil", func() {
		It("should panic", func() {
			fn := func() {
				vmlifecycle.UpdateNetworkStatusConfig(nil, args)
			}
			Expect(fn).Should(PanicWith("vm is nil"))
		})
	})

	When("vm is not nil", func() {
		JustBeforeEach(func() {
			vmlifecycle.UpdateNetworkStatusConfig(vm, args)
		})

		When("bootstrap args are empty", func() {
			BeforeEach(func() {
				args = vmlifecycle.BootstrapArgs{}
			})
			Specify("status.network should be nil", func() {
				Expect(vm.Status.Network).To(BeNil())
			})
		})

		When("bootstrap args are not empty", func() {
			var (
				config *vmopv1.VirtualMachineNetworkConfigStatus
			)
			BeforeEach(func() {
				vm.Spec.Network = nil
			})
			JustBeforeEach(func() {
				slices.Sort(args.DNSServers)
				slices.Sort(args.SearchSuffixes)
				ExpectWithOffset(1, vm.Status.Network).ToNot(BeNil())
				ExpectWithOffset(1, vm.Status.Network.Config).ToNot(BeNil())
				config = vm.Status.Network.Config
			})

			When("there are no network interface results", func() {
				BeforeEach(func() {
					args.NetworkResults.Results = nil
				})
				Specify("status.network.config.interfaces should be nil", func() {
					Expect(config.Interfaces).To(BeNil())
				})
				Specify("status.network.config.dns should not be nil", func() {
					assertExpectedDNS(config)
				})
			})

			When("there is a single network interface result", func() {
				Specify("status.network.config.dns should not be nil", func() {
					assertExpectedDNS(config)
				})
				Specify("status.network.config.interfaces should have one interface", func() {
					assertExpectedNetworkInterfaces(config, 1)
					Expect(config.Interfaces[0].DNS).To(BeNil())
				})

				When("there is no DNS information", func() {
					BeforeEach(func() {
						args.HostName = ""
						args.DomainName = ""
					})
					When("the DNS servers and search suffixes are nil", func() {
						BeforeEach(func() {
							args.DNSServers = nil
							args.SearchSuffixes = nil
						})
						Specify("status.network.config.dns should be nil", func() {
							Expect(config.DNS).To(BeNil())
						})
						Specify("status.network.config.interfaces should have one interface", func() {
							assertExpectedNetworkInterfaces(config, 1)
							Expect(config.Interfaces[0].DNS).To(BeNil())
						})
					})
					When("the DNS servers and search suffixes are empty", func() {
						BeforeEach(func() {
							args.DNSServers = []string{}
							args.SearchSuffixes = []string{}
						})
						Specify("status.network.config.dns should be nil", func() {
							Expect(config.DNS).To(BeNil())
						})
						Specify("status.network.config.interfaces should have one interface", func() {
							assertExpectedNetworkInterfaces(config, 1)
							Expect(config.Interfaces[0].DNS).To(BeNil())
						})
					})
				})

				When("there is DNS information in the interface", func() {
					BeforeEach(func() {
						args.NetworkResults.Results[0].Nameservers = []string{"1.1.1.1"}
						args.NetworkResults.Results[0].SearchDomains = []string{"per.vm"}
					})
					Specify("status.network.config.interfaces should have one interface", func() {
						assertExpectedNetworkInterfaces(config, 1)
						ic := config.Interfaces[0]
						Expect(ic.DNS).ToNot(BeNil())
						Expect(ic.DNS.HostName).To(BeEmpty())
						Expect(ic.DNS.DomainName).To(BeEmpty())
						Expect(ic.DNS.Nameservers).To(Equal([]string{"1.1.1.1"}))
						Expect(ic.DNS.SearchDomains).To(Equal([]string{"per.vm"}))
					})
				})
			})

			When("there are two network interface results", func() {
				BeforeEach(func() {
					args.NetworkResults.Results = append(
						args.NetworkResults.Results,
						network.NetworkInterfaceResult{
							Name: "eth1",
							IPConfigs: []network.NetworkInterfaceIPConfig{
								{
									IPCIDR:  "192.168.0.3/24",
									IsIPv4:  true,
									Gateway: "192.168.0.1",
								},
								{
									IPCIDR:  "FD00:F53B:82E4::54/24",
									IsIPv4:  false,
									Gateway: "FD00:F500::::",
								},
							},
						})
				})

				Specify("status.network.config.dns should not be nil", func() {
					assertExpectedDNS(config)
				})
				Specify("status.network.config.interfaces should have two interfaces", func() {
					assertExpectedNetworkInterfaces(config, 2)
					Expect(config.Interfaces[0].DNS).To(BeNil())
					ic := config.Interfaces[1]
					ExpectWithOffset(1, ic.Name).To(Equal("eth1"))
					ExpectWithOffset(1, ic.IP).ToNot(BeNil())
					ExpectWithOffset(1, ic.IP.Addresses).To(HaveLen(2))
					ExpectWithOffset(1, ic.IP.Addresses[0]).To(Equal("192.168.0.3/24"))
					ExpectWithOffset(1, ic.IP.Addresses[1]).To(Equal("FD00:F53B:82E4::54/24"))
					ExpectWithOffset(1, ic.IP.DHCP).To(BeNil())
					ExpectWithOffset(1, ic.IP.Gateway4).To(Equal("192.168.0.1"))
					ExpectWithOffset(1, ic.IP.Gateway6).To(Equal("FD00:F500::::"))
				})
			})
		})
	})
})

var _ = Describe("Group status", func() {
	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		data  vmlifecycle.ReconcileStatusData
		vcVM  *object.VirtualMachine
		vm    *vmopv1.VirtualMachine
		vmg   *vmopv1.VirtualMachineGroup
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMGroups = true
		})

		vm = builder.DummyVirtualMachine()
		vm.Name = "group-status-test"
		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		// The following vars are just required to call ReconcileStatus().
		// They're not actually used in these group status tests.
		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		Expect(vcVM.Properties(
			ctx,
			vcVM.Reference(),
			vsphere.VMUpdatePropertiesSelector,
			&vmCtx.MoVM)).To(Succeed())

		data = vmlifecycle.ReconcileStatusData{
			NetworkDeviceKeysToSpecIdx: map[int32]int{},
		}
	})

	JustBeforeEach(func() {
		Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("VM has no group name specified", func() {
		BeforeEach(func() {
			vm.Spec.GroupName = ""
			conditions.MarkTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
		})

		It("should delete the GroupLinked condition", func() {
			Expect(conditions.Get(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeNil())
		})
	})

	When("VM group name is set to a non-existent group", func() {
		BeforeEach(func() {
			vm.Spec.GroupName = "non-existent-group"
		})

		It("should set GroupLinked condition to False with NotFound reason", func() {
			c := conditions.Get(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal("NotFound"))
		})
	})

	When("VM group name is set to an existing group", func() {
		BeforeEach(func() {
			vmg = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-group",
					Namespace: vm.Namespace,
				},
				Spec: vmopv1.VirtualMachineGroupSpec{
					BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
						{
							Members: []vmopv1.GroupMember{},
						},
					},
				},
			}
			Expect(ctx.Client.Create(ctx, vmg)).To(Succeed())

			vm.Spec.GroupName = vmg.Name
		})

		When("VM is not a member of the group", func() {
			It("should set GroupLinked condition to False with NotMember reason", func() {
				c := conditions.Get(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				Expect(c).ToNot(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
				Expect(c.Reason).To(Equal("NotMember"))
			})
		})

		When("VM is a member of the group", func() {
			BeforeEach(func() {
				vmg.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Name: vm.Name,
								Kind: "VirtualMachine",
							},
						},
					},
				}
				Expect(ctx.Client.Update(ctx, vmg)).To(Succeed())
			})

			It("should set GroupLinked condition to True", func() {
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
			})
		})
	})
})

var _ = Describe("Hardware status", func() {
	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		vcVM  *object.VirtualMachine
		data  vmlifecycle.ReconcileStatusData
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		vm := builder.DummyVirtualMachine()
		vm.Name = "hardware-status-test"

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		Expect(vcVM.Properties(
			ctx,
			vcVM.Reference(),
			vsphere.VMUpdatePropertiesSelector,
			&vmCtx.MoVM)).To(Succeed())

		data = vmlifecycle.ReconcileStatusData{
			NetworkDeviceKeysToSpecIdx: map[int32]int{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("reconcileStatusHardware", func() {
		When("config is nil", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = nil
			})

			It("should return nil without setting hardware status", func() {
				err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmCtx.VM.Status.Hardware).To(BeNil())
			})
		})

		When("config is valid", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						NumCPU:   4,
						MemoryMB: 2048,
						Device:   []vimtypes.BaseVirtualDevice{},
					},
					CpuAllocation: &vimtypes.ResourceAllocationInfo{
						Reservation: ptr.To(int64(1000)),
					},
					MemoryAllocation: &vimtypes.ResourceAllocationInfo{
						Reservation: ptr.To(int64(1024)),
					},
				}
			})

			It("should populate basic hardware status", func() {
				err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
				Expect(err).ToNot(HaveOccurred())

				Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Hardware.CPU.Total).To(Equal(int32(4)))
				Expect(vmCtx.VM.Status.Hardware.CPU.Reservation).To(Equal(int64(1000)))

				Expect(vmCtx.VM.Status.Hardware.Memory.Total).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Hardware.Memory.Total.String()).To(Equal("2048M"))

				Expect(vmCtx.VM.Status.Hardware.Memory.Reservation).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Hardware.Memory.Reservation.String()).To(Equal("1024M"))
			})

			When("CPU total is zero and allocation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.NumCPU = 0
					vmCtx.MoVM.Config.CpuAllocation = nil
				})

				It("should not set CPU status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.CPU).To(BeNil())
				})
			})

			When("CPU total is zero and reservation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.NumCPU = 0
					vmCtx.MoVM.Config.CpuAllocation = &vimtypes.ResourceAllocationInfo{}
				})

				It("should not set CPU status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.CPU).To(BeNil())
				})
			})

			When("CPU total is zero and reservation is zero", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.NumCPU = 0
					vmCtx.MoVM.Config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
						Reservation: ptr.To(int64(0)),
					}
				})

				It("should not set CPU status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.CPU).To(BeNil())
				})
			})

			When("CPU allocation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.CpuAllocation = nil
				})

				It("should not set CPU reservation", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.CPU.Total).To(Equal(int32(4)))
					Expect(vmCtx.VM.Status.Hardware.CPU.Reservation).To(Equal(int64(0)))
				})
			})

			When("CPU reservation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.CpuAllocation.Reservation = nil
				})

				It("should not set CPU reservation", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.CPU.Reservation).To(Equal(int64(0)))
				})
			})

			When("memory total is zero and allocation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.MemoryMB = 0
					vmCtx.MoVM.Config.MemoryAllocation = nil
				})

				It("should not set memory status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory).To(BeNil())
				})
			})

			When("memory total is zero and reservation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.MemoryMB = 0
					vmCtx.MoVM.Config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{}
				})

				It("should not set memory status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory).To(BeNil())
				})
			})

			When("memory total is zero and reservation is zero", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.MemoryMB = 0
					vmCtx.MoVM.Config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
						Reservation: ptr.To(int64(0)),
					}
				})

				It("should not set memory status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory).To(BeNil())
				})
			})

			When("memory allocation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.MemoryAllocation = nil
				})

				It("should not set memory reservation", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory.Reservation).To(BeNil())
				})
			})

			When("memory reservation is nil", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.MemoryAllocation.Reservation = nil
				})

				It("should not set memory reservation", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory.Reservation).To(BeNil())
				})
			})

			When("memory reservation is zero", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.MemoryAllocation.Reservation = ptr.To(int64(0))
				})

				It("should not set memory reservation", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory.Reservation).To(BeNil())
				})
			})

			When("memory is zero", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.MemoryMB = 0
				})

				It("should not set memory", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.Memory.Total).To(BeNil())
				})
			})

			When("memory is larger than math.MaxInt32 bytes", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.MemoryMB = math.MaxInt32/(1000*1000) + 1
				})

				It("should set the memory status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())
					Expect(vmCtx.VM.Status.Hardware.Memory.Total.String()).To(Equal("2148M"))
				})
			})
		})

		Context("nVidia vGPU devices", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						NumCPU:   2,
						MemoryMB: 1024,
					},
				}
			})

			When("PCI passthrough device has VmiopBacking with no migration support", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "grid_p40-2q",
								},
							},
						})
				})

				It("should add nVidia vGPU with None migration type", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(1))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Profile).To(Equal("grid_p40-2q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeNone))
				})

				It("should add single nVidia vGPU with None migration type when reconciled multiple times", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(1))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Profile).To(Equal("grid_p40-2q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeNone))
				})
			})

			When("PCI passthrough device has VmiopBacking with normal migration support", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu:             "grid_p40-4q",
									MigrateSupported: ptr.To(true),
								},
							},
						})
				})

				It("should add nVidia vGPU with Normal migration type", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(1))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Profile).To(Equal("grid_p40-4q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeNormal))
				})
			})

			When("PCI passthrough device has VmiopBacking with enhanced migration support", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu:                      "grid_p40-8q",
									MigrateSupported:          ptr.To(true),
									EnhancedMigrateCapability: ptr.To(true),
								},
							},
						})
				})

				It("should add nVidia vGPU with Enhanced migration type", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(1))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Profile).To(Equal("grid_p40-8q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeEnhanced))
				})
			})

			When("multiple nVidia vGPU devices are present", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "grid_p40-1q",
								},
							},
						},
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu:                      "grid_p40-2q",
									MigrateSupported:          ptr.To(true),
									EnhancedMigrateCapability: ptr.To(true),
								},
							},
						})
				})

				It("should add all nVidia vGPUs", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(2))

					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].Profile).To(Equal("grid_p40-1q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[0].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeNone))

					Expect(vmCtx.VM.Status.Hardware.VGPUs[1].Type).To(Equal(vmopv1.VirtualMachineVGPUTypeNVIDIA))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[1].Profile).To(Equal("grid_p40-2q"))
					Expect(vmCtx.VM.Status.Hardware.VGPUs[1].MigrationType).To(Equal(vmopv1.VirtualMachineVGPUMigrationTypeEnhanced))
				})
			})

			When("PCI passthrough device has different backing type", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{},
							},
						})
				})

				It("should not add any nVidia vGPUs", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Hardware).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Hardware.VGPUs).To(HaveLen(0))
				})
			})
		})

		Context("vTPM devices", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						NumCPU:   2,
						MemoryMB: 1024,
					},
				}
			})

			When("vTPM device is present", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualTPM{})
				})

				It("should set crypto status with vTPM", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Crypto).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Crypto.HasVTPM).To(BeTrue())
				})
			})

			When("multiple vTPM devices are present", func() {
				BeforeEach(func() {
					vmCtx.MoVM.Config.Hardware.Device = append(vmCtx.MoVM.Config.Hardware.Device,
						&vimtypes.VirtualTPM{},
						&vimtypes.VirtualTPM{})
				})

				It("should set crypto status with vTPM", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Crypto).ToNot(BeNil())
					Expect(vmCtx.VM.Status.Crypto.HasVTPM).To(BeTrue())
				})
			})

			When("no vTPM devices are present", func() {
				It("should not set crypto status", func() {
					err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmCtx.VM.Status.Crypto).To(BeNil())
				})
			})
		})
	})
})

var _ = Describe("Guest status", func() {
	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		vcVM  *object.VirtualMachine
		data  vmlifecycle.ReconcileStatusData
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		vm := builder.DummyVirtualMachine()
		vm.Name = "guest-status-test"

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		Expect(vcVM.Properties(
			ctx,
			vcVM.Reference(),
			vsphere.VMUpdatePropertiesSelector,
			&vmCtx.MoVM)).To(Succeed())

		data = vmlifecycle.ReconcileStatusData{
			NetworkDeviceKeysToSpecIdx: map[int32]int{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("reconcileStatusGuest", func() {
		When("config is nil", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = nil
			})

			It("should not set guest status", func() {
				err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmCtx.VM.Status.Guest).To(BeNil())
			})
		})

		When("config has guest information", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					GuestId:       "ubuntu64Guest",
					GuestFullName: "Ubuntu Linux (64-bit)",
					Hardware: vimtypes.VirtualHardware{
						NumCPU:   2,
						MemoryMB: 1024,
					},
				}
			})

			It("should populate guest status", func() {
				err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
				Expect(err).ToNot(HaveOccurred())

				Expect(vmCtx.VM.Status.Guest).ToNot(BeNil())
				Expect(vmCtx.VM.Status.Guest.GuestID).To(Equal("ubuntu64Guest"))
				Expect(vmCtx.VM.Status.Guest.GuestFullName).To(Equal("Ubuntu Linux (64-bit)"))
			})
		})

		When("config has empty guest information", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					GuestId:       "",
					GuestFullName: "",
					Hardware: vimtypes.VirtualHardware{
						NumCPU:   2,
						MemoryMB: 1024,
					},
				}
			})

			It("should set guest status to nil", func() {
				err := vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmCtx.VM.Status.Guest).To(BeNil())
			})
		})
	})
})

var _ = Describe("Snapshot status", func() {
	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		vcVM  *object.VirtualMachine
		data  vmlifecycle.ReconcileStatusData
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSnapshots = true
		})

		vm := builder.DummyVirtualMachine()
		vm.Name = "snapshot-status-test"

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		// Initialize with the expected properties. Tests can overwrite this if needed.
		Expect(vcVM.Properties(
			ctx,
			vcVM.Reference(),
			vsphere.VMUpdatePropertiesSelector,
			&vmCtx.MoVM)).To(Succeed())

		data = vmlifecycle.ReconcileStatusData{
			NetworkDeviceKeysToSpecIdx: map[int32]int{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("updateCurrentSnapshotStatus", func() {
		When("VM has no snapshots", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = nil
				vmCtx.VM.Status.CurrentSnapshot = snapshotRefManaged("old-snapshot")
			})

			It("should clear the current snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
			})
		})

		When("VM has snapshots but no current snapshot", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: nil,
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "snapshot-1",
						},
					},
				}
				vmCtx.VM.Status.CurrentSnapshot = snapshotRefManaged("old-snapshot")
			})

			It("should clear the current snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
			})
		})

		When("VM has a current snapshot but snapshot name cannot be found in tree", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-not-found",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "snapshot-1",
						},
					},
				}
				vmCtx.VM.Status.CurrentSnapshot = snapshotRefManaged("old-snapshot")
			})

			It("should clear the current snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
			})
		})

		When("VM has a current snapshot with a name that exists in the tree", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "test-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			Context("when snapshots capability is not enabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSnapshots = false
					})
				})

				It("should not update the current snapshot status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
				})
			})

			Context("when snapshots capability is enabled", func() {
				It("should update the current snapshot status to match the found snapshot", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.CurrentSnapshot).ToNot(BeNil())
					Expect(vmCtx.VM.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
					Expect(vmCtx.VM.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))
				})
			})

			When("the snapshot custom resource doesn't exist", func() {
				BeforeEach(func() {
					// clear the finalizers to allow for actual deletion of the resource
					vmSnapshot.Finalizers = []string{}
					Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				})

				It("should mark an Unmanaged snapshot as the current snapshot", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.CurrentSnapshot).ToNot(BeNil())
					Expect(vmCtx.VM.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged))
				})
			})
			When("the snapshot custom resource that is marked for deletion", func() {
				BeforeEach(func() {
					// the vmSnapshot object already has the vmoperator.vmware.com/virtualmachinesnapshot finalizer
					// it will only be marked for deletion but not actually deleted
					Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				})

				It("should not update the status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
				})
			})
		})

		When("VM has a nested snapshot structure", func() {
			var vmSnapshot1, vmSnapshot2 *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "child-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot1)).To(Succeed())
				vmSnapshot2 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "root-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot2)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "child-snapshot-ref",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "root-snapshot-ref",
							},
							Name: "root-snapshot",
							ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "child-snapshot-ref",
									},
									Name: "child-snapshot",
								},
							},
						},
					},
				}
			})

			It("should find and update the current snapshot status for the nested snapshot", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).ToNot(BeNil())
				Expect(vmCtx.VM.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
				Expect(vmCtx.VM.Status.CurrentSnapshot.Name).To(Equal("child-snapshot"))
			})
		})

		When("VM has a deeply nested snapshot structure", func() {
			var vmSnapshot1, vmSnapshot2, vmSnapshot3 *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "root-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot1)).To(Succeed())
				vmSnapshot2 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "level1-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot2)).To(Succeed())
				vmSnapshot3 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "deep-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot3)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "deep-snapshot-ref",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "root-snapshot-ref",
							},
							Name: "root-snapshot",
							ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "level1-snapshot-ref",
									},
									Name: "level1-snapshot",
									ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
										{
											Snapshot: vimtypes.ManagedObjectReference{
												Type:  "VirtualMachineSnapshot",
												Value: "deep-snapshot-ref",
											},
											Name: "deep-snapshot",
										},
									},
								},
							},
						},
					},
				}
			})

			It("should find and update the current snapshot status for the deeply nested snapshot", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).ToNot(BeNil())
				Expect(vmCtx.VM.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
				Expect(vmCtx.VM.Status.CurrentSnapshot.Name).To(Equal("deep-snapshot"))
			})
		})

		When("VM has multiple root snapshots", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-2", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-2-ref",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1-ref",
							},
							Name: "snapshot-1",
						},
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-2-ref",
							},
							Name: "snapshot-2",
						},
					},
				}
			})

			It("should find and update the current snapshot status from the second root snapshot", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).ToNot(BeNil())
				Expect(vmCtx.VM.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
				Expect(vmCtx.VM.Status.CurrentSnapshot.Name).To(Equal("snapshot-2"))
			})

			It("should find and update the correct root snapshots in the status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status).ToNot(BeNil())
				Expect(vmCtx.VM.Status.RootSnapshots).To(HaveLen(2))

				Expect(vmCtx.VM.Status.RootSnapshots[0]).ToNot(BeNil())
				Expect(vmCtx.VM.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged))
				// RootSnapshots[0] is unmanaged, so it doesn't have a corresponding snapshot name.

				Expect(vmCtx.VM.Status.RootSnapshots[1]).ToNot(BeNil())
				Expect(vmCtx.VM.Status.RootSnapshots[1].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
				Expect(vmCtx.VM.Status.RootSnapshots[1].Name).To(Equal("snapshot-2"))
			})
		})

		When("the current snapshot status is already correct", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "test-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.VM.Status.CurrentSnapshot = &vmopv1.VirtualMachineSnapshotReference{
					Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
					Name: vmSnapshot.Name,
				}

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			It("should not change the current snapshot status", func() {
				originalSnapshot := vmCtx.VM.Status.CurrentSnapshot
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(originalSnapshot))
			})
		})

		When("there's an error getting the VirtualMachineSnapshot custom resource", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			It("should return an error", func() {
				mockClient := &mockClient{
					Client:   ctx.Client,
					getError: fmt.Errorf("database connection failed"),
				}
				err := vmlifecycle.ReconcileStatus(vmCtx, mockClient, vcVM, data)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection failed"))
			})
		})
	})

	Context("updateSnapshotTreeChildrenStatus", func() {
		When("VM has no snapshots", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = nil
			})

			It("should return success and not updating any snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
			})
		})

		When("VM has snapshots but no root snapshot", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{},
				}
			})

			It("should return success and not updating any snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
			})
		})

		When("VM has a snapshot", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "test-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			Context("when snapshots capability is not enabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSnapshots = false
					})
				})

				It("should return success and not updating any snapshot status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				})
			})

			Context("when snapshots capability is enabled", func() {
				It("should succeed, but children status of the snapshot is not updated", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "test-snapshot",
					}, vmSnapshot)).To(Succeed())
					Expect(vmSnapshot.Status.Children).To(BeNil())
				})
			})

			When("the snapshot custom resource doesn't exist", func() {
				BeforeEach(func() {
					// Delete the finalizer
					vmSnapshot.Finalizers = []string{}
					Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				})

				It("should return success and not updating any snapshot status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				})
			})
		})

		When("VM has nested snapshots", func() {
			var vmSnapshot1, vmSnapshot2, vmSnapshot3, vmSnapshot4,
				vmSnapshot5, vmSnapshot6, vmSnapshot7 *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				//         1
				//      /      \
				//    2 3 4     6
				//       /     /
				//      5     7

				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-1", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot1)).To(Succeed())
				vmSnapshot2 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-2", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot2)).To(Succeed())
				vmSnapshot3 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-3", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot3)).To(Succeed())
				vmSnapshot4 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-4", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot4)).To(Succeed())
				vmSnapshot5 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-5", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot5)).To(Succeed())
				vmSnapshot6 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-6", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot6)).To(Succeed())
				vmSnapshot7 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-7", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot7)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-2-ref",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1-ref",
							},
							Name: "snapshot-1",
							ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "snapshot-2-ref",
									},
									Name: "snapshot-2",
								},
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "snapshot-3-ref",
									},
									Name: "snapshot-3",
								},
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "snapshot-4-ref",
									},
									Name: "snapshot-4",
									ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
										{
											Snapshot: vimtypes.ManagedObjectReference{
												Type:  "VirtualMachineSnapshot",
												Value: "snapshot-5-ref",
											},
											Name: "snapshot-5",
										},
									},
								},
							},
						},
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-6-ref",
							},
							Name: "snapshot-6",
							ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
								{
									Snapshot: vimtypes.ManagedObjectReference{
										Type:  "VirtualMachineSnapshot",
										Value: "snapshot-7-ref",
									},
									Name: "snapshot-7",
								},
							},
						},
					},
				}
			})

			It("should update all children status of snapshots that has children", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-1",
				}, vmSnapshot1)).To(Succeed())
				Expect(vmSnapshot1.Status.Children).To(HaveLen(3))
				Expect(vmSnapshot1.Status.Children).To(ContainElements(
					vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot2.Name,
					},
					vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot3.Name,
					},
					vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot4.Name,
					},
				))

				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-2",
				}, vmSnapshot2)).To(Succeed())
				Expect(vmSnapshot2.Status.Children).To(BeNil())
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-3",
				}, vmSnapshot3)).To(Succeed())
				Expect(vmSnapshot3.Status.Children).To(BeNil())
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-4",
				}, vmSnapshot4)).To(Succeed())
				Expect(vmSnapshot4.Status.Children).To(HaveLen(1))
				Expect(vmSnapshot4.Status.Children).To(ContainElements(vmopv1.VirtualMachineSnapshotReference{
					Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
					Name: vmSnapshot5.Name,
				}))
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-5",
				}, vmSnapshot5)).To(Succeed())
				Expect(vmSnapshot5.Status.Children).To(BeNil())
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-6",
				}, vmSnapshot6)).To(Succeed())
				Expect(vmSnapshot6.Status.Children).To(HaveLen(1))
				Expect(vmSnapshot6.Status.Children).To(ContainElements(vmopv1.VirtualMachineSnapshotReference{
					Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
					Name: vmSnapshot7.Name,
				}))
				Expect(ctx.Client.Get(ctx, types.NamespacedName{
					Namespace: vmCtx.VM.Namespace,
					Name:      "snapshot-7",
				}, vmSnapshot7)).To(Succeed())
				Expect(vmSnapshot7.Status.Children).To(BeNil())
			})

			When("one of the snapshot CR(snapshot-4) doesn't exist", func() {
				BeforeEach(func() {
					By("delete snapshot-4")
					// Delete the finalizer
					vmSnapshot4.Finalizers = []string{}
					Expect(ctx.Client.Update(ctx, vmSnapshot4)).To(Succeed())
					// Delete the internal snapshot
					Expect(ctx.Client.Delete(ctx, vmSnapshot4)).To(Succeed())
				})

				It("should succeed, but children status of the snapshot is updated, and the deleted one(snapshot-4) is not", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-1",
					}, vmSnapshot1)).To(Succeed())
					Expect(vmSnapshot1.Status.Children).To(HaveLen(3))
					Expect(vmSnapshot1.Status.Children).To(ContainElements(vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot2.Name,
					},
						vmopv1.VirtualMachineSnapshotReference{
							Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
							Name: vmSnapshot3.Name,
						},
						vmopv1.VirtualMachineSnapshotReference{
							Type: vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged,
							// snapshot-4 is deleted, so it should be marked as Unmanaged
							Name: "",
						},
					))

					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-2",
					}, vmSnapshot2)).To(Succeed())
					Expect(vmSnapshot2.Status.Children).To(BeNil())
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-3",
					}, vmSnapshot3)).To(Succeed())
					Expect(vmSnapshot3.Status.Children).To(BeNil())
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-5",
					}, vmSnapshot5)).To(Succeed())
					Expect(vmSnapshot5.Status.Children).To(BeNil())
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-6",
					}, vmSnapshot6)).To(Succeed())
					Expect(vmSnapshot6.Status.Children).To(HaveLen(1))
					Expect(vmSnapshot6.Status.Children).To(ContainElements(vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot7.Name,
					}))
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-7",
					}, vmSnapshot7)).To(Succeed())
					Expect(vmSnapshot7.Status.Children).To(BeNil())

					By("snapshot-4 should not be found")
					Expect(ctx.Client.Get(ctx, types.NamespacedName{
						Namespace: vmCtx.VM.Namespace,
						Name:      "snapshot-4",
					}, vmSnapshot4)).NotTo(Succeed())
				})
			})
		})

		When("there's an error getting the VirtualMachineSnapshot custom resource", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			It("should return an error", func() {
				mockClient := &mockClient{
					Client:   ctx.Client,
					getError: fmt.Errorf("database connection failed"),
				}
				err := vmlifecycle.ReconcileStatus(vmCtx, mockClient, vcVM, data)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection failed"))
			})
		})
	})

	Context("updateRootSnapshots", func() {
		When("VM has no snapshots", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = nil
				vmCtx.VM.Status.RootSnapshots = []vmopv1.VirtualMachineSnapshotReference{
					*snapshotRefManaged("old-snapshot"),
				}
			})

			It("should clear the root snapshots status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.RootSnapshots).To(BeNil())
			})
		})

		When("VM has snapshots but no root snapshot", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{},
				}
				vmCtx.VM.Status.RootSnapshots = []vmopv1.VirtualMachineSnapshotReference{
					*snapshotRefManaged("old-snapshot"),
				}
			})

			It("should clear the root snapshots status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.RootSnapshots).To(BeNil())
			})
		})

		When("VM has a root snapshot but snapshot name cannot be found in tree", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-not-found",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "snapshot-1",
						},
					},
				}
				vmCtx.VM.Status.CurrentSnapshot = snapshotRefManaged("old-snapshot")
			})

			It("should clear the current snapshot status", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.CurrentSnapshot).To(BeNil())
			})
		})

		When("VM has a root snapshot", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "test-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			Context("when snapshots capability is not enabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSnapshots = false
					})
				})

				It("should not update the root snapshots status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.RootSnapshots).To(BeNil())
				})
			})

			Context("when snapshots capability is enabled", func() {
				It("should update the root snapshots status to match the found snapshot", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.RootSnapshots).ToNot(BeNil())
					Expect(vmCtx.VM.Status.RootSnapshots).To(HaveLen(1))
					Expect(vmCtx.VM.Status.RootSnapshots[0]).To(Equal(
						vmopv1.VirtualMachineSnapshotReference{
							Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
							Name: vmSnapshot.Name,
						},
					))
				})
			})

			When("the snapshot custom resource doesn't exist", func() {
				BeforeEach(func() {
					// Delete the finalizer
					vmSnapshot.Finalizers = []string{}
					Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				})

				It("should mark an Unmanaged snapshot", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.RootSnapshots).To(HaveLen(1))
					Expect(vmCtx.VM.Status.RootSnapshots).To(ContainElement(vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged,
					}))
				})
			})

			When("the snapshot custom resource that is marked for deletion", func() {
				BeforeEach(func() {
					// the vmSnapshot object already has the vmoperator.vmware.com/virtualmachinesnapshot finalizer
					// it will only be marked for deletion but not actually deleted
					Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
				})

				It("should update the status", func() {
					Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
					Expect(vmCtx.VM.Status.RootSnapshots).ToNot(BeNil())
					Expect(vmCtx.VM.Status.RootSnapshots).To(HaveLen(1))
					Expect(vmCtx.VM.Status.RootSnapshots[0]).To(Equal(
						vmopv1.VirtualMachineSnapshotReference{
							Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
							Name: "test-snapshot",
						},
					))
				})
			})
		})

		When("VM has multiple root snapshots", func() {
			var vmSnapshot1, vmSnapshot2 *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-1", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot1)).To(Succeed())
				vmSnapshot2 = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "snapshot-2", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot2)).To(Succeed())

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-2-ref",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1-ref",
							},
							Name: "snapshot-1",
						},
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-2-ref",
							},
							Name: "snapshot-2",
						},
					},
				}
			})

			It("should find and update the root snapshots status with both snapshots", func() {
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.RootSnapshots).ToNot(BeNil())
				Expect(vmCtx.VM.Status.RootSnapshots).To(HaveLen(2))
				Expect(vmCtx.VM.Status.RootSnapshots).To(ContainElement(
					vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot1.Name,
					},
				))
				Expect(vmCtx.VM.Status.RootSnapshots).To(ContainElement(
					vmopv1.VirtualMachineSnapshotReference{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot2.Name,
					},
				))
			})
		})

		When("the root snapshots status is already correct", func() {
			var vmSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				vmSnapshot = builder.DummyVirtualMachineSnapshot(vmCtx.VM.Namespace, "test-snapshot", vmCtx.VM.Name)
				Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

				vmCtx.VM.Status.RootSnapshots = []vmopv1.VirtualMachineSnapshotReference{
					{
						Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
						Name: vmSnapshot.Name,
					},
				}

				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			It("should not change the root snapshots status", func() {
				originalRootSnapshots := vmCtx.VM.Status.RootSnapshots
				Expect(vmlifecycle.ReconcileStatus(vmCtx, ctx.Client, vcVM, data)).To(Succeed())
				Expect(vmCtx.VM.Status.RootSnapshots).To(Equal(originalRootSnapshots))
			})
		})

		When("there's an error getting the VirtualMachineSnapshot custom resource", func() {
			BeforeEach(func() {
				vmCtx.MoVM.Snapshot = &vimtypes.VirtualMachineSnapshotInfo{
					CurrentSnapshot: &vimtypes.ManagedObjectReference{
						Type:  "VirtualMachineSnapshot",
						Value: "snapshot-1",
					},
					RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
						{
							Snapshot: vimtypes.ManagedObjectReference{
								Type:  "VirtualMachineSnapshot",
								Value: "snapshot-1",
							},
							Name: "test-snapshot",
						},
					},
				}
			})

			It("should return an error", func() {
				mockClient := &mockClient{
					Client:   ctx.Client,
					getError: fmt.Errorf("database connection failed"),
				}
				err := vmlifecycle.ReconcileStatus(vmCtx, mockClient, vcVM, data)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection failed"))
			})
		})
	})
})

// mockClient is a simple mock that can be used to simulate client errors.
type mockClient struct {
	ctrlclient.Client
	getError error
}

func (m *mockClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
	if m.getError != nil {
		return m.getError
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

//nolint:unparam
func snapshotRefManaged(snapshotName string) *vmopv1.VirtualMachineSnapshotReference {
	return &vmopv1.VirtualMachineSnapshotReference{
		Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
		Name: snapshotName,
	}
}
