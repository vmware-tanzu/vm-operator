// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package veeam

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	managedServersPath = "/api/v1/backupInfrastructure/managedServers"
	credentialsPath    = "/api/v1/credentials"

	// managedServerTypeViHost is the managed server type of a vCenter or a
	// standalone ESXi host.
	managedServerTypeViHost = "ViHost"

	vCenterPort = 443

	// ManagedServerStatusAvailable is the status of a managed server VBR can
	// connect to.
	ManagedServerStatusAvailable = "Available"
)

// ManagedServer is a server VBR manages, such as a vCenter.
type ManagedServer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	CredentialsID string `json:"credentialsId"`
	Status        string `json:"status"`
}

// VCenterSpec describes a vCenter to register with VBR.
type VCenterSpec struct {
	// Name is the vCenter PNID. Registering by IP fails because VBR checks
	// the certificate's name against it.
	Name        string
	Username    string
	Password    string
	Description string
}

// FindManagedServer returns the vSphere managed server with the given name,
// or nil when VBR does not manage it. Names are compared case-insensitively,
// as host names are.
func (c *Client) FindManagedServer(ctx context.Context, name string) (*ManagedServer, error) {
	var resp page[ManagedServer]

	// Match the name client-side rather than with nameFilter, a pattern
	// match: missing an existing registration would add the vCenter twice.
	if err := c.do(ctx, "GET", managedServersPath+"?typeFilter="+managedServerTypeViHost, nil, &resp); err != nil {
		return nil, err
	}

	for i := range resp.Data {
		if s := resp.Data[i]; s.Type == managedServerTypeViHost && strings.EqualFold(s.Name, name) {
			return &s, nil
		}
	}

	return nil, nil
}

// RegisterVCenter adds a vCenter to VBR and waits until VBR has connected to
// it. It creates a credentials record for the vCenter, which UnregisterVCenter
// removes along with the managed server. If registration fails, the
// credentials record is removed before returning.
func (c *Client) RegisterVCenter(ctx context.Context, spec VCenterSpec, opts WaitOptions) (ManagedServer, error) {
	var creds struct {
		ID string `json:"id"`
	}

	if err := c.do(ctx, "POST", credentialsPath, map[string]any{
		"type":        "Standard",
		"username":    spec.Username,
		"password":    spec.Password,
		"description": spec.Description,
	}, &creds); err != nil {
		return ManagedServer{}, fmt.Errorf("failed to create veeam credentials for vCenter %s: %w", spec.Name, err)
	}

	if creds.ID == "" {
		return ManagedServer{}, fmt.Errorf("veeam created credentials for vCenter %s but returned no id", spec.Name)
	}

	server, err := c.addVCenter(ctx, spec, creds.ID, opts)
	if err != nil {
		if delErr := c.deleteCredentials(ctx, creds.ID); delErr != nil {
			err = errors.Join(err, delErr)
		}

		return ManagedServer{}, err
	}

	return server, nil
}

func (c *Client) addVCenter(ctx context.Context, spec VCenterSpec, credentialsID string, opts WaitOptions) (ManagedServer, error) {
	// VBR only accepts the thumbprint in the form it computes itself.
	var cert struct {
		Certificate struct {
			Thumbprint string `json:"thumbprint"`
		} `json:"certificate"`
	}

	if err := c.do(ctx, "POST", "/api/v1/connectionCertificate", map[string]any{
		"serverName":    spec.Name,
		"type":          managedServerTypeViHost,
		"credentialsId": credentialsID,
		"port":          vCenterPort,
	}, &cert); err != nil {
		return ManagedServer{}, fmt.Errorf("failed to get the certificate of vCenter %s: %w", spec.Name, err)
	}

	if cert.Certificate.Thumbprint == "" {
		return ManagedServer{}, fmt.Errorf("veeam returned no certificate thumbprint for vCenter %s", spec.Name)
	}

	var s Session
	if err := c.do(ctx, "POST", managedServersPath, map[string]any{
		"type":                  managedServerTypeViHost,
		"name":                  spec.Name,
		"description":           spec.Description,
		"credentialsId":         credentialsID,
		"port":                  vCenterPort,
		"certificateThumbprint": cert.Certificate.Thumbprint,
	}, &s); err != nil {
		return ManagedServer{}, fmt.Errorf("failed to add vCenter %s to veeam: %w", spec.Name, err)
	}

	if _, err := c.WaitForSession(ctx, s.ID, opts); err != nil {
		return ManagedServer{}, fmt.Errorf("failed to add vCenter %s to veeam: %w", spec.Name, err)
	}

	server, err := c.FindManagedServer(ctx, spec.Name)
	if err != nil {
		return ManagedServer{}, err
	}

	if server == nil {
		return ManagedServer{}, fmt.Errorf("veeam added vCenter %s but does not list it as a managed server", spec.Name)
	}

	return *server, nil
}

// UnregisterVCenter removes a managed server that RegisterVCenter added, and
// then its credentials record. Missing objects are ignored.
func (c *Client) UnregisterVCenter(ctx context.Context, server ManagedServer, opts WaitOptions) error {
	var s Session

	err := c.do(ctx, "DELETE", managedServersPath+"/"+server.ID, nil, &s)

	switch {
	case IsNotFound(err):
	case err != nil:
		return fmt.Errorf("failed to remove vCenter %s (%s) from veeam: %w", server.Name, server.ID, err)
	case s.ID != "":
		if _, err := c.WaitForSession(ctx, s.ID, opts); err != nil {
			return fmt.Errorf("failed to remove vCenter %s (%s) from veeam: %w", server.Name, server.ID, err)
		}
	}

	return c.deleteCredentials(ctx, server.CredentialsID)
}

func (c *Client) deleteCredentials(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}

	if err := c.do(ctx, "DELETE", credentialsPath+"/"+id, nil, nil); err != nil && !IsNotFound(err) {
		return fmt.Errorf("failed to delete veeam credentials %s: %w", id, err)
	}

	return nil
}
