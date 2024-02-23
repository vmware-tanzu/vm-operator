// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("UpdateStatus", func() {

	var (
		ctx   *builder.TestContextForVCSim
		err   error
		vmCtx context.VirtualMachineContextA2
		vcVM  *object.VirtualMachine
		vmMO  *mo.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		vm := builder.DummyVirtualMachineA2()
		vm.Name = "update-status-test"

		vmCtx = context.VirtualMachineContextA2{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vmMO = &mo.VirtualMachine{}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		err = vmlifecycle.UpdateStatus(vmCtx, ctx.Client, vcVM, vmMO)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Network", func() {

		Context("Interfaces", func() {

			Context("VM has pseudo devices", func() {
				BeforeEach(func() {
					vmMO.Guest = &types.GuestInfo{
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
					vmMO.Guest = &types.GuestInfo{
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
				vmMO.Guest = &types.GuestInfo{
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

				Expect(network.IPRoutes).To(HaveLen(2))
				Expect(network.IPRoutes[0].NetworkAddress).To(Equal("192.168.1.0/24"))
				Expect(network.IPRoutes[1].NetworkAddress).To(Equal("e9ef:6df5:eb14:42e2:5c09:9982:a9b5:8c2b/48"))
			})
		})
	})

	Context("Copies values to the VM status", func() {
		biosUUID, instanceUUID := "f7c371d6-2003-5a48-9859-3bc9a8b0890", "6132d223-1566-5921-bc3b-df91ece09a4d"
		BeforeEach(func() {
			vmMO.Summary = types.VirtualMachineSummary{
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
