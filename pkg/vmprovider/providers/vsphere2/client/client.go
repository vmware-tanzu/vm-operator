// Copyright (c) 2018-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
)

type Client struct {
	*client.Client
	contentLibClient contentlibrary.Provider
	clusterModClient clustermodules.Provider
	config           *config.VSphereVMProviderConfig
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
		Client:           c,
		contentLibClient: contentlibrary.NewProvider(ctx, c.RestClient()),
		clusterModClient: clustermodules.NewProvider(c.RestClient()),
		config:           config,
	}, nil
}

func (c *Client) ContentLibClient() contentlibrary.Provider {
	return c.contentLibClient
}

func (c *Client) ClusterModuleClient() clustermodules.Provider {
	return c.clusterModClient
}

func (c *Client) Config() *config.VSphereVMProviderConfig {
	return c.config
}
