// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("UpdateStatus", func() {

	var (
		ctx   *builder.TestContextForVCSim
		vmCtx context.VirtualMachineContext
		vcVM  *object.VirtualMachine
		moVM  *mo.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		vm := builder.DummyVirtualMachineA2()
		vm.Name = "update-status-test"

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		moVM = &mo.VirtualMachine{}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		err := vmlifecycle.UpdateStatus(vmCtx, ctx.Client, vcVM, moVM)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Network", func() {

		Context("PrimaryIP", func() {
			const (
				invalid  = "abc"
				validIP4 = "192.168.0.2"
				validIP6 = "FD00:F53B:82E4::54"
			)
			BeforeEach(func() {
				moVM.Guest = &types.GuestInfo{}
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
						moVM.Guest.IpAddress = invalid
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is unspecified", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "0.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local multicast", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "224.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local unicast", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "169.254.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is loopback", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "127.0.0.0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is ip6", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = validIP6
					})
					Specify("status.network.primaryIP4 should be empty", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(BeEmpty())
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(Equal(validIP6))
					})
				})
				When("address is valid", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = validIP4
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
						moVM.Guest.IpAddress = invalid
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is unspecified", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "::0"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local multicast", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "FF02:::"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is link-local unicast", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "FE80:::"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is loopback", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = "::1"
					})
					Specify("status.network should be nil", func() {
						Expect(vmCtx.VM.Status.Network).To(BeNil())
					})
				})
				When("address is ip4", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = validIP4
					})
					Specify("status.network.primaryIP6 should be empty", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(Equal(validIP4))
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(BeEmpty())
					})
				})
				When("address is valid", func() {
					BeforeEach(func() {
						moVM.Guest.IpAddress = validIP6
					})
					Specify("status.network.primaryIP6 should be expected value", func() {
						Expect(vmCtx.VM.Status.Network).ToNot(BeNil())
						Expect(vmCtx.VM.Status.Network.PrimaryIP4).To(BeEmpty())
						Expect(vmCtx.VM.Status.Network.PrimaryIP6).To(Equal(validIP6))
					})
				})
			})
		})

		Context("Interfaces", func() {
			Context("VM has pseudo devices", func() {
				BeforeEach(func() {
					moVM.Guest = &types.GuestInfo{
						Net: []types.GuestNicInfo{
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
					moVM.Guest = &types.GuestInfo{
						Net: []types.GuestNicInfo{
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
		})

		Context("IPRoutes", func() {
			BeforeEach(func() {
				moVM.Guest = &types.GuestInfo{
					IpStack: []types.GuestStackInfo{
						{
							IpRouteConfig: &types.NetIpRouteConfigInfo{
								IpRoute: []types.NetIpRouteConfigInfoIpRoute{
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
					moVM.Guest = nil
					vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
				})

				Specify("status.network should be nil", func() {
					Expect(vmCtx.VM.Status.Network).To(BeNil())
				})
			})

			When("Guest property is empty", func() {
				BeforeEach(func() {
					moVM.Guest = &types.GuestInfo{}
					vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
				})

				Specify("status.network should be nil", func() {
					Expect(vmCtx.VM.Status.Network).To(BeNil())
				})
			})

			When("Empty guest property but has existing status.network.config", func() {
				BeforeEach(func() {
					moVM.Guest = &types.GuestInfo{}
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

	Context("Copies values to the VM status", func() {
		biosUUID, instanceUUID := "f7c371d6-2003-5a48-9859-3bc9a8b0890", "6132d223-1566-5921-bc3b-df91ece09a4d"
		BeforeEach(func() {
			moVM.Summary = types.VirtualMachineSummary{
				Config: types.VirtualMachineConfigSummary{
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
})

var _ = Describe("VirtualMachineTools Status to VM Status Condition", func() {
	Context("markVMToolsRunningStatusCondition", func() {
		var (
			vm        *vmopv1.VirtualMachine
			guestInfo *types.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &types.GuestInfo{
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
				guestInfo.ToolsRunningStatus = string(types.VirtualMachineToolsRunningStatusGuestToolsNotRunning)
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
				guestInfo.ToolsRunningStatus = string(types.VirtualMachineToolsRunningStatusGuestToolsRunning)
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
				guestInfo.ToolsRunningStatus = string(types.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts)
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
			guestInfo *types.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &types.GuestInfo{
				CustomizationInfo: &types.GuestInfoCustomizationInfo{},
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
				guestInfo.CustomizationInfo.CustomizationStatus = string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE)
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
				guestInfo.CustomizationInfo.CustomizationStatus = string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
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
				guestInfo.CustomizationInfo.CustomizationStatus = string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
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
				guestInfo.CustomizationInfo.CustomizationStatus = string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED)
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
				guestInfo.CustomizationInfo.CustomizationStatus = string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED)
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := []metav1.Condition{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, guestInfo.CustomizationInfo.ErrorMsg),
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
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, "Unknown", guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})

var _ = Describe("VSphere Bootstrap Status to VM Status Condition", func() {
	Context("MarkBootstrapCondition", func() {
		var (
			vm         *vmopv1.VirtualMachine
			configInfo *types.VirtualMachineConfigInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			configInfo = &types.VirtualMachineConfigInfo{}
		})

		JustBeforeEach(func() {
			vmlifecycle.MarkBootstrapCondition(vm, configInfo)
		})

		Context("unknown condition", func() {
			When("configInfo unset", func() {
				BeforeEach(func() {
					configInfo = nil
				})
				It("sets condition unknown", func() {
					expectedConditions := []metav1.Condition{
						*conditions.UnknownCondition(vmopv1.GuestBootstrapCondition, "NoConfigInfo", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("extraConfig unset", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = nil
				})
				It("sets condition unknown", func() {
					expectedConditions := []metav1.Condition{
						*conditions.UnknownCondition(vmopv1.GuestBootstrapCondition, "NoExtraConfig", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("no bootstrap status", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
					}
				})
				It("sets condition unknown", func() {
					expectedConditions := []metav1.Condition{
						*conditions.UnknownCondition(vmopv1.GuestBootstrapCondition, "NoBootstrapStatus", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
		Context("successful condition", func() {
			When("status is 1", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "1",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []metav1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is TRUE", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "TRUE",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []metav1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true,my-reason",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true,my-reason,my,comma,delimited,message",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "0",
						},
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
			When("status is FALSE", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "FALSE",
						},
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
			When("status is false", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "not a boolean value",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false,my-reason",
						},
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
					configInfo.ExtraConfig = []types.BaseOptionValue{
						&types.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&types.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false,my-reason,my,comma,delimited,message",
						},
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
			Hostname:       "my-vm",
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
						args.Hostname = ""
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
