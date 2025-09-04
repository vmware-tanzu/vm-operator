// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

type Config struct {
	Host       string
	Port       string
	Username   string
	Password   string
	CAFilePath string
	Insecure   bool
	Datacenter string
}

type Client struct {
	vimClient      *vim25.Client
	restClient     *rest.Client
	pbmClient      *pbm.Client
	sessionManager *session.Manager
	config         Config

	finder     *find.Finder
	datacenter *object.Datacenter
}

// NewClient returns a new client.
func NewClient(ctx context.Context, config Config) (*Client, error) {
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

	pbmClient, err := pbm.NewClient(ctx, vimClient)
	if err != nil {
		return nil, err
	}

	return &Client{
		vimClient:      vimClient,
		finder:         finder,
		datacenter:     datacenter,
		restClient:     restClient,
		pbmClient:      pbmClient,
		sessionManager: sm,
		config:         config,
	}, nil
}

// Idle time before a keepalive will be invoked.
const keepAliveIdleTime = 5 * time.Minute

// SoapKeepAliveHandlerFn returns a keepalive handler function suitable for use
// with the SOAP handler. In case the connectivity to VC is down long enough,
// the session expires. Further attempts to use the client yield
// NotAuthenticated fault. This handler ensures that we re-login the client in
// those scenarios.
func SoapKeepAliveHandlerFn(
	ctx context.Context,
	sc *soap.Client,
	sm *session.Manager,
	userInfo *url.Userinfo) func() error {

	log := pkglog.FromContextOrDefault(ctx).WithName("SoapKeepAliveHandlerFn")

	return func() error {
		ctx := context.Background()
		if _, err := methods.GetCurrentTime(ctx, sc); err != nil && IsNotAuthenticatedError(err) {
			log.Info("Re-authenticating vim client")
			if err = sm.Login(ctx, userInfo); err != nil {
				if IsInvalidLogin(err) {
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

// RestKeepAliveHandlerFn returns a keepalive handler function suitable for use
// with the REST handler. Similar to the SOAP handler, we customize the handler
// here so we can re-login the client in case the REST session expires due to
// connectivity issues.
func RestKeepAliveHandlerFn(
	ctx context.Context,
	c *rest.Client,
	userInfo *url.Userinfo) func() error {

	log := pkglog.FromContextOrDefault(ctx).WithName("RestKeepAliveHandlerFn")

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
func newRestClient(
	ctx context.Context,
	vimClient *vim25.Client,
	config Config) (*rest.Client, error) {

	log := pkglog.FromContextOrDefault(ctx).WithName("newRestClient")

	log.Info("Creating new REST Client", "VcPNID", config.Host, "VcPort", config.Port)
	restClient := rest.NewClient(vimClient)

	userInfo := url.UserPassword(config.Username, config.Password)

	// Set a custom keepalive handler function
	restClient.Transport = keepalive.NewHandlerREST(
		restClient,
		keepAliveIdleTime,
		RestKeepAliveHandlerFn(ctx, restClient, userInfo))

	// Initial login. This will also start the keepalive.
	if err := restClient.Login(ctx, userInfo); err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, fmt.Errorf("login failed for url: %v: %w", vimClient.URL(), err)
	}

	return restClient, nil
}

// NewVimClient creates a new vim25 client which is configured to use a custom
// keepalive handler function.
func NewVimClient(
	ctx context.Context,
	config Config) (*vim25.Client, *session.Manager, error) {

	log := pkglog.FromContextOrDefault(ctx).WithName("NewVimClient")

	log.Info("Creating new vim Client", "VcPNID", config.Host, "VcPort", config.Port)
	soapURL, err := soap.ParseURL(net.JoinHostPort(config.Host, config.Port))
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to parse %s:%s: %w", config.Host, config.Port, err)
	}

	soapClient := soap.NewClient(soapURL, config.Insecure)
	if config.CAFilePath != "" {
		err = soapClient.SetRootCAs(config.CAFilePath)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to set root CA %s: %w", config.CAFilePath, err)
		}
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"error creating a new vim client for url: %v: %w", soapURL, err)
	}

	if err := vimClient.UseServiceVersion(); err != nil {
		return nil, nil, fmt.Errorf(
			"error setting vim client version for url: %v: %w", soapURL, err)
	}

	userInfo := url.UserPassword(config.Username, config.Password)
	sm := session.NewManager(vimClient)

	// Set a custom keepalive handler function
	vimClient.RoundTripper = keepalive.NewHandlerSOAP(
		soapClient,
		keepAliveIdleTime,
		SoapKeepAliveHandlerFn(ctx, soapClient, sm, userInfo))

	// Initial login. This will also start the keepalive.
	if err = sm.Login(ctx, userInfo); err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, nil, fmt.Errorf(
			"login failed for url: %v: %w", soapURL, err)
	}

	return vimClient, sm, err
}

func newFinder(
	ctx context.Context,
	vimClient *vim25.Client,
	config Config) (*find.Finder, *object.Datacenter, error) {

	finder := find.NewFinder(vimClient, false)

	dcRef, err := finder.ObjectReference(
		ctx,
		vimtypes.ManagedObjectReference{
			Type:  "Datacenter",
			Value: config.Datacenter,
		})
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to find Datacenter %q: %w", config.Datacenter, err)
	}

	dc := dcRef.(*object.Datacenter)
	finder.SetDatacenter(dc)

	return finder, dc, nil
}

func IsNotAuthenticatedError(err error) bool {
	return fault.Is(err, &vimtypes.NotAuthenticated{})
}

func IsInvalidLogin(err error) bool {
	return fault.Is(err, &vimtypes.InvalidLogin{})
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

func (c *Client) PbmClient() *pbm.Client {
	return c.pbmClient
}

func (c *Client) RestClient() *rest.Client {
	return c.restClient
}

func (c *Client) Config() Config {
	return c.config
}

func (c *Client) Valid() bool {
	if c == nil || c.vimClient == nil {
		return false
	}
	return c.VimClient().Valid()
}

func (c *Client) Logout(ctx context.Context) {
	log := pkglog.FromContextOrDefault(ctx).WithName("Logout")

	clientURL := c.vimClient.URL()
	log.Info("vsphere client logging out from", "VC", clientURL.Host)

	if err := c.sessionManager.Logout(ctx); err != nil {
		log.Error(err, "Error logging out the vim25 session",
			"username", clientURL.User.Username(),
			"host", clientURL.Host)
	}

	if err := c.restClient.Logout(ctx); err != nil {
		log.Error(err, "Error logging out the rest session",
			"username", clientURL.User.Username(),
			"host", clientURL.Host)
	}
}
