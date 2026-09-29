// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type VirtualMachineGroupPublishRequestYaml struct {
	Namespace               string   `json:"namespace,omitempty"`
	Name                    string   `json:"name,omitempty"`
	Source                  string   `json:"source,omitempty"`
	Target                  string   `json:"target,omitempty"`
	VirtualMachines         []string `json:"virtualMachines,omitempty"`
	TTLSecondsAfterFinished int64    `json:"ttlSecondsAfterFinished,omitempty"`
}

// GetVirtualMachineGroupPublishRequestYaml returns a v1alpha5
// VirtualMachineGroupPublishRequest YAML manifest.
func GetVirtualMachineGroupPublishRequestYaml(vmGroupPubYaml VirtualMachineGroupPublishRequestYaml) []byte {
	return ToYAML(VirtualMachineGroupPublishRequestA5(vmGroupPubYaml))
}

// VirtualMachineGroupPublishRequestA5 returns the v1alpha5
// VirtualMachineGroupPublishRequest described by vmGroupPubYaml.
func VirtualMachineGroupPublishRequestA5(vmGroupPubYaml VirtualMachineGroupPublishRequestYaml) *vmopv1a5.VirtualMachineGroupPublishRequest {
	obj := &vmopv1a5.VirtualMachineGroupPublishRequest{
		TypeMeta: typeMeta(vmopv1a5.GroupVersion.String(), "VirtualMachineGroupPublishRequest"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmGroupPubYaml.Name,
			Namespace: vmGroupPubYaml.Namespace,
		},
		Spec: vmopv1a5.VirtualMachineGroupPublishRequestSpec{
			Source:          vmGroupPubYaml.Source,
			Target:          vmGroupPubYaml.Target,
			VirtualMachines: slices.Clone(vmGroupPubYaml.VirtualMachines),
		},
	}

	if vmGroupPubYaml.TTLSecondsAfterFinished != 0 {
		obj.Spec.TTLSecondsAfterFinished = ptr.To(vmGroupPubYaml.TTLSecondsAfterFinished)
	}

	return obj
}
