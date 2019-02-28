/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"context"

	"github.com/golang/glog"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

// TODO: Make task tracking funcs async via goroutines

type VSphereManager struct {
	Config        VSphereVmProviderConfig

	// cached Session Context
	Session       *session.SessionContext

	//cache Resource Context
	ResourceContext *ResourceContext
}

// Construct a ResourceContext from VSphereVMProviderConfig. This will implicitly validate the config as well
// since we create the objects here. Any ambiguity on the VC will return an error failing the operation.
// TODO (parunesh): This should be retried?
func InitResourceContext(sc *session.SessionContext, config *VSphereVmProviderConfig) (*ResourceContext, error) {
	glog.Infof("Initializing Resource Context")
	ctx, close := context.WithCancel(context.Background())
	defer close()

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

// Initialize a new VSphereManager. Create and cache a new SessionContext, ResourceContext
func NewVSphereManager() *VSphereManager {
	glog.Infof("Initializing VSphereManager!!")
	// TODO(Arunesh): Should we defer a session start to when someone actually makes a call?
	config := GetVsphereVmProviderConfig()

	sc, err := session.NewSessionContext(config.VcUrl)
	if err != nil {
		// TODO(Arunesh): Should this be non fatal and retried later?
		glog.Fatalf("Error in creating the Session Context [%s]", err)
	}

	rc, err := InitResourceContext(sc, config)
	if err != nil {
		glog.Errorf("Error in initializing Resource Context [%s]", err)
	}

	return &VSphereManager{
		Config: *config,
		Session:sc,
		ResourceContext:rc,
	}
}

// Return a cached session if exists. Create, cache and return a new one otherwise.
// This should be the only entry point for getting a session to ensure we dont duplicate govmomi clients.
// TODO(parunesh): Handle thread safety; Should we check ResourceContext here as well?
func (vs *VSphereManager) GetSession() (*session.SessionContext, error) {
	if vs.Session != nil {
		return vs.Session, nil
	}
	sc, err := session.NewSessionContext(vs.Config.VcUrl)
	if err != nil {
		glog.Errorf("Error creating a new Session: [%s]", err)
		return nil, err
	}
	vs.Session = sc

	return sc, nil
}
