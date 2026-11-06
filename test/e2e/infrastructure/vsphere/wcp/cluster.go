// Copyright (c) 2020-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

type ClusterContentLibrarySpec struct {
	ContentLibrary         string   `json:"content_library"`
	SupervisorServices     []string `json:"supervisor_services"`
	ResourceNamingStrategy string   `json:"resource_naming_strategy"`
}

// WCPClusterDetails contains detailed information about a cluster, including IP addresses, content library details, etc.
//
//	This is information obtained by getting a specific WCP enabled cluster.
type WCPClusterDetails struct {
	MoID                        string                      `json:"cluster"`
	KubernetesStatus            string                      `json:"kubernetes_status"`
	ConfigStatus                string                      `json:"config_status"`
	APIServerClusterEndpoint    string                      `json:"api_server_cluster_endpoint"`
	APIServerManagementEndpoint string                      `json:"api_server_management_endpoint"`
	DefaultContentLibraryID     string                      `json:"default_kubernetes_service_content_library"`
	ContentLibraries            []ClusterContentLibrarySpec `json:"content_libraries"`
	ClusterProxyConfig          ClusterProxyConfig          `json:"cluster_proxy_config"`
	NetworkProvider             string                      `json:"network_provider"`
}
