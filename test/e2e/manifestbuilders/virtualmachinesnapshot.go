// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

const (
	virtualMachineSnapshotDir = "test/e2e/fixtures/yaml/vmoperator/virtualmachinesnapshot"
)

type VirtualMachineSnapshotYaml struct {
	Namespace        string `json:"namespace,omitempty"`
	Name             string `json:"name,omitempty"`
	VMName           string `json:"vmName,omitempty"`
	Memory           bool   `json:"memory,omitempty"`
	Quiesce          string `json:"quiesce,omitempty"`
	Description      string `json:"description,omitempty"`
	ImportedSnapshot bool   `json:"importedSnapshot,omitempty"`
}

func GetVirtualMachineSnapshotYaml(vmSnapshotYaml VirtualMachineSnapshotYaml) []byte {
	return GetYaml(
		vmSnapshotYaml,
		virtualMachineSnapshotDir,
		"v1alpha5-vmsnapshot.yaml.in",
		"VirtualMachineSnapshot")
}
