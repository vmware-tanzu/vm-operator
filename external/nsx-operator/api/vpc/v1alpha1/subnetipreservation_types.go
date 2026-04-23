/* Copyright © 2025 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubnetIPReservationSpec defines the desired state of SubnetIPReservation
type SubnetIPReservationSpec struct {
	// Subnet specifies the Subnet to reserve IPs from.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Subnet is immutable"
	Subnet string `json:"subnet"`

	// NumberOfIPs defines number of IPs requested to be reserved.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="NumberOfIPs is immutable"
	// +kubebuilder:validation:Maximum:=100
	// +kubebuilder:validation:Minimum:=1
	NumberOfIPs int `json:"numberOfIPs"`
}

// SubnetIPReservationStatus defines the observed state of SubnetIPReservation
type SubnetIPReservationStatus struct {
	// Conditions described if the SubnetIPReservation is configured on NSX or not.
	// Condition type ""
	Conditions []Condition `json:"conditions,omitempty"`
	// List of reserved IPs.
	// Supported formats include: ["192.168.1.1", "192.168.1.3-192.168.1.100"]
	IPs []string `json:"ips,omitempty"`
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:selectablefield:JSONPath=.spec.subnet

// SubnetIPReservation is the Schema for the subnetipreservations API
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`,description="The parent Subnet name of the SubnetIPReservation."
// +kubebuilder:printcolumn:name="NumberOfIPs",type=string,JSONPath=`.spec.numberOfIPs`,description="Number of IPs requested to be reserved."
// +kubebuilder:printcolumn:name="IPs",type=string,JSONPath=`.status.ips[:]`,description="List of reserved IPs."
type SubnetIPReservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   SubnetIPReservationSpec   `json:"spec"`
	Status SubnetIPReservationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SubnetIPReservationList contains a list of SubnetIPReservation
type SubnetIPReservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubnetIPReservation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SubnetIPReservation{}, &SubnetIPReservationList{})
}
