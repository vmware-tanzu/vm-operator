/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Package session caches objects related to a vSphere session
package session

import (
	"context"

	"github.com/golang/glog"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/soap"
)

// When VSphereVmProvider is initialized, we create a session and cache it in VSphereManager. Further calls to
// VSphereManager.GetSession return the cached session (create a new one if not).
type SessionContext struct {
	Client *govmomi.Client
	Finder *find.Finder

	// TODO (parunesh): other customizations to session (insecure, keepalives etc)
}

// For now, a session is created using the url provided in the config. This should be changed to creating sessions
// using certs and identities.
func NewSessionContext(url string) (*SessionContext, error) {
	var sc SessionContext

	soapUrl, err := soap.ParseURL(url)
	if soapUrl == nil || err != nil {
		glog.Errorf("Error parsing Url: %s [%s]", soapUrl, err)
		return nil, err
	}

	sc.Client, err = govmomi.NewClient(context.Background(), soapUrl, true)
	if err != nil {
		glog.Errorf("Error while creating a new Client: [%s]", err)
		return nil, err
	}

	sc.Finder = find.NewFinder(sc.Client.Client, false)

	return &sc, nil
}
