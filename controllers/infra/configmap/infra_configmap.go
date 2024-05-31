// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	// WcpClusterConfigMapNamespace is the namespace of the wcp-cluster-config
	// ConfigMap.
	WcpClusterConfigMapNamespace = "kube-system"

	// WcpClusterConfigMapName is the name of the wcp-cluster-config ConfigMap.
	WcpClusterConfigMapName = "wcp-cluster-config"

	// WcpClusterConfigFileName is the name of the key specified in the wcp-cluster-config ConfigMap containing the cluster config.
	WcpClusterConfigFileName = "wcp-cluster-config.yaml"
)

type WcpClusterConfig struct {
	VcPNID string `json:"vc_pnid"`
	VcPort string `json:"vc_port"`
}

// ParseWcpClusterConfig builds and returns the cluster config.
func ParseWcpClusterConfig(wcpClusterCfgData map[string]string) (*WcpClusterConfig, error) {
	data, ok := wcpClusterCfgData[WcpClusterConfigFileName]
	if !ok {
		return nil, fmt.Errorf("required key %s not found", WcpClusterConfigFileName)
	}

	wcpClusterConfig := &WcpClusterConfig{}
	if err := yaml.Unmarshal([]byte(data), wcpClusterConfig); err != nil {
		return nil, err
	}

	return wcpClusterConfig, nil
}

func NewWcpClusterConfigMap(wcpClusterConfig WcpClusterConfig) (*corev1.ConfigMap, error) {
	bytes, err := yaml.Marshal(wcpClusterConfig)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WcpClusterConfigMapName,
			Namespace: WcpClusterConfigMapNamespace,
		},
		Data: map[string]string{
			WcpClusterConfigFileName: string(bytes),
		},
	}, nil
}
