// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"encoding/base64"
	"strings"
	"text/template"

	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

const XdsNodePort = 31799

var envoyBootstrapConfigTemplate, _ = template.New("envoyBootstrapConfig").Parse(envoyBootstrapConfig)

type lbConfigParams struct {
	NodeID      string
	Ports       []vmopv1.VirtualMachineServicePort
	CPNodes     []string
	XdsNodePort int
}

const envoyBootstrapConfig = `node:
  id: {{.NodeID}}
  cluster: vmop-simple-lb
static_resources:
  listeners:
  # {{- range .Ports}}
  - name: {{.Name}}
    address:
      socket_address:
        address: 0.0.0.0
        port_value: {{.Port}}
    filter_chains:
    - filters:
      - name: envoy.tcp_proxy
        config:
          stat_prefix: ingress_tcp
          cluster: {{.Name}}
  # {{- end}}

  clusters:
  # {{- range .Ports}}
  - name: {{.Name}}
    connect_timeout: 0.25s
    type: EDS
    lb_policy: ROUND_ROBIN
    eds_cluster_config:
      eds_config:
        api_config_source:
          api_type: GRPC
          grpc_services:
            envoy_grpc:
              cluster_name: xds_cluster
  # {{- end}}

  - name: xds_cluster
    connect_timeout: 0.25s
    type: STATIC
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
    upstream_connection_options:
      tcp_keepalive: {}
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
      - lb_endpoints:
        # {{- $xdsNodePort := .XdsNodePort}}
        # {{- range .CPNodes}}
        - endpoint:
            address:
              socket_address:
                address: {{.}}
                port_value: {{$xdsNodePort}}
        # {{- end}}

admin:
  access_log_path: "/dev/null"
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901
`

const cloudConfigPrefix = `#cloud-config
`

type cloudConfig struct {
	WriteFiles []writeFile `json:"write_files,omitempty"`
}

type writeFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func renderAndBase64EncodeLBCloudConfig(params lbConfigParams) string {
	envoyConfigStringBuilder := &strings.Builder{}
	_ = envoyBootstrapConfigTemplate.Execute(envoyConfigStringBuilder, params)

	cc := &cloudConfig{
		WriteFiles: []writeFile{{
			Path:    "/etc/envoy/envoy.yaml",
			Content: envoyConfigStringBuilder.String(),
		}},
	}
	ccYamlBytes, _ := yaml.Marshal(cc)

	b64StringBuilder := &strings.Builder{}
	b64enc := base64.NewEncoder(base64.StdEncoding, b64StringBuilder)

	_, _ = b64enc.Write([]byte(cloudConfigPrefix))
	_, _ = b64enc.Write(ccYamlBytes)
	_ = b64enc.Close()

	return b64StringBuilder.String()
}
