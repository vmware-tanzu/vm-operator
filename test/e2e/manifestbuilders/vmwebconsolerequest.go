// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

// webConsolePublicKey is the RSA public key used by the web console requests.
const webConsolePublicKey = `-----BEGIN PUBLIC KEY-----
MIIBCgKCAQEAs8F7eAedZ4R1qKDQVOqyOjzToYs62iFqUZ9TnW+0HVO+tmnWq0Tj
TlJ7w46KGpBKxN8KlO82+ovrqkBr4OudQFkn7BbmrZ134phIcc0SQZs2nz9+h1AX
1hSHhozp1mS91XvGlrK0k44a2i6boh257de2rWHh3L5zfPJe31h3L90F43Je9/Oh
FVrm8NUlRzIUd8ADm/dBEu5bUQ+vHIoh/Xqfglf7oRjp8UHuvV/nHI7XmR607QxI
o7QLbIgh3wv4TbfFFJelGpkj7gORSG7gdF7EY0lz5jm/Or4qVUUqAAybyAT1UyiW
hfIIwodsz4QjCpL2LDgxro//gFqWRZurZwIDAQAB
-----END PUBLIC KEY-----
`

type VirtualMachineWebConsoleRequestYaml struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	VMName    string `json:"virtualMachineName"`
}

// GetV1A1WebConsoleRequestYaml returns a v1alpha1 WebConsoleRequest YAML
// manifest.
func GetV1A1WebConsoleRequestYaml(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) []byte {
	return ToYAML(WebConsoleRequestA1(vmWebConsoleRequestYaml))
}

// GetVirtualMachineWebConsoleRequestYaml returns a v1alpha2
// VirtualMachineWebConsoleRequest YAML manifest.
func GetVirtualMachineWebConsoleRequestYaml(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) []byte {
	return ToYAML(VirtualMachineWebConsoleRequestA2(vmWebConsoleRequestYaml))
}

// WebConsoleRequestA1 returns the v1alpha1 WebConsoleRequest described by
// vmWebConsoleRequestYaml.
func WebConsoleRequestA1(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) *vmopv1a1.WebConsoleRequest {
	return &vmopv1a1.WebConsoleRequest{
		TypeMeta: typeMeta(vmopv1a1.GroupVersion.String(), "WebConsoleRequest"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmWebConsoleRequestYaml.Name,
			Namespace: vmWebConsoleRequestYaml.Namespace,
		},
		Spec: vmopv1a1.WebConsoleRequestSpec{
			VirtualMachineName: vmWebConsoleRequestYaml.VMName,
			PublicKey:          webConsolePublicKey,
		},
	}
}

// VirtualMachineWebConsoleRequestA2 returns the v1alpha2
// VirtualMachineWebConsoleRequest described by vmWebConsoleRequestYaml.
func VirtualMachineWebConsoleRequestA2(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) *vmopv1a2.VirtualMachineWebConsoleRequest {
	return &vmopv1a2.VirtualMachineWebConsoleRequest{
		TypeMeta: typeMeta(vmopv1a2.GroupVersion.String(), "VirtualMachineWebConsoleRequest"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmWebConsoleRequestYaml.Name,
			Namespace: vmWebConsoleRequestYaml.Namespace,
		},
		Spec: vmopv1a2.VirtualMachineWebConsoleRequestSpec{
			Name:      vmWebConsoleRequestYaml.VMName,
			PublicKey: webConsolePublicKey,
		},
	}
}
