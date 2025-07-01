// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&SupervisorProperties{}, &SupervisorPropertiesList{})
}

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:singular=supervisorproperty
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SupervisorProperties is the Schema for the SupervisorProperties API
type SupervisorProperties struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec SupervisorPropertiesSpec `json:"spec,omitempty"`
}

// +kubebuilder:validation:Enum=TINY;SMALL;MEDIUM;LARGE
type ControlPlaneVMSize string

// SupervisorPropertiesSpec defines the values of the properties requested by a Supervisor Service Package.
type SupervisorPropertiesSpec struct {

	// VCenterPublicKeys indicates the base64 encoded vCenter OIDC issuer, client audience and the public keys in JWKS format.
	VCenterPublicKeys string `json:"vcPublicKeys,omitempty"`

	// VCenterPNID indicates the Primary Network Identifier of vCenter.
	VCenterPNID string `json:"vcPNID,omitempty"`

	// NetworkProvider indicates the Network Provider used on Supervisor. (e.g. NSX, nsx-vpc, or vsphere-network)
	NetworkProvider string `json:"networkProvider,omitempty"`

	// VCenterPort indicates the port of vCenter.
	VCenterPort string `json:"vcPort,omitempty"`

	// VirtualIP indicates the IP address of the Kubernetes LoadBalancer type service fronting the apiservers.
	VirtualIP string `json:"virtualIP,omitempty"`

	// SSODomain indicates the name of the default SSO domain configured in vCenter.
	SSODomain string `json:"ssoDomain,omitempty"`

	// ControlPlaneVMSize indicates the capacity of the Supervisor Control Plane. It's derived from Supervisor's tshirt size.
	ControlPlaneVMSize ControlPlaneVMSize `json:"cpVMSize,omitempty"`

	// TMCNamespace indicates the namespace used for TMC to be deployed.
	TMCNamespace string `json:"tmcNamespace,omitempty"`

	// NamespacesCLIPluginVersion indicates the Supervisor recommended namespaces CLIPlugin CR version.
	NamespacesCLIPluginVersion string `json:"namespacesCLIPluginVersion,omitempty"`

	// CloudVCenter indicates if the vCenter is deployed on cloud.
	CloudVCenter bool `json:"cloudVC,omitempty"`

	// PodVMSupported indicates if the Supervisor supports PodVMs.
	PodVMSupported bool `json:"podVMSupported,omitempty"`

	// StretchedSupervisor indicates if the Supervisor is enabled on a set of vSphere Zones.
	StretchedSupervisor bool `json:"stretchedSupervisor,omitempty"`

	// ControlPlaneCount indicates the number of control planes enabled on the Supervisor.
	ControlPlaneCount int `json:"controlPlaneCount,omitempty"`

	// Capabilities defines the capabilities the Supervisor has. The common case of the capability is the feature supported of the vCenter.
	Capabilities []Capability `json:"capabilities,omitempty"`

	// APIServerDNSNames indicates the API server DNS Names associated with the supervisor.
	APIServerDNSNames []string `json:"apiServerDNSNames,omitempty"`
}

// Capability defines the feature supported by the Supervisor.
type Capability struct {

	// The name of the capability.
	Name string `json:"name"`

	// +kubebuilder:default:=false
	// The value indicates if the capability is supported.
	Value bool `json:"value"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SupervisorPropertiesList contains a list of SupervisorProperties
type SupervisorPropertiesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupervisorProperties `json:"items"`
}
