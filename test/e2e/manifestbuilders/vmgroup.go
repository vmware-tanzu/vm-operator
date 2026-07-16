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

// This struct is needed to serialize the bootOrder.PowerOnDelay field correctly as it's a pointer type in VMOP API.
type BootOrder struct {
	Members      []vmopv1.GroupMember `json:"members,omitempty"`
	PowerOnDelay string               `json:"powerOnDelay,omitempty"`
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
