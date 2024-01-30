// Copyright (c) 2021-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"strconv"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func ipCIDRNotation(ipAddress string, prefix int32) string {
	return ipAddress + "/" + strconv.Itoa(int(prefix))
}

func NicInfoToNetworkIfStatus(nicInfo vimTypes.GuestNicInfo) vmopv1.NetworkInterfaceStatus {
	var ipAddresses []string
	if nicInfo.IpConfig != nil {
		ipAddresses = make([]string, 0, len(nicInfo.IpConfig.IpAddress))
		for _, ipAddress := range nicInfo.IpConfig.IpAddress {
			ipAddresses = append(ipAddresses, ipCIDRNotation(ipAddress.IpAddress, ipAddress.PrefixLength))
		}
	}
	return vmopv1.NetworkInterfaceStatus{
		Connected:   nicInfo.Connected,
		MacAddress:  nicInfo.MacAddress,
		IpAddresses: ipAddresses,
	}
}

func MarkVMToolsRunningStatusCondition(vm *vmopv1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		msg := "VMware Tools is not running"
		conditions.MarkFalse(vm, vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason,
			vmopv1.ConditionSeverityError, msg)
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning), string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, vmopv1.VirtualMachineToolsCondition)
	default:
		msg := "Unexpected VMware Tools running status"
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "", msg)
	}
}

func MarkCustomizationInfoCondition(vm *vmopv1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, vmopv1.GuestCustomizationCondition, "", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, vmopv1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, vmopv1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, vmopv1.ConditionSeverityError, errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, "", vmopv1.ConditionSeverityError, errorMsg)
	}
}

func MarkBootstrapCondition(
	vm *vmopv1.VirtualMachine,
	configInfo *vimTypes.VirtualMachineConfigInfo) {

	if configInfo == nil {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoConfigInfo", "")
		return
	}

	if len(configInfo.ExtraConfig) == 0 {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoExtraConfig", "")
		return
	}

	status, reason, msg, ok := util.GetBootstrapConditionValues(configInfo)
	if !ok {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoBootstrapStatus", "")
		return
	}
	if status {
		c := conditions.TrueCondition(vmopv1.GuestBootstrapCondition)
		if reason != "" {
			c.Reason = reason
		}
		c.Message = msg
		conditions.Set(vm, c)
	} else {
		conditions.MarkFalse(
			vm,
			vmopv1.GuestBootstrapCondition,
			reason,
			vmopv1.ConditionSeverityError,
			msg)
	}
}

func (s *Session) updateVMStatus(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine) error {

	// TODO: We could be smarter about not re-fetching the config: if we didn't do a
	// reconfigure or power change, the prior config is still entirely valid.
	moVM, err := resVM.GetProperties(vmCtx, []string{
		"config.changeTrackingEnabled",
		"config.extraConfig",
		"guest",
		"summary",
	})
	if err != nil {
		// Leave the current Status unchanged.
		return err
	}

	var errs []error
	vm := vmCtx.VM
	summary := moVM.Summary

	vm.Status.Phase = vmopv1.Created
	vm.Status.PowerState = vmopv1.VirtualMachinePowerState(summary.Runtime.PowerState)
	vm.Status.UniqueID = resVM.MoRef().Value
	vm.Status.BiosUUID = summary.Config.Uuid
	vm.Status.InstanceUUID = summary.Config.InstanceUuid
	vm.Status.HardwareVersion = util.ParseVirtualHardwareVersion(summary.Config.HwVersion)

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
		var networkIfStatuses []vmopv1.NetworkInterfaceStatus
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
	MarkBootstrapCondition(vm, moVM.Config)

	if config := moVM.Config; config != nil {
		vm.Status.ChangeBlockTracking = config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}

	zoneName := vm.Labels[topology.KubernetesTopologyZoneLabelKey]

	if zoneName == "" {
		var err error
		zoneName, err = topology.LookupZoneForClusterMoID(vmCtx, s.K8sClient, s.Cluster.Reference().Value)
		if err != nil {
			errs = append(errs, err)
		} else {
			if vm.Labels == nil {
				vm.Labels = map[string]string{}
			}
			vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}
	}

	if zoneName != "" {
		vm.Status.Zone = zoneName
	}

	return k8serrors.NewAggregate(errs)
}
