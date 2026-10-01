// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

type VirtualMachinePublishRequestSource struct {
	Name string `json:"name,omitempty"`
}

type VirtualMachinePublishRequestTarget struct {
	Item     VirtualMachinePublishRequestTargetItem     `json:"item,omitempty"`
	Location VirtualMachinePublishRequestTargetLocation `json:"location,omitempty"`
}

type VirtualMachinePublishRequestTargetItem struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type VirtualMachinePublishRequestTargetLocation struct {
	Name string `json:"name,omitempty"`
}

type VirtualMachinePublishRequestYaml struct {
	Namespace   string                             `json:"namespace,omitempty"`
	Name        string                             `json:"name,omitempty"`
	Labels      map[string]string                  `json:"labels,omitempty"`
	Annotations map[string]string                  `json:"annotations,omitempty"`
	Source      VirtualMachinePublishRequestSource `json:"source,omitempty"`
	Target      VirtualMachinePublishRequestTarget `json:"target,omitempty"`
}

// GetVirtualMachinePublishRequestYaml returns a v1alpha2
// VirtualMachinePublishRequest YAML manifest.
func GetVirtualMachinePublishRequestYaml(vmPublishRequestYaml VirtualMachinePublishRequestYaml) []byte {
	return ToYAML(VirtualMachinePublishRequestA2(vmPublishRequestYaml))
}

// VirtualMachinePublishRequestA2 returns the v1alpha2
// VirtualMachinePublishRequest described by vmPublishRequestYaml.
func VirtualMachinePublishRequestA2(vmPublishRequestYaml VirtualMachinePublishRequestYaml) *vmopv1a2.VirtualMachinePublishRequest {
	return &vmopv1a2.VirtualMachinePublishRequest{
		TypeMeta: typeMeta(vmopv1a2.GroupVersion.String(), "VirtualMachinePublishRequest"),
		ObjectMeta: metav1.ObjectMeta{
			Name:        vmPublishRequestYaml.Name,
			Namespace:   vmPublishRequestYaml.Namespace,
			Labels:      vmPublishRequestYaml.Labels,
			Annotations: vmPublishRequestYaml.Annotations,
		},
		Spec: vmopv1a2.VirtualMachinePublishRequestSpec{
			Source: vmopv1a2.VirtualMachinePublishRequestSource{
				Name: vmPublishRequestYaml.Source.Name,
			},
			Target: vmopv1a2.VirtualMachinePublishRequestTarget{
				Item: vmopv1a2.VirtualMachinePublishRequestTargetItem{
					Name:        vmPublishRequestYaml.Target.Item.Name,
					Description: vmPublishRequestYaml.Target.Item.Description,
				},
				Location: vmopv1a2.VirtualMachinePublishRequestTargetLocation{
					Name: vmPublishRequestYaml.Target.Location.Name,
				},
			},
		},
	}
}
