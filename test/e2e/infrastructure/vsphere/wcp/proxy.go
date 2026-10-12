// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

type ClusterProxySource string

const (
	VcInherited       ClusterProxySource = "VC_INHERITED"
	ClusterConfigured ClusterProxySource = "CLUSTER_CONFIGURED"
	None              ClusterProxySource = "NONE"
)

// ClusterProxyConfig contains the proxy configuration for a cluster. The
// JSON values are retrieved from running dcli commands with the +formatter=json
// option.
type ClusterProxyConfig struct {
	TLSRootCABundle     string             `json:"tls_root_ca_bundle"`
	HTTPProxyConfig     string             `json:"http_proxy_config"`
	HTTPSProxyConfig    string             `json:"https_proxy_config"`
	ProxySettingsSource ClusterProxySource `json:"proxy_settings_source"`
	NoProxyConfig       []string           `json:"no_proxy_config"`
}
