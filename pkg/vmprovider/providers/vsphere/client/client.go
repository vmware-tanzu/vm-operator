// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
)

var log = logf.Log.WithName("vsphere").WithName("client")

type Client struct {
	vimClient        *vim25.Client
	finder           *find.Finder
	datacenter       *object.Datacenter
	restClient       *rest.Client
	contentLibClient contentlibrary.Provider
	clusterModClient clustermodules.Provider
	sessionManager   *session.Manager
	config           *config.VSphereVMProviderConfig
}

// Idle time before a keepalive will be invoked.
const keepAliveIdleTime = 5 * time.Minute

// SoapKeepAliveHandlerFn returns a keepalive handler function suitable for use with the SOAP handler.
// In case the connectivity to VC is down long enough, the session expires. Further attempts to use the
// client yield NotAuthenticated fault. This handler ensures that we re-login the client in those scenarios.
func SoapKeepAliveHandlerFn(sc *soap.Client, sm *session.Manager, userInfo *url.Userinfo) func() error {
	return func() error {
		ctx := context.Background()
		if _, err := methods.GetCurrentTime(ctx, sc); err != nil && isNotAuthenticatedError(err) {
			log.Info("Re-authenticating vim client")
			if err = sm.Login(ctx, userInfo); err != nil {
				if isInvalidLogin(err) {
					log.Error(err, "Invalid login in keepalive handler", "url", sc.URL())
					return err
				}
			}
		} else if err != nil {
			log.Error(err, "Error in vim25 client's keepalive handler", "url", sc.URL())
		}

		return nil
	}
}

// RestKeepAliveHandlerFn returns a keepalive handler function suitable for use with the REST handler.
// Similar to the SOAP handler, we customize the handler here so we can re-login the client in case the
// REST session expires due to connectivity issues.
func RestKeepAliveHandlerFn(c *rest.Client, userInfo *url.Userinfo) func() error {
	return func() error {
		ctx := context.Background()
		if sess, err := c.Session(ctx); err == nil && sess == nil {
			// session is Unauthorized.
			log.Info("Re-authenticating REST client")
			if err = c.Login(ctx, userInfo); err != nil {
				log.Error(err, "Invalid login in keepalive handler", "url", c.URL())
				return err
			}
		} else if err != nil {
			log.Error(err, "Error in rest client's keepalive handler", "url", c.URL())
		}

		return nil
	}
}

// newRestClient creates a rest client which is configured to use a custom keepalive handler function.
func newRestClient(ctx context.Context, vimClient *vim25.Client, config *config.VSphereVMProviderConfig) (*rest.Client, error) {
	log.Info("Creating new REST Client", "VcPNID", config.VcPNID, "VcPort", config.VcPort)
	restClient := rest.NewClient(vimClient)

	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)

	// Set a custom keepalive handler function
	restClient.Transport = keepalive.NewHandlerREST(restClient, keepAliveIdleTime, RestKeepAliveHandlerFn(restClient, userInfo))

	// Initial login. This will also start the keepalive.
	if err := restClient.Login(ctx, userInfo); err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, errors.Wrapf(err, "login failed for url: %v", vimClient.URL())
	}

	return restClient, nil
}

// NewVimClient creates a new vim25 client which is configured to use a custom keepalive handler function.
// Making this public to allow access from other packages when only VimClient is needed.
func NewVimClient(ctx context.Context, config *config.VSphereVMProviderConfig) (*vim25.Client, *session.Manager, error) {
	log.Info("Creating new vim Client", "VcPNID", config.VcPNID, "VcPort", config.VcPort)
	soapURL, err := soap.ParseURL(net.JoinHostPort(config.VcPNID, config.VcPort))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse %s:%s", config.VcPNID, config.VcPort)
	}

	soapClient := soap.NewClient(soapURL, config.InsecureSkipTLSVerify)
	if config.CAFilePath != "" {
		err = soapClient.SetRootCAs(config.CAFilePath)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to set root CA %s", config.CAFilePath)
		}
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating a new vim client for url: %v", soapURL)
	}

	if err := vimClient.UseServiceVersion(); err != nil {
		return nil, nil, errors.Wrapf(err, "error setting vim client version for url: %v", soapURL)
	}

	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)
	sm := session.NewManager(vimClient)

	// Set a custom keepalive handler function
	vimClient.RoundTripper = keepalive.NewHandlerSOAP(soapClient, keepAliveIdleTime, SoapKeepAliveHandlerFn(soapClient, sm, userInfo))

	// Initial login. This will also start the keepalive.
	if err = sm.Login(ctx, userInfo); err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, nil, errors.Wrapf(err, "login failed for url: %v", soapURL)
	}

	return vimClient, sm, err
}

func newFinder(
	ctx context.Context,
	vimClient *vim25.Client,
	config *config.VSphereVMProviderConfig) (*find.Finder, *object.Datacenter, error) {

	finder := find.NewFinder(vimClient, false)

	dcRef, err := finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "Datacenter", Value: config.Datacenter})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to find Datacenter %q", config.Datacenter)
	}

	dc := dcRef.(*object.Datacenter)
	finder.SetDatacenter(dc)

	return finder, dc, nil
}

// NewClient creates a new Client. As a side effect, it creates a vim25 client and a REST client.
func NewClient(ctx context.Context, config *config.VSphereVMProviderConfig) (*Client, error) {
	vimClient, sm, err := NewVimClient(ctx, config)
	if err != nil {
		return nil, err
	}

	finder, datacenter, err := newFinder(ctx, vimClient, config)
	if err != nil {
		return nil, err
	}

	restClient, err := newRestClient(ctx, vimClient, config)
	if err != nil {
		return nil, err
	}

	return &Client{
		vimClient:        vimClient,
		finder:           finder,
		datacenter:       datacenter,
		restClient:       restClient,
		contentLibClient: contentlibrary.NewProvider(restClient),
		clusterModClient: clustermodules.NewProvider(restClient),
		sessionManager:   sm,
		config:           config,
	}, nil
}

func isNotAuthenticatedError(err error) bool {
	if soap.IsSoapFault(err) {
		vimFault := soap.ToSoapFault(err).VimFault()
		if _, ok := vimFault.(types.NotAuthenticated); ok {
			return true
		}
	}

	return false
}

func isInvalidLogin(err error) bool {
	if soap.IsSoapFault(err) {
		vimFault := soap.ToSoapFault(err).VimFault()
		if _, ok := vimFault.(types.InvalidLogin); ok {
			return true
		}
	}

	return false
}

func (c *Client) VimClient() *vim25.Client {
	return c.vimClient
}

func (c *Client) Finder() *find.Finder {
	return c.finder
}

func (c *Client) Datacenter() *object.Datacenter {
	return c.datacenter
}

func (c *Client) RestClient() *rest.Client {
	return c.restClient
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

func (c *Client) Logout(ctx context.Context) {
	clientURL := c.vimClient.URL()
	log.Info("vsphere client logging out from", "VC", clientURL.Host)
	if err := c.sessionManager.Logout(ctx); err != nil {
		log.Error(err, "Error logging out the vim25 session", "username", clientURL.User.Username(), "host", clientURL.Host)
	}

	if err := c.restClient.Logout(ctx); err != nil {
		log.Error(err, "Error logging out the rest session", "username", clientURL.User.Username(), "host", clientURL.Host)
	}
}
