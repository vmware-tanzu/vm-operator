// Copyright (c) 2021-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("Network Interfaces VM Status", func() {
	Context("nicInfoToNetworkIfStatus", func() {
		dummyMacAddress := "00:50:56:8c:7b:34"
		dummyIPAddress1 := vimTypes.NetIpConfigInfoIpAddress{
			IpAddress:    "192.168.128.5",
			PrefixLength: 16,
		}
		dummyIPAddress2 := vimTypes.NetIpConfigInfoIpAddress{
			IpAddress:    "fe80::250:56ff:fe8c:7b34",
			PrefixLength: 64,
		}
		dummyIPConfig := &vimTypes.NetIpConfigInfo{
			IpAddress: []vimTypes.NetIpConfigInfoIpAddress{
				dummyIPAddress1,
				dummyIPAddress2,
			},
		}
		guestNicInfo := vimTypes.GuestNicInfo{
			Connected:  true,
			MacAddress: dummyMacAddress,
			IpConfig:   dummyIPConfig,
		}

		It("returns populated NetworkInterfaceStatus", func() {
			networkIfStatus := session.NicInfoToNetworkIfStatus(guestNicInfo)
			Expect(networkIfStatus.MacAddress).To(Equal(dummyMacAddress))
			Expect(networkIfStatus.Connected).To(BeTrue())
			Expect(networkIfStatus.IpAddresses[0]).To(Equal("192.168.128.5/16"))
			Expect(networkIfStatus.IpAddresses[1]).To(Equal("fe80::250:56ff:fe8c:7b34/64"))
		})
	})
})

var _ = Describe("VirtualMachineTools Status to VM Status Condition", func() {
	Context("markVMToolsRunningStatusCondition", func() {
		var (
			vm        *vmopv1.VirtualMachine
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &vimTypes.GuestInfo{
				ToolsRunningStatus: "",
			}
		})

		JustBeforeEach(func() {
			session.MarkVMToolsRunningStatusCondition(vm, guestInfo)
		})

		Context("guestInfo is nil", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("ToolsRunningStatus is empty", func() {
			It("sets condition unknown", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is not running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning)
			})
			It("sets condition to false", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason, vmopv1.ConditionSeverityError, "VMware Tools is not running"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.TrueCondition(vmopv1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is starting", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1.Conditions{
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
				expectedConditions := vmopv1.Conditions{
					*conditions.UnknownCondition(vmopv1.VirtualMachineToolsCondition, "", "Unexpected VMware Tools running status"),
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
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			guestInfo = &vimTypes.GuestInfo{
				CustomizationInfo: &vimTypes.GuestInfoCustomizationInfo{},
			}
		})

		JustBeforeEach(func() {
			session.MarkCustomizationInfoCondition(vm, guestInfo)
		})

		Context("guestInfo unset", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.UnknownCondition(vmopv1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo unset", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.UnknownCondition(vmopv1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo idle", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.TrueCondition(vmopv1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo pending", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, vmopv1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo running", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, vmopv1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo succeeded", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.TrueCondition(vmopv1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo failed", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED)
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, vmopv1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
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
				expectedConditions := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.GuestCustomizationCondition, "", vmopv1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
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
			configInfo *vimTypes.VirtualMachineConfigInfo
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			configInfo = &vimTypes.VirtualMachineConfigInfo{}
		})

		JustBeforeEach(func() {
			session.MarkBootstrapCondition(vm, configInfo)
		})

		Context("unknown condition", func() {
			When("configInfo unset", func() {
				BeforeEach(func() {
					configInfo = nil
				})
				It("sets condition unknown", func() {
					expectedConditions := []vmopv1.Condition{
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
					expectedConditions := []vmopv1.Condition{
						*conditions.UnknownCondition(vmopv1.GuestBootstrapCondition, "NoExtraConfig", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("no bootstrap status", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
					}
				})
				It("sets condition unknown", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.UnknownCondition(vmopv1.GuestBootstrapCondition, "NoBootstrapStatus", ""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
		Context("successful condition", func() {
			When("status is 1", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "1",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is TRUE", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "TRUE",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true and there is a reason", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true,my-reason",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.TrueCondition(vmopv1.GuestBootstrapCondition),
					}
					expectedConditions[0].Reason = "my-reason"
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is true and there is a reason and message", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "true,my-reason,my,comma,delimited,message",
						},
					}
				})
				It("sets condition true", func() {
					expectedConditions := []vmopv1.Condition{
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
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "0",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"",
							vmopv1.ConditionSeverityError,
							""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is FALSE", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "FALSE",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"",
							vmopv1.ConditionSeverityError,
							""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is false", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"",
							vmopv1.ConditionSeverityError,
							""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is non-truthy", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "not a boolean value",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"",
							vmopv1.ConditionSeverityError,
							""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is false and there is a reason", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false,my-reason",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"my-reason",
							vmopv1.ConditionSeverityError,
							""),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
			When("status is false and there is a reason and message", func() {
				BeforeEach(func() {
					configInfo.ExtraConfig = []vimTypes.BaseOptionValue{
						&vimTypes.OptionValue{
							Key:   "key1",
							Value: "val1",
						},
						&vimTypes.OptionValue{
							Key:   util.GuestInfoBootstrapCondition,
							Value: "false,my-reason,my,comma,delimited,message",
						},
					}
				})
				It("sets condition false", func() {
					expectedConditions := []vmopv1.Condition{
						*conditions.FalseCondition(
							vmopv1.GuestBootstrapCondition,
							"my-reason",
							vmopv1.ConditionSeverityError,
							"my,comma,delimited,message"),
					}
					Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
				})
			})
		})
	})
})
