// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

type Client struct {
	*client.Client
	config config.VSphereVMProviderConfig
}

// NewClient creates a new Client. As a side effect, it creates a vim25 client
// and a REST client.
func NewClient(
	ctx context.Context,
	config *config.VSphereVMProviderConfig) (*Client, error) {

	c, err := client.NewClient(ctx, client.Config{
		Host:       config.VcPNID,
		Port:       config.VcPort,
		Username:   config.VcCreds.Username,
		Password:   config.VcCreds.Password,
		CAFilePath: config.CAFilePath,
		Insecure:   config.InsecureSkipTLSVerify,
		Datacenter: config.Datacenter,
	})

	if err != nil {
		return nil, err
	}

	return &Client{
		Client: c,
		config: *config,
	}, nil
}

func (c *Client) Config() config.VSphereVMProviderConfig {
	return c.config
}
