// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// QueryConfigOptionEx retrieves the VirtualMachineConfigOption for the given
// hardware version from the EnvironmentBrowser of the cluster identified by
// clusterMoID.
func QueryConfigOptionEx(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoID string,
	hardwareVersion string) (*vimtypes.VirtualMachineConfigOption, error) {

	ccrRef := vimtypes.ManagedObjectReference{
		Type:  "ClusterComputeResource",
		Value: clusterMoID,
	}

	var cr mo.ClusterComputeResource
	pc := property.DefaultCollector(vimClient)
	if err := pc.RetrieveOne(ctx, ccrRef, []string{"environmentBrowser"}, &cr); err != nil {
		return nil, fmt.Errorf("failed to retrieve EnvironmentBrowser for cluster %s: %w",
			clusterMoID, err)
	}

	if cr.EnvironmentBrowser == nil {
		return nil, fmt.Errorf("cluster %s has no EnvironmentBrowser", clusterMoID)
	}

	eb := object.NewEnvironmentBrowser(vimClient, *cr.EnvironmentBrowser)
	spec := &vimtypes.EnvironmentBrowserConfigOptionQuerySpec{
		Key: hardwareVersion,
	}
	opt, err := eb.QueryConfigOption(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("QueryConfigOptionEx for %s failed: %w", hardwareVersion, err)
	}
	return opt, nil
}
