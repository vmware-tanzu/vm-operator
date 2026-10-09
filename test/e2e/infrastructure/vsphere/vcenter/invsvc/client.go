// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package invsvc

import (
	"context"
	"net/url"
	"strings"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// The Global Permissions API is still internal-only. Some MOB based workarounds exist, see:
// https://williamlam.com/2017/03/automating-vsphere-global-permissions-with-powercli.html
// https://github.com/vmware/govmomi/pull/2485
// This client uses the inventory service API directly, rather than via the MOB.

const (
	Namespace = "inventoryservice"
	Path      = "/invsvc/vmomi/sdk"
	Version   = "4.0"
)

var (
	AuthorizationService = vimtypes.ManagedObjectReference{
		Type:  "AuthorizationService",
		Value: "authorizationService",
	}

	SessionManager = vimtypes.ManagedObjectReference{
		Type:  "InventoryServiceSessionManager",
		Value: "sessionManager",
	}
)

type Principal struct {
	Name  string `xml:"name"`
	Group bool   `xml:"group"`
}

type AccessControl struct {
	Principal Principal `xml:"principal"`
	Roles     []int64   `xml:"roles,omitempty"`
	Propagate bool      `xml:"propagate"`
	Version   int64     `xml:"version"`
}

type Client struct {
	*soap.Client
}

func NewClient(ctx context.Context, c *vim25.Client) *Client {
	// TODO: should use soap.NewServiceClient, but vcSessionCookie SOAP header causes InventoryServiceLogin() to fail
	u := c.URL()
	u.Path = Path
	sc := soap.NewClient(u, c.DefaultTransport().TLSClientConfig.InsecureSkipVerify)
	sc.Namespace = "urn:" + Namespace
	sc.Version = Version

	return &Client{sc}
}

func (c *Client) Login(ctx context.Context, u *url.Userinfo) error {
	var reqBody, resBody InventoryServiceLoginBody

	p, _ := u.Password()

	reqBody.Req = &InventoryServiceLoginRequest{
		This:     SessionManager,
		Username: u.Username(),
		Password: p,
	}

	return c.RoundTrip(ctx, &reqBody, &resBody)
}

func (c *Client) Logout(ctx context.Context) error {
	var reqBody, resBody InventoryServiceLogoutBody

	reqBody.Req = &InventoryServiceLogoutRequest{
		This: SessionManager,
	}

	return c.RoundTrip(ctx, &reqBody, &resBody)
}

func (c *Client) GetGlobalAccessControlList(ctx context.Context) ([]AccessControl, error) {
	var reqBody, resBody GetGlobalAccessControlListBody

	reqBody.Req = &GetGlobalAccessControlListRequest{
		This: AuthorizationService,
	}

	err := c.RoundTrip(ctx, &reqBody, &resBody)
	if err != nil {
		return nil, err
	}

	return resBody.Res.Returnval, nil
}

func (p *Principal) format() Principal {
	name := strings.SplitN(p.Name, "@", 2)
	if len(name) == 2 {
		// Convert from UPN to domain/username format
		p.Name = name[1] + `\` + name[0]
	}

	return *p
}

func (c *Client) AddGlobalAccessControlList(ctx context.Context, permissions ...AccessControl) error {
	for i, p := range permissions {
		permissions[i].Principal = p.Principal.format()
	}

	var reqBody, resBody AddGlobalAccessControlListBody

	reqBody.Req = &AddGlobalAccessControlListRequest{
		This:        AuthorizationService,
		Permissions: permissions,
	}

	return c.RoundTrip(ctx, &reqBody, &resBody)
}

func (c *Client) RemoveGlobalAccess(ctx context.Context, principals ...Principal) error {
	for i, p := range principals {
		principals[i] = p.format()
	}

	var reqBody, resBody RemoveGlobalAccessBody

	reqBody.Req = &RemoveGlobalAccessRequest{
		This:       AuthorizationService,
		Principals: principals,
	}

	return c.RoundTrip(ctx, &reqBody, &resBody)
}

type InventoryServiceLoginRequest struct {
	This     vimtypes.ManagedObjectReference `xml:"_this"`
	Username string                          `xml:"username"`
	Password string                          `xml:"password"`
}

type InventoryServiceLoginResponse struct {
}

type InventoryServiceLoginBody struct {
	Req    *InventoryServiceLoginRequest  `xml:"urn:inventoryservice InventoryServiceLogin,omitempty"`
	Res    *InventoryServiceLoginResponse `xml:"urn:inventoryservice InventoryServiceLoginResponse,omitempty"`
	Fault_ *soap.Fault                    `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *InventoryServiceLoginBody) Fault() *soap.Fault { return b.Fault_ }

type InventoryServiceLogoutRequest struct {
	This vimtypes.ManagedObjectReference `xml:"_this"`
}

type InventoryServiceLogoutResponse struct {
}

type InventoryServiceLogoutBody struct {
	Req    *InventoryServiceLogoutRequest  `xml:"urn:inventoryservice InventoryServiceLogout,omitempty"`
	Res    *InventoryServiceLogoutResponse `xml:"urn:inventoryservice InventoryServiceLogoutResponse,omitempty"`
	Fault_ *soap.Fault                     `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *InventoryServiceLogoutBody) Fault() *soap.Fault { return b.Fault_ }

type GetGlobalAccessControlListRequest struct {
	This vimtypes.ManagedObjectReference `xml:"_this"`
}

type GetGlobalAccessControlListResponse struct {
	Returnval []AccessControl `xml:"returnval"`
}

type GetGlobalAccessControlListBody struct {
	Req    *GetGlobalAccessControlListRequest  `xml:"urn:inventoryservice AuthorizationService.GetGlobalAccessControlList,omitempty"`
	Res    *GetGlobalAccessControlListResponse `xml:"urn:inventoryservice AuthorizationService.GetGlobalAccessControlListResponse,omitempty"`
	Fault_ *soap.Fault                         `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *GetGlobalAccessControlListBody) Fault() *soap.Fault { return b.Fault_ }

type AddGlobalAccessControlListRequest struct {
	This        vimtypes.ManagedObjectReference `xml:"_this"`
	Permissions []AccessControl                 `xml:"permissions"`
}

type AddGlobalAccessControlListResponse struct {
}

type AddGlobalAccessControlListBody struct {
	Req    *AddGlobalAccessControlListRequest  `xml:"urn:inventoryservice AuthorizationService.AddGlobalAccessControlList,omitempty"`
	Res    *AddGlobalAccessControlListResponse `xml:"urn:inventoryservice AuthorizationService.AddGlobalAccessControlListResponse,omitempty"`
	Fault_ *soap.Fault                         `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *AddGlobalAccessControlListBody) Fault() *soap.Fault { return b.Fault_ }

type RemoveGlobalAccessRequest struct {
	This       vimtypes.ManagedObjectReference `xml:"_this"`
	Principals []Principal                     `xml:"principals"`
}

type RemoveGlobalAccessResponse struct {
}

type RemoveGlobalAccessBody struct {
	Req    *RemoveGlobalAccessRequest  `xml:"urn:inventoryservice AuthorizationService.RemoveGlobalAccess,omitempty"`
	Res    *RemoveGlobalAccessResponse `xml:"urn:inventoryservice AuthorizationService.RemoveGlobalAccessResponse,omitempty"`
	Fault_ *soap.Fault                 `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *RemoveGlobalAccessBody) Fault() *soap.Fault { return b.Fault_ }
