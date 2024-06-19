// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability

import (
	"errors"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// WcpClusterCapabilitiesConfigMapName is the name of the wcp-cluster-capabilities ConfigMap.
	WcpClusterCapabilitiesConfigMapName = "wcp-cluster-capabilities"

	// WcpClusterCapabilitiesNamespace is the namespace of the wcp-cluster-capabilities
	// ConfigMap.
	WcpClusterCapabilitiesNamespace = "kube-system"

	// TKGMultipleCLCapabilityKey is the name of capability key defined in wcp-cluster-capabilities ConfigMap.
	TKGMultipleCLCapabilityKey = "MultipleCL_For_TKG_Supported"
)

func NewWcpClusterCapabilitiesConfigMap(wcpClusterConfig map[string]string) (*corev1.ConfigMap, error) {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WcpClusterCapabilitiesConfigMapName,
			Namespace: WcpClusterCapabilitiesNamespace,
		},
		Data: wcpClusterConfig,
	}, nil
}

// IsTKGMultipleCLSupported returns if TKG_Multiple_CL is enabled or not in the
// wcp-cluster-capabilities ConfigMap.
func IsTKGMultipleCLSupported(wcpClusterCapabilitiesData map[string]string) (bool, error) {
	multipleCLValue, isPresent := wcpClusterCapabilitiesData[TKGMultipleCLCapabilityKey]
	if !isPresent {
		return false, errors.New("required key is not present in the configmap")
	}
	isMultipleCLSupported, err := strconv.ParseBool(multipleCLValue)
	if err != nil {
		return false, err
	}
	return isMultipleCLSupported, nil
}
