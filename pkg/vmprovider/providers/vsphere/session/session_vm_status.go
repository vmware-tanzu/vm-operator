// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"strconv"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func ipCIDRNotation(ipAddress string, prefix int32) string {
	return ipAddress + "/" + strconv.Itoa(int(prefix))
}

func NicInfoToNetworkIfStatus(nicInfo vimTypes.GuestNicInfo) v1alpha1.NetworkInterfaceStatus {
	var ipAddresses []string
	if nicInfo.IpConfig != nil {
		ipAddresses = make([]string, 0, len(nicInfo.IpConfig.IpAddress))
		for _, ipAddress := range nicInfo.IpConfig.IpAddress {
			ipAddresses = append(ipAddresses, ipCIDRNotation(ipAddress.IpAddress, ipAddress.PrefixLength))
		}
	}
	return v1alpha1.NetworkInterfaceStatus{
		Connected:   nicInfo.Connected,
		MacAddress:  nicInfo.MacAddress,
		IpAddresses: ipAddresses,
	}
}

func MarkVMToolsRunningStatusCondition(vm *v1alpha1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, v1alpha1.VirtualMachineToolsCondition, "", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		msg := "VMware Tools is not running"
		conditions.MarkFalse(vm, v1alpha1.VirtualMachineToolsCondition, v1alpha1.VirtualMachineToolsNotRunningReason,
			v1alpha1.ConditionSeverityError, msg)
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning), string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, v1alpha1.VirtualMachineToolsCondition)
	default:
		msg := "Unexpected VMware Tools running status"
		conditions.MarkUnknown(vm, v1alpha1.VirtualMachineToolsCondition, "", msg)
	}
}

func MarkCustomizationInfoCondition(vm *v1alpha1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, v1alpha1.GuestCustomizationCondition, "", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, v1alpha1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationPendingReason, v1alpha1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationRunningReason, v1alpha1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, v1alpha1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationFailedReason, v1alpha1.ConditionSeverityError, errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, "", v1alpha1.ConditionSeverityError, errorMsg)
	}
}

func (s *Session) updateVMStatus(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine) error {

	// TODO: We could be smarter about not re-fetching the config: if we didn't do a
	// reconfigure or power change, the prior config is still entirely valid.
	moVM, err := resVM.GetProperties(vmCtx, []string{"config.changeTrackingEnabled", "guest", "summary"})
	if err != nil {
		// Leave the current Status unchanged.
		return err
	}

	var errs []error
	vm := vmCtx.VM
	summary := moVM.Summary

	vm.Status.Phase = v1alpha1.Created
	vm.Status.PowerState = v1alpha1.VirtualMachinePowerState(summary.Runtime.PowerState)
	vm.Status.UniqueID = resVM.MoRef().Value
	vm.Status.BiosUUID = summary.Config.Uuid
	vm.Status.InstanceUUID = summary.Config.InstanceUuid

	if host := summary.Runtime.Host; host != nil {
		hostSystem := object.NewHostSystem(s.Client.VimClient(), *host)
		if hostName, err := hostSystem.ObjectName(vmCtx); err != nil {
			// Leave existing vm.Status.Host value.
			errs = append(errs, err)
		} else {
			vm.Status.Host = hostName
		}
	} else {
		vm.Status.Host = ""
	}

	guestInfo := moVM.Guest

	if guestInfo != nil {
		vm.Status.VmIp = guestInfo.IpAddress
		var networkIfStatuses []v1alpha1.NetworkInterfaceStatus
		for _, nicInfo := range guestInfo.Net {
			networkIfStatuses = append(networkIfStatuses, NicInfoToNetworkIfStatus(nicInfo))
		}
		vm.Status.NetworkInterfaces = networkIfStatuses
	} else {
		vm.Status.VmIp = ""
		vm.Status.NetworkInterfaces = nil
	}

	MarkCustomizationInfoCondition(vm, guestInfo)
	MarkVMToolsRunningStatusCondition(vm, guestInfo)

	if config := moVM.Config; config != nil {
		vm.Status.ChangeBlockTracking = config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}

	if lib.IsWcpFaultDomainsFSSEnabled() {
		vm.Status.Zone = vm.Labels[topology.KubernetesTopologyZoneLabelKey]
	}

	return k8serrors.NewAggregate(errs)
}
