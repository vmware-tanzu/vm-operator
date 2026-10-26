// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigMap struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// goscCloudConfig is the cloud-init user-data used by CloudConfigMap and
// OvfEnvConfigMap, the latter of which sends the same content base64-encoded
// under an "OvfEnv" key naming convention.
const goscCloudConfig = `#cloud-config
ssh_pwauth: true
users:
  - name: vmware
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: false
    # Password set to Admin!23
    passwd: '$1$salt$SOC33fVbA/ZxeIwD5yw1u1'
    shell: /bin/bash
write_files:
  - content: |
      VMSVC Says Hello World
    path: /helloworld
`

// CloudConfigMap returns a ConfigMap containing plaintext cloud-init user-data.
func CloudConfigMap(cm ConfigMap) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta:   typeMeta("v1", "ConfigMap"),
		ObjectMeta: configMapObjectMeta(cm),
		Data: map[string]string{
			"user-data": goscCloudConfig,
		},
	}
}

// OvfEnvConfigMap returns a ConfigMap containing base64-encoded cloud-init user-data for OVF delivery.
func OvfEnvConfigMap(cm ConfigMap) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta:   typeMeta("v1", "ConfigMap"),
		ObjectMeta: configMapObjectMeta(cm),
		Data: map[string]string{
			"user-data": base64.StdEncoding.EncodeToString([]byte(goscCloudConfig)),
		},
	}
}

// VAppConfigMap returns a ConfigMap containing vApp property template expressions.
func VAppConfigMap(cm ConfigMap) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta:   typeMeta("v1", "ConfigMap"),
		ObjectMeta: configMapObjectMeta(cm),
		Data: map[string]string{
			"nameservers":        `{{ (index .V1alpha1.Net.Nameservers 0) }}`,
			"hostname":           `{{ .V1alpha1.VM.Name }}`,
			"management_ip":      `{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}`,
			"management_gateway": `{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}`,
		},
	}
}

func configMapObjectMeta(cm ConfigMap) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      cm.Name,
		Namespace: cm.Namespace,
	}
}
