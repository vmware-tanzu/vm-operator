// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

type VirtualMachineGroupPublishRequestYaml struct {
	Namespace               string   `json:"namespace,omitempty"`
	Name                    string   `json:"name,omitempty"`
	Source                  string   `json:"source,omitempty"`
	Target                  string   `json:"target,omitempty"`
	VirtualMachines         []string `json:"virtualMachines,omitempty"`
	TTLSecondsAfterFinished int64    `json:"ttlSecondsAfterFinished,omitempty"`
}

func GetVirtualMachineGroupPublishRequestYaml(vmGroupPubYaml VirtualMachineGroupPublishRequestYaml) []byte {
	return GetYaml(
		vmGroupPubYaml,
		"test/e2e/fixtures/yaml/vmoperator/virtualmachinegrouppublishrequests",
		"vm-group-publish.yaml.in",
		"VirtualMachineGroupPublishRequest")
}
