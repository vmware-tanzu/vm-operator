/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"time"

	"github.com/golang/glog"
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

// NewClient creates a new govmomi client; sets a keepalive handler to re-login on not authenticated errors.
// TODO(bryanv) Password should be separate from the url
func NewClient(ctx context.Context, url string) (*Client, error) {
	soapUrl, err := soap.ParseURL(url)
	if soapUrl == nil || err != nil {
		return nil, errors.Wrapf(err, "failed to parse soap url: %s", soapUrl)
	}

	// Decompose govmomi.NewClient so we can create a client with a custom keepalive handler which
	// logs in on NotAuthenticated errors.
	soapClient := soap.NewClient(soapUrl, true)
	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, errors.Wrapf(err, "erorr creating a new vim client for soap url: %s", soapUrl)
	}

	vcClient := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	const idleTime time.Duration = 5 * time.Minute
	k := session.KeepAliveHandler(vimClient.RoundTripper, idleTime, func(rt soap.RoundTripper) error {
		_, err := methods.GetCurrentTime(ctx, rt)

		if err != nil {
			if isNotAuthenticatedError(err) {
				if err = vcClient.Login(ctx, soapUrl.User); err != nil {
					if isInvalidLogin(err) {
						glog.Error("error in session re-authentication. Quitting Keep alive handler.")
						return err
					}
				} else {
					glog.Info("successfully reauthenticated the session")
				}
			}
		}
		return nil
	})
	vimClient.RoundTripper = k

	if err = vcClient.Login(ctx, soapUrl.User); err != nil {
		return nil, errors.Wrapf(err, "error logging in the new client using soapUrl: %s", soapUrl)
	}

	return &Client{
		client: vcClient,
	}, nil
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

func isNotAuthenticatedError(err error) bool {
	if soap.IsSoapFault(err) {
		switch soap.ToSoapFault(err).VimFault().(type) {
		case types.NotAuthenticated:
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
		glog.Errorf("Error logging out url %q: %v", c.client.URL(), err)
	}
}
