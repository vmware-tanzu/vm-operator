/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"

	"github.com/pkg/errors"

	"github.com/golang/glog"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

type VSphereManager struct {
	Config VSphereVmProviderConfig

	// Cached SessionContext
	Session *session.SessionContext

	// Cached ResourceContext
	ResourceContext *ResourceContext
}

// Construct a ResourceContext from VSphereVMProviderConfig. This will implicitly validate the config as well
// since we create the objects here. Any ambiguity on the VC will return an error failing the operation.
func initResourceContext(sc *session.SessionContext, config *VSphereVmProviderConfig) (*ResourceContext, error) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dc, err := resources.NewDatacenter(ctx, sc.Finder, config.Datacenter)
	if err != nil {
		return nil, err
	}

	sc.Finder.SetDatacenter(dc.Datacenter)

	folder, err := resources.NewFolder(ctx, sc.Finder, dc.Datacenter, config.Folder)
	if err != nil {
		return nil, err
	}

	rp, err := resources.NewResourcePool(ctx, sc.Finder, config.ResourcePool)
	if err != nil {
		return nil, err
	}

	ds, err := resources.NewDatastore(ctx, sc.Finder, config.Datastore)
	if err != nil {
		return nil, err
	}

	return &ResourceContext{
		Datacenter:   dc,
		Folder:       folder,
		ResourcePool: rp,
		Datastore:    ds,
	}, nil
}

// NewVSphereManager creates a new VSphereManager. Create and cache the SessionContext and ResourceContext.
func NewVSphereManager(config *VSphereVmProviderConfig) (*VSphereManager, error) {
	glog.Infof("Initializing VSphereManager")

	sc, err := session.NewSessionContext(config.VcUrl)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create SessionContext")
	}

	rc, err := initResourceContext(sc, config)
	if err != nil {
		return nil, errors.Wrap(err, "cannot initialize the session ResourceContext")
	}

	return &VSphereManager{
		Config:          *config,
		Session:         sc,
		ResourceContext: rc,
	}, nil
}

// GetSession returns a cached session if exists. Create, cache and return a new one otherwise.
// This should be the only entry point for getting a session to ensure we dont duplicate govmomi clients.
// TODO(parunesh): Handle thread safety; Should we check ResourceContext here as well?
func (v *VSphereManager) GetSession() (*session.SessionContext, error) {

	if v.Session != nil {
		return v.Session, nil
	}

	// BMV: This is dead code, right? NewVSphereManager() always initializes the SessionContext so
	// when do we get here? The new SessionContext is missing the sc.Finder.SetDatacenter(dc.Datacenter)
	// call from initResourceContext() would this work anyways?
	sc, err := session.NewSessionContext(v.Config.VcUrl)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create new session")
	}

	v.Session = sc

	return sc, nil
}
