// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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

// BootOrder is needed to serialize the bootOrder.PowerOnDelay/PowerOffDelay
// fields correctly as their pointer types in the VMOP API.
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

// GetVirtualMachineGroupYaml returns a v1alpha5 VirtualMachineGroup YAML
// manifest whose Members form a single boot order group.
func GetVirtualMachineGroupYaml(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return ToYAML(VirtualMachineGroupA5(vmGroupYaml))
}

// GetVirtualMachineGroupWithBootOrderYaml returns a v1alpha5
// VirtualMachineGroup YAML manifest using the explicit BootOrder.
func GetVirtualMachineGroupWithBootOrderYaml(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return ToYAML(must(VirtualMachineGroupWithBootOrderA5(vmGroupYaml)))
}

// GetVirtualMachineGroupWithBootOrderYamlV1Alpha6 returns a v1alpha6
// VirtualMachineGroup YAML manifest using the explicit BootOrder. The v1alpha6
// (hub) API is required to set bootOrder[].powerOffDelay.
func GetVirtualMachineGroupWithBootOrderYamlV1Alpha6(vmGroupYaml VirtualMachineGroupYaml) []byte {
	return ToYAML(must(VirtualMachineGroupWithBootOrderA6(vmGroupYaml)))
}

// VirtualMachineGroupA5 returns a v1alpha5 VirtualMachineGroup whose Members
// form a single boot order group.
func VirtualMachineGroupA5(vmGroupYaml VirtualMachineGroupYaml) *vmopv1a5.VirtualMachineGroup {
	obj := &vmopv1a5.VirtualMachineGroup{
		TypeMeta: typeMeta(vmopv1a5.GroupVersion.String(), "VirtualMachineGroup"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmGroupYaml.Name,
			Namespace: vmGroupYaml.Namespace,
		},
		Spec: vmopv1a5.VirtualMachineGroupSpec{
			GroupName: vmGroupYaml.GroupName,
		},
	}

	if len(vmGroupYaml.Members) > 0 {
		obj.Spec.BootOrder = []vmopv1a5.VirtualMachineGroupBootOrderGroup{
			{
				Members: groupMembersToV1A5(vmGroupYaml.Members),
			},
		}
	}

	return obj
}

// VirtualMachineGroupWithBootOrderA5 returns a v1alpha5 VirtualMachineGroup
// using the explicit BootOrder. BootOrder[].PowerOffDelay is ignored since it
// is not part of the v1alpha5 API. An error is returned if a delay is not a
// valid duration.
func VirtualMachineGroupWithBootOrderA5(vmGroupYaml VirtualMachineGroupYaml) (*vmopv1a5.VirtualMachineGroup, error) {
	obj := &vmopv1a5.VirtualMachineGroup{
		TypeMeta: typeMeta(vmopv1a5.GroupVersion.String(), "VirtualMachineGroup"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmGroupYaml.Name,
			Namespace: vmGroupYaml.Namespace,
		},
		Spec: vmopv1a5.VirtualMachineGroupSpec{
			GroupName:                   vmGroupYaml.GroupName,
			PowerState:                  vmopv1a5.VirtualMachinePowerState(vmGroupYaml.PowerState),
			PowerOffMode:                vmopv1a5.VirtualMachinePowerOpMode(vmGroupYaml.PowerOffMode),
			NextForcePowerStateSyncTime: vmGroupYaml.NextForcePowerStateSyncTime,
		},
	}

	for _, bo := range vmGroupYaml.BootOrder {
		powerOnDelay, err := parseDuration(bo.PowerOnDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid powerOnDelay: %w", err)
		}
		obj.Spec.BootOrder = append(obj.Spec.BootOrder, vmopv1a5.VirtualMachineGroupBootOrderGroup{
			Members:      groupMembersToV1A5(bo.Members),
			PowerOnDelay: powerOnDelay,
		})
	}

	return obj, nil
}

// VirtualMachineGroupWithBootOrderA6 returns a v1alpha6 VirtualMachineGroup
// using the explicit BootOrder. An error is returned if a delay is not a valid
// duration.
func VirtualMachineGroupWithBootOrderA6(vmGroupYaml VirtualMachineGroupYaml) (*vmopv1.VirtualMachineGroup, error) {
	obj := &vmopv1.VirtualMachineGroup{
		TypeMeta: typeMeta(vmopv1.GroupVersion.String(), "VirtualMachineGroup"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmGroupYaml.Name,
			Namespace: vmGroupYaml.Namespace,
		},
		Spec: vmopv1.VirtualMachineGroupSpec{
			GroupName:                   vmGroupYaml.GroupName,
			PowerState:                  vmopv1.VirtualMachinePowerState(vmGroupYaml.PowerState),
			PowerOffMode:                vmopv1.VirtualMachinePowerOpMode(vmGroupYaml.PowerOffMode),
			NextForcePowerStateSyncTime: vmGroupYaml.NextForcePowerStateSyncTime,
		},
	}

	for _, bo := range vmGroupYaml.BootOrder {
		powerOnDelay, err := parseDuration(bo.PowerOnDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid powerOnDelay: %w", err)
		}
		powerOffDelay, err := parseDuration(bo.PowerOffDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid powerOffDelay: %w", err)
		}
		obj.Spec.BootOrder = append(obj.Spec.BootOrder, vmopv1.VirtualMachineGroupBootOrderGroup{
			Members:       slices.Clone(bo.Members),
			PowerOnDelay:  powerOnDelay,
			PowerOffDelay: powerOffDelay,
		})
	}

	return obj, nil
}

func groupMembersToV1A5(in []vmopv1.GroupMember) []vmopv1a5.GroupMember {
	if in == nil {
		return nil
	}

	out := make([]vmopv1a5.GroupMember, len(in))
	for i, m := range in {
		out[i] = vmopv1a5.GroupMember{
			Name: m.Name,
			Kind: m.Kind,
		}
	}

	return out
}

// parseDuration parses s as a duration. An empty string returns nil.
func parseDuration(s string) (*metav1.Duration, error) {
	if s == "" {
		return nil, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, err
	}

	return &metav1.Duration{Duration: d}, nil
}
