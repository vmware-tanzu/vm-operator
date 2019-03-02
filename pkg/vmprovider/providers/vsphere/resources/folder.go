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

type Folder struct {
	Name   string
	Folder *object.Folder
}

// Lookup a Folder with a given name in the given datacenter. If success, return a resources.Folder, error otherwise.
func NewFolder(ctx context.Context, finder *find.Finder, dc *object.Datacenter, name string) (*Folder, error) {
	folders, err := dc.Folders(ctx)
	if err != nil {
		glog.Errorf("No Folders with name: [%s] found.", name)
		return nil, err
	}

	return &Folder{
		Name:   name,
		Folder: folders.VmFolder,
	}, nil
}
