// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/antchfx/htmlquery"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/session/cache"
	vapirest "github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
)

const (
	h5LoginURLFormat  = "https://%s/ui/login"
	ssoLoginURLFormat = "https://%s/ui/saml/websso/sso"
	vcsimURLFormat    = "https://user:pass@%s:8989"

	castleAuthorizationKey    = "CastleAuthorization"
	castleAuthorizationFormat = "Basic %s"
	samlResponseKey           = "SAMLResponse"
	relayStateKey             = "RelayState"
)

// NewVimClientFromKubeConfig uses a kubeconfig pointed at a supervisor cluster context with an
//
//	administrator user to determine the vCenter instance and setup an authenticated vim client.
func NewVimClientFromKubeconfig(ctx context.Context, path string) *vim25.Client {
	return NewVimClientFromKubeconfigFile(ctx, path)
}

func NewVimClientFromKubeconfigFile(ctx context.Context, path string) *vim25.Client {
	vCenterHostname := GetVCPNIDFromKubeconfigFile(ctx, path)
	Expect(vCenterHostname).NotTo(Equal(""), "Unable to determine VC PNID")

	// TODO (GCM-3023) Figure out how to get these from E2E test config.
	//  Hardcoding is a short term fix until then.
	vCenterAdminUser := testbed.AdminUsername
	vCenterAdminPassword := testbed.AdminPassword

	vimClient, err := NewVimClient(vCenterHostname, vCenterAdminUser, vCenterAdminPassword)
	Expect(err).NotTo(HaveOccurred())

	return vimClient
}

// Constructs a new vim client authenticated with the given user credentials.
func NewVimClient(vCenterHost string, username string, password string) (*vim25.Client, error) {
	clientURL := url.URL{
		Scheme: "https",
		Host:   vCenterHost,
		Path:   "sdk",
	}
	ctx := context.Background()
	sc := soap.NewClient(&clientURL, true)

	client, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, err
	}

	loginRequest := types.Login{
		This:     *client.ServiceContent.SessionManager,
		UserName: username,
		Password: password,
	}

	_, err = methods.Login(ctx, sc, &loginRequest)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// LogoutVimClient sends a logout request using the given client.
func LogoutVimClient(client *vim25.Client) {
	if client == nil || client.RoundTripper == nil {
		return
	}

	logoutRequest := types.Logout{
		This: *client.ServiceContent.SessionManager,
	}
	_, err := methods.Logout(context.Background(), client.RoundTripper, &logoutRequest)
	Expect(err).NotTo(HaveOccurred())
}

// GetH5SessionCookie gets the session cookie by logging into the H5 client with given user credentials.
// For further H5 requests, attach this session cookie in order to authenticate.
func GetH5SessionCookie(client *http.Client, hostname, username, password string) *http.Cookie {
	// 1. Starts the login process in the H5 Client. Returns a correctly formed redirection link to the WebSSO
	// that contains the required URL parameters such as SAMLRequest, signature and signature algorithm.
	redirectToSso := getWebSsoLocation(client, hostname)

	// 2. Login in the Web SSO with the supplied username and password. Returns a SAMLResponse that packs the
	// SAML token issued from the SSO and also the RelayState.
	// Note: The step of retrieving the HTML that is needed for rendering the Login Page is skipped.
	samlResponse, relayState := loginToWebSso(client, redirectToSso, username, password)

	// 3. Finish the login process by passing the returned SAMLResponse and RelayState back to the H5 Client.
	cookie := loginByTokenToH5(client, hostname, relayState, samlResponse)

	return cookie
}

func getWebSsoLocation(client *http.Client, hostname string) string {
	resp, err := client.Get(fmt.Sprintf(h5LoginURLFormat, hostname))
	Expect(err).NotTo(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	return resp.Request.URL.String()
}

func loginToWebSso(client *http.Client, location, username, password string) (string, string) {
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	resp, err := client.PostForm(location, url.Values{castleAuthorizationKey: {fmt.Sprintf(castleAuthorizationFormat, auth)}})
	Expect(err).NotTo(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	doc, err := htmlquery.Parse(strings.NewReader(string(body)))
	Expect(err).NotTo(HaveOccurred())

	samlResponseNode := htmlquery.FindOne(doc, "/html/body/form/input[@name=\"SAMLResponse\"]")
	relayStateNode := htmlquery.FindOne(doc, "/html/body/form/input[@name=\"RelayState\"]")

	return htmlquery.SelectAttr(samlResponseNode, "value"), htmlquery.SelectAttr(relayStateNode, "value")
}

func loginByTokenToH5(client *http.Client, hostname, relayState, samlResponse string) *http.Cookie {
	resp, err := client.PostForm(fmt.Sprintf(ssoLoginURLFormat, hostname), url.Values{samlResponseKey: {samlResponse}, relayStateKey: {relayState}})
	Expect(err).NotTo(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	cookie := strings.Split(resp.Request.Response.Header.Get("Set-Cookie"), ";")

	return &http.Cookie{
		Name:  strings.Split(cookie[0], "=")[0],
		Value: strings.Split(cookie[0], "=")[1],
		Path:  strings.Split(cookie[1], "=")[1],
	}
}

// NewVCSimClient creates a vcsim vim25.Client for using in the kind environment.
func NewVcSimClient(ctx context.Context, vcip string) *vim25.Client {
	u, err := soap.ParseURL(fmt.Sprintf(vcsimURLFormat, vcip))
	Expect(err).NotTo(HaveOccurred(), "Should be able to parse the url.")

	s := &cache.Session{
		URL:      u,
		Insecure: true,
	}

	c := new(vim25.Client)
	err = s.Login(ctx, c, nil)
	Expect(err).NotTo(HaveOccurred(), "Should be able to login to the vcsim")

	return c
}

// Constructs a new rest client authenticated with the given user credentials.
func NewRestClient(ctx context.Context, c *vim25.Client, userName string, password string) (*vapirest.Client, error) {
	restClient := vapirest.NewClient(c)
	userInfo := url.UserPassword(userName, password)
	err := restClient.Login(ctx, userInfo)

	return restClient, err
}
