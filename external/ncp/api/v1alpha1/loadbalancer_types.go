/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancer defines the configuration for loadBalancer service
// +k8s:openapi-gen=true
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   LoadBalancerSpec   `json:"spec,omitempty"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// LoadBalancerSize defines load balancer size supported
type LoadBalancerSize string

const (
	// SizeSmall defines the size as SMALL
	SizeSmall LoadBalancerSize = "SMALL"
	// SizeMedium defines the size as MEDIUM
	SizeMedium LoadBalancerSize = "MEDIUM"
	// SizeLarge defines the size as LARGE
	SizeLarge LoadBalancerSize = "LARGE"
)

// LoadBalancerSpec defines the desired state of loadBalancer
type LoadBalancerSpec struct {
	HTTPConfig         *LoadBalancerHTTPConfig `json:"httpConfig,omitempty"`
	Size               LoadBalancerSize        `json:"size,omitempty"`
	VirtualNetworkName string                  `json:"virtualNetworkName,omitempty"`
}

// LoadBalancerXForwardedFor describes the xForwardedFor option for LoadBalancerHTTPConfig
type LoadBalancerXForwardedFor string

const (
	// LoadBalancerXForwardedForInsert means xForwardedFor is INSERT
	LoadBalancerXForwardedForInsert LoadBalancerXForwardedFor = "INSERT"
	// LoadBalancerXForwardedForReplace means xForwardedFor is REPLACE
	LoadBalancerXForwardedForReplace LoadBalancerXForwardedFor = "REPLACE"
)

// LoadBalancerHTTPConfig defines the http config for the LoadBalancer
type LoadBalancerHTTPConfig struct {
	VirtualIP     string                    `json:"virtualIP,omitempty"`
	Port          int32                     `json:"port,omitempty"`
	TLS           *LoadBalancerTLS          `json:"tls,omitempty"`
	XForwardedFor LoadBalancerXForwardedFor `json:"xForwardedFor,omitempty"`
	Affinity      *LoadBalancerAffinity     `json:"affinity,omitempty"`
}

// LoadBalancerTLS defines the TLS configuration of LoadBalancer
type LoadBalancerTLS struct {
	Port       int32  `json:"port,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}

// LoadBalancerAffinity defines the affinity of LoadBalancer
type LoadBalancerAffinity struct {
	Type    string `json:"type,omitempty"`
	Timeout int32  `json:"timeout,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	HTTPVirtualIP string `json:"httpVirtualIP,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadBalancerList is a list of LoadBalancer
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}
