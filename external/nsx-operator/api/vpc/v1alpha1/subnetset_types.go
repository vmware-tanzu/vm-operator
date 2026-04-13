/* Copyright © 2022-2023 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubnetSetSpec defines the desired state of SubnetSet.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.accessMode) || has(self.accessMode)", message="accessMode is required once set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.ipv4SubnetSize) || has(self.ipv4SubnetSize)", message="ipv4SubnetSize is required once set"
// +kubebuilder:validation:XValidation:rule="!has(self.subnetDHCPConfig) || has(self.subnetDHCPConfig) && !has(self.subnetDHCPConfig.dhcpServerAdditionalConfig) || has(self.subnetDHCPConfig) && has(self.subnetDHCPConfig.dhcpServerAdditionalConfig) && !has(self.subnetDHCPConfig.dhcpServerAdditionalConfig.reservedIPRanges)", message="reservedIPRanges is not supported in SubnetSet"
type SubnetSetSpec struct {
	// Size of Subnet based upon estimated workload count.
	// +kubebuilder:validation:Maximum:=65536
	// +kubebuilder:validation:Minimum:=16
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	IPv4SubnetSize int `json:"ipv4SubnetSize,omitempty"`
	// Access mode of Subnet, accessible only from within VPC or from outside VPC.
	// +kubebuilder:validation:Enum=Private;Public;PrivateTGW
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	AccessMode AccessMode `json:"accessMode,omitempty"`
	// DHCP mode of a Subnet can only switch between DHCPServer or DHCPRelay.
	// If subnetDHCPConfig is not set, the DHCP mode is DHCPDeactivated by default.
	// In order to enforce this rule, three XValidation rules are defined.
	// The rule on SubnetSpec prevents the condition that subnetDHCPConfig is not set in
	// old or current SubnetSpec while the current or old SubnetSpec specifies a Mode
	// other than DHCPDeactivated.
	// The rule on SubnetDHCPConfig prevents the condition that Mode is not set in old
	// or current SubnetDHCPConfig while the current or old one specifies a Mode other
	// than DHCPDeactivated.
	// The rule on SubnetDHCPConfig.Mode prevents the Mode changing between DHCPDeactivated
	// and DHCPServer or DHCPRelay.

	// Subnet DHCP configuration.
	SubnetDHCPConfig SubnetDHCPConfig `json:"subnetDHCPConfig,omitempty"`
}

// SubnetInfo defines the observed state of a single Subnet of a SubnetSet.
type SubnetInfo struct {
	// Network address of the Subnet.
	NetworkAddresses []string `json:"networkAddresses,omitempty"`
	// Gateway address of the Subnet.
	GatewayAddresses []string `json:"gatewayAddresses,omitempty"`
	// Dhcp server IP address.
	DHCPServerAddresses []string `json:"DHCPServerAddresses,omitempty"`
}

// SubnetSetStatus defines the observed state of SubnetSet.
type SubnetSetStatus struct {
	Conditions []Condition  `json:"conditions,omitempty"`
	Subnets    []SubnetInfo `json:"subnets,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// SubnetSet is the Schema for the subnetsets API.
// +kubebuilder:printcolumn:name="AccessMode",type=string,JSONPath=`.spec.accessMode`,description="Access mode of Subnet"
// +kubebuilder:printcolumn:name="IPv4SubnetSize",type=string,JSONPath=`.spec.ipv4SubnetSize`,description="Size of Subnet"
// +kubebuilder:printcolumn:name="NetworkAddresses",type=string,JSONPath=`.status.subnets[*].networkAddresses[*]`,description="CIDRs for the SubnetSet"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.spec) || has(self.spec)", message="spec is required once set"
type SubnetSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSetSpec   `json:"spec,omitempty"`
	Status SubnetSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubnetSetList contains a list of SubnetSet.
type SubnetSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubnetSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SubnetSet{}, &SubnetSetList{})
}
