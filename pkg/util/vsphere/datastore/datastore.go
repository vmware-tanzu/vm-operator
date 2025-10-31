// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
)

// GetDatastoreURLFromDatastorePath returns the datastore URL for a given
// datastore path.
func GetDatastoreURLFromDatastorePath(
	ctx context.Context,
	vimClient *vim25.Client,
	path string) (string, error) {

	if ctx == nil {
		panic("ctx is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}

	var dsPath object.DatastorePath
	if !dsPath.FromString(path) {
		return "", fmt.Errorf("failed to parse datastore path %q", path)
	}

	finder := find.NewFinder(vimClient)

	datastore, err := finder.Datastore(ctx, dsPath.Datastore)
	if err != nil {
		return "", fmt.Errorf(
			"failed to get datastore for %q: %w", dsPath.Datastore, err)
	}

	return datastore.NewURL(dsPath.Path).String(), nil
}
