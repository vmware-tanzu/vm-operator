/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Package session caches objects related to a vSphere session.
package session

import (
	"context"

	"github.com/pkg/errors"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/soap"
)

// When VSphereVmProvider is initialized, we create a session and cache it in VSphereManager. Further calls to
// VSphereManager.GetSession return the cached session (create a new one if not).
type SessionContext struct {
	Client *govmomi.Client
	Finder *find.Finder

	// TODO(parunesh): other customizations to session (insecure, keepalives etc)
}

// NewSessionContext returns a new SessionContext. A session is created using the url provided.
// This should be changed to creating sessions using certs and identities, or at least the url
// should not contain the password.
func NewSessionContext(url string) (*SessionContext, error) {

	soapUrl, err := soap.ParseURL(url)
	if soapUrl == nil || err != nil {
		return nil, errors.Wrapf(err, "cannot parse soap url: %s", soapUrl)
	}

	client, err := govmomi.NewClient(context.Background(), soapUrl, true)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create govmoni client")
	}

	var sc SessionContext
	sc.Client = client
	sc.Finder = find.NewFinder(sc.Client.Client, false)

	return &sc, nil
}
