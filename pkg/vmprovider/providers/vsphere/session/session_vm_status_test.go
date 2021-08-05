// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
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
			vm        *vmopv1alpha1.VirtualMachine
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1alpha1.VirtualMachine{}
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
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("ToolsRunningStatus is empty", func() {
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is not running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning)
			})
			It("sets condition to false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.VirtualMachineToolsCondition, vmopv1alpha1.VirtualMachineToolsNotRunningReason, vmopv1alpha1.ConditionSeverityError, "VMware Tools is not running"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is starting", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("Unexpected vmtools running status", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = "blah"
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", "Unexpected VMware Tools running status"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})

var _ = Describe("VSphere Customization Status to VM Status Condition", func() {
	Context("markCustomizationInfoCondition", func() {
		var (
			vm        *vmopv1alpha1.VirtualMachine
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1alpha1.VirtualMachine{}
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
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo unset", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo idle", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo pending", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationPendingReason, vmopv1alpha1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo running", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationRunningReason, vmopv1alpha1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo succeeded", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.GuestCustomizationCondition),
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
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationFailedReason, vmopv1alpha1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
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
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, "", vmopv1alpha1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})
