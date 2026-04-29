package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// GetHostsForCluster queries a specific vSphere cluster for its host membership.
func GetHostsForCluster(ctx context.Context, client *vim25.Client, clusterMoID string) ([]string, error) {
	// Create cluster object reference
	cluster := object.NewClusterComputeResource(
		client,
		vimtypes.ManagedObjectReference{
			Type:  "ClusterComputeResource",
			Value: clusterMoID,
		},
	)

	// Query cluster properties to get host references
	var cr mo.ComputeResource
	err := cluster.Properties(ctx, cluster.Reference(), []string{"host"}, &cr)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster properties: %w", err)
	}

	// Extract host IDs from the references
	hostIDs := make([]string, len(cr.Host))
	for i, hostRef := range cr.Host {
		hostIDs[i] = hostRef.Value
	}

	return hostIDs, nil
}
