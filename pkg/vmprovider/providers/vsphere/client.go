/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"net"
	"time"

	"net/url"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

type Client struct {
	client *govmomi.Client
}

const idleTime = 5 * time.Minute

// NewClient creates a new govmomi client; sets a keepalive handler to re-login on not authenticated errors.
func NewClient(ctx context.Context, config *VSphereVmProviderConfig) (*Client, error) {
	soapURL, err := soap.ParseURL(net.JoinHostPort(config.VcPNID, config.VcPort))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s:%s", config.VcPNID, config.VcPort)
	}

	// Decompose govmomi.NewClient so we can create a client with a custom keepalive handler which
	//  logs in on NotAuthenticated errors.
	soapClient := soap.NewClient(soapURL, config.InsecureSkipTLSVerify)
	if !config.InsecureSkipTLSVerify {
		err = soapClient.SetRootCAs(config.CAFilePath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set root CA %s: %v", config.CAFilePath, err)
		}
	}
	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating a new vim client for url: %v", soapURL)
	}

	vcClient := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)

	vimClient.RoundTripper = session.KeepAliveHandler(vimClient.RoundTripper, idleTime, func(rt soap.RoundTripper) error {
		ctx := context.Background()
		if _, err := methods.GetCurrentTime(ctx, rt); err != nil && isNotAuthenticatedError(err) {
			if err = vcClient.Login(ctx, userInfo); err != nil {
				if isInvalidLogin(err) {
					log.Error(err, "Invalid login in keep alive handler", "url", soapURL)
					return err
				}
			}
		}
		return nil
	})

	if err = vcClient.Login(ctx, userInfo); err != nil {
		return nil, errors.Wrapf(err, "login failed for url: %v", soapURL)
	}

	return &Client{
		client: vcClient,
	}, nil
}

func isNotAuthenticatedError(err error) bool {
	if soap.IsSoapFault(err) {
		switch soap.ToSoapFault(err).VimFault().(type) {
		case types.NotAuthenticated:
			return true
		}
	}
	return false
}

func isInvalidLogin(err error) bool {
	if soap.IsSoapFault(err) {
		switch soap.ToSoapFault(err).VimFault().(type) {
		case types.InvalidLogin:
			return true
		}
	}
	return false
}

func (c *Client) VimClient() *vim25.Client {
	return c.client.Client
}

func (c *Client) Logout(ctx context.Context) {
	if err := c.client.Logout(ctx); err != nil {
		clientURL := c.client.URL()
		log.Error(err, "Error logging out url", "username", clientURL.User.Username(), "host", clientURL.Host)
	}
}
