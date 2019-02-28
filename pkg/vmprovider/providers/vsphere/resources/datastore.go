/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"

	"github.com/golang/glog"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type Datastore struct {
	Datastore  *object.Datastore
	name       string
}

// Lookup a Datastore with a given name. If success, return a resources.Datastore, error otherwise.
func NewDatastore(ctx context.Context, finder *find.Finder, dsName string) (*Datastore, error) {
	ds, err := finder.DatastoreOrDefault(ctx, dsName)
	if err != nil {
		glog.Errorf("No Datastores with name: %s found. Error: [%s]", dsName, err)
		return nil, err
	}

	return &Datastore{
		name: dsName,
		Datastore: ds,
	}, nil
}
