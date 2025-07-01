// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package netplan

import (
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan/schema"
)

// Config describes the desired network configuration from
// https://netplan.readthedocs.io/en/stable/netplan-yaml/.
type Config = schema.Config

type Network = schema.NetworkConfig

type Ethernet = schema.EthernetConfig

type Match = schema.MatchConfig

type Nameserver = schema.NameserverConfig

type Route = schema.RoutingConfig

type Address = schema.AddressMapping

type Renderer = schema.Renderer

func MarshalYAML(in Config) ([]byte, error) {
	return yaml.Marshal(in)
}

func UnmarshalYAML(data []byte) (Config, error) {
	var out Config
	err := yaml.Unmarshal(data, &out)
	return out, err
}
