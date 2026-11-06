// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// importedSnapshotAnnotation marks a VirtualMachineSnapshot as imported.
const importedSnapshotAnnotation = "vmoperator.vmware.com/imported-snapshot"

type VirtualMachineSnapshotYaml struct {
	Namespace        string `json:"namespace,omitempty"`
	Name             string `json:"name,omitempty"`
	VMName           string `json:"vmName,omitempty"`
	Memory           bool   `json:"memory,omitempty"`
	Quiesce          string `json:"quiesce,omitempty"`
	Description      string `json:"description,omitempty"`
	ImportedSnapshot bool   `json:"importedSnapshot,omitempty"`
}

// GetVirtualMachineSnapshotYaml returns a v1alpha5 VirtualMachineSnapshot YAML
// manifest.
func GetVirtualMachineSnapshotYaml(vmSnapshotYaml VirtualMachineSnapshotYaml) []byte {
	return ToYAML(must(VirtualMachineSnapshotA5(vmSnapshotYaml)))
}

// VirtualMachineSnapshotA5 returns the v1alpha5 VirtualMachineSnapshot
// described by vmSnapshotYaml. An error is returned if Quiesce is not a valid
// duration.
func VirtualMachineSnapshotA5(vmSnapshotYaml VirtualMachineSnapshotYaml) (*vmopv1a5.VirtualMachineSnapshot, error) {
	obj := &vmopv1a5.VirtualMachineSnapshot{
		TypeMeta: typeMeta(vmopv1a5.GroupVersion.String(), "VirtualMachineSnapshot"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmSnapshotYaml.Name,
			Namespace: vmSnapshotYaml.Namespace,
		},
		Spec: vmopv1a5.VirtualMachineSnapshotSpec{
			VMName:      vmSnapshotYaml.VMName,
			Memory:      vmSnapshotYaml.Memory,
			Description: vmSnapshotYaml.Description,
		},
	}

	if vmSnapshotYaml.ImportedSnapshot {
		obj.Annotations = map[string]string{
			importedSnapshotAnnotation: "",
		}
	}

	timeout, err := parseDuration(vmSnapshotYaml.Quiesce)
	if err != nil {
		return nil, fmt.Errorf("invalid quiesce timeout: %w", err)
	}
	if timeout != nil {
		obj.Spec.Quiesce = &vmopv1a5.QuiesceSpec{
			Timeout: timeout,
		}
	}

	return obj, nil
}
