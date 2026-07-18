// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

type VirtualMachineGroupYaml struct {
	Namespace                   string               `json:"namespace,omitempty"`
	Name                        string               `json:"name,omitempty"`
	GroupName                   string               `json:"groupName,omitempty"`
	PowerState                  string               `json:"powerState,omitempty"`
	PowerOffMode                string               `json:"powerOffMode,omitempty"`
	NextForcePowerStateSyncTime string               `json:"nextForcePowerStateSyncTime,omitempty"`
	Members                     []vmopv1.GroupMember `json:"members,omitempty"`
	BootOrder                   []BootOrder          `json:"bootOrder,omitempty"`
}

// This struct is needed to serialize the bootOrder.PowerOnDelay/PowerOffDelay
// fields correctly as they're pointer types in the VMOP API.
//
// PowerOffDelay is only defined on the v1alpha6 (hub) API, so it is only
// rendered by the v1alpha6 boot-order template
// (GetVirtualMachineGroupWithBootOrderYamlV1Alpha6); the v1alpha5 template
// ignores it.
type BootOrder struct {
	Members       []vmopv1.GroupMember `json:"members,omitempty"`
	PowerOnDelay  string               `json:"powerOnDelay,omitempty"`
	PowerOffDelay string               `json:"powerOffDelay,omitempty"`
}

func GetVirtualMachineGroupYaml(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return GetYaml(
		vmGroupYaml,
		"test/e2e/fixtures/yaml/vmoperator/virtualmachinegroups",
		"vm-group.yaml.in",
		"VirtualMachineGroup")
}

func GetVirtualMachineGroupWithBootOrderYaml(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return GetYaml(
		vmGroupYaml,
		"test/e2e/fixtures/yaml/vmoperator/virtualmachinegroups",
		"vm-group-with-boot-order.yaml.in",
		"VirtualMachineGroup")
}

// GetVirtualMachineGroupWithBootOrderYamlV1Alpha6 renders a VirtualMachineGroup
// using the v1alpha6 (hub) apiVersion, which is required to set
// bootOrder[].powerOffDelay (only defined on the hub API).
func GetVirtualMachineGroupWithBootOrderYamlV1Alpha6(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return GetYaml(
		vmGroupYaml,
		"test/e2e/fixtures/yaml/vmoperator/virtualmachinegroups",
		"vm-group-with-boot-order-v1alpha6.yaml.in",
		"VirtualMachineGroup")
}
