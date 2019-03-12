/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

type Client struct {
	client *govmomi.Client
}

// TODO(bryanv) Password should be separate from the url
func NewClient(ctx context.Context, url string) (*Client, error) {
	soapUrl, err := soap.ParseURL(url)
	if soapUrl == nil || err != nil {
		return nil, errors.Wrapf(err, "failed to parse soap url: %s", soapUrl)
	}

	client, err := govmomi.NewClient(ctx, soapUrl, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create govmomi client")
	}

	return &Client{
		client: client,
	}, nil
}

func (c *Client) VimClient() *vim25.Client {
	return c.client.Client
}

func (c *Client) Logout(ctx context.Context) {
	if err := c.client.Logout(ctx); err != nil {
		glog.Errorf("Error logging out url %q: %v", c.client.URL(), err)
	}
}
