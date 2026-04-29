// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere"

	"github.com/vmware/govmomi/vim25"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
)

// ZoneHostInfo contains host information for a specific zone.
type ZoneHostInfo struct {
	ZoneName string   `json:"zoneName"`
	HostIDs  []string `json:"hostIDs"`
}

func ListAvailabilityZones(ctx context.Context, client ctrlclient.Client) (*topologyv1.AvailabilityZoneList, error) {
	availabilityZoneList := &topologyv1.AvailabilityZoneList{}

	err := client.List(ctx, availabilityZoneList)
	if err != nil {
		return nil, err
	}

	return availabilityZoneList, nil
}

func ListZonesByNamespace(ctx context.Context, client ctrlclient.Client, ns string) (*topologyv1.ZoneList, error) {
	listOptions := &ctrlclient.ListOptions{Namespace: ns}

	zoneList := &topologyv1.ZoneList{}

	err := client.List(ctx, zoneList, listOptions)
	if err != nil {
		return nil, err
	}

	return zoneList, nil
}

// GetHostsPerZone returns accurate mapping of zone names to their associated host IDs.
func GetHostsPerZone(ctx context.Context, k8sClient ctrlclient.Client, kubeconfigPath string) ([]ZoneHostInfo, error) {
	// Get all availability zones (cluster-scoped)
	availabilityZones, err := ListAvailabilityZones(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to list availability zones: %w", err)
	}

	// Create vSphere client
	vimClient := vcenter.NewVimClientFromKubeconfigFile(ctx, kubeconfigPath)
	if vimClient == nil {
		return nil, fmt.Errorf("failed to create vSphere client")
	}
	defer vcenter.LogoutVimClient(vimClient)

	var zoneHostInfos []ZoneHostInfo

	// For each availability zone, query its cluster(s) for hosts
	for _, az := range availabilityZones.Items {
		hostIDs, err := getHostsForAvailabilityZone(ctx, vimClient, &az)
		if err != nil {
			return nil, fmt.Errorf("failed to get hosts for zone %s: %w", az.Name, err)
		}

		zoneInfo := ZoneHostInfo{
			ZoneName: az.Name,
			HostIDs:  hostIDs,
		}
		zoneHostInfos = append(zoneHostInfos, zoneInfo)
	}

	return zoneHostInfos, nil
}

// getHostsForAvailabilityZone queries vSphere to get all hosts in the clusters associated with an availability zone.
func getHostsForAvailabilityZone(ctx context.Context, client *vim25.Client, az *topologyv1.AvailabilityZone) ([]string, error) {
	var allHostIDs []string

	// Handle both single and multiple cluster MoIDs
	clusterMoIDs := az.Spec.ClusterComputeResourceMoIDs
	if len(clusterMoIDs) == 0 && az.Spec.ClusterComputeResourceMoId != "" {
		clusterMoIDs = []string{az.Spec.ClusterComputeResourceMoId}
	}

	if len(clusterMoIDs) == 0 {
		return nil, fmt.Errorf("no cluster compute resource MoIDs found for availability zone %s", az.Name)
	}

	// Query each cluster for its hosts
	for _, clusterMoID := range clusterMoIDs {
		hostIDs, err := vsphere.GetHostsForCluster(ctx, client, clusterMoID)
		if err != nil {
			return nil, fmt.Errorf("failed to get hosts for cluster %s: %w", clusterMoID, err)
		}
		allHostIDs = append(allHostIDs, hostIDs...)
	}

	return allHostIDs, nil
}
