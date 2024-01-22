// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"context"
	"errors"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
)

const (
	// DefaultAvailabilityZoneName is the name of the default availability
	// zone used in backwards compatibility mode.
	DefaultAvailabilityZoneName = "default"

	// KubernetesTopologyZoneLabelKey is the label used to specify the
	// availability zone for a VM.
	KubernetesTopologyZoneLabelKey = "topology.kubernetes.io/zone"

	// NamespaceRPAnnotationKey is the annotation value set by WCP that
	// indicates the managed object ID of the resource pool for a given
	// namespace.
	NamespaceRPAnnotationKey = "vmware-system-resource-pool"

	// NamespaceFolderAnnotationKey is the annotation value set by WCP
	// that indicates the managed object ID of the folder for a given
	// namespace.
	NamespaceFolderAnnotationKey = "vmware-system-vm-folder"
)

var (
	// ErrNoAvailabilityZones occurs when no availability zones are detected.
	ErrNoAvailabilityZones = errors.New("no availability zones")

	// ErrWcpFaultDomainsFSSIsEnabled occurs when a function is invoked that
	// is disabled when the WCP_FaultDomains FSS is enabled.
	ErrWcpFaultDomainsFSSIsEnabled = errors.New("wcp fault domains fss is enabled")
)

// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones/status,verbs=get;list;watch

// LookupZoneForClusterMoID returns the zone for the given Cluster MoID.
func LookupZoneForClusterMoID(
	ctx context.Context,
	client ctrlclient.Client,
	clusterMoID string) (string, error) {

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", err
	}

	if len(availabilityZones) == 0 {
		return "", fmt.Errorf("no AvailabilityZones")
	}

	for _, az := range availabilityZones {
		if az.Spec.ClusterComputeResourceMoId == clusterMoID {
			return az.Name, nil
		}

		for _, moID := range az.Spec.ClusterComputeResourceMoIDs {
			if moID == clusterMoID {
				return az.Name, nil
			}
		}
	}

	return "", fmt.Errorf("failed to find zone for cluster MoID %s", clusterMoID)
}

// GetNamespaceFolderAndRPMoID returns the Folder and ResourcePool MoID for the zone and namespace.
func GetNamespaceFolderAndRPMoID(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName, namespace string) (string, string, error) {

	availabilityZone, err := GetAvailabilityZone(ctx, client, availabilityZoneName)
	if err != nil {
		return "", "", err
	}

	nsInfo, ok := availabilityZone.Spec.Namespaces[namespace]
	if !ok {
		return "", "", fmt.Errorf("availability zone %q missing info for namespace %s",
			availabilityZoneName, namespace)
	}

	poolMoID := nsInfo.PoolMoId
	if len(nsInfo.PoolMoIDs) != 0 {
		poolMoID = nsInfo.PoolMoIDs[0]
	}

	return nsInfo.FolderMoId, poolMoID, nil
}

// GetNamespaceFolderAndRPMoIDs returns the Folder and ResourcePool MoIDs for the namespace, across all zones.
func GetNamespaceFolderAndRPMoIDs(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) (string, []string, error) {

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", nil, err
	}

	var folderMoID string
	var rpMoIDs []string

	for _, az := range availabilityZones {
		if nsInfo, ok := az.Spec.Namespaces[namespace]; ok {
			folderMoID = nsInfo.FolderMoId
			if len(nsInfo.PoolMoIDs) != 0 {
				rpMoIDs = append(rpMoIDs, nsInfo.PoolMoIDs...)
			} else {
				rpMoIDs = append(rpMoIDs, nsInfo.PoolMoId)
			}
		}
	}

	return folderMoID, rpMoIDs, nil
}

// GetNamespaceFolderMoID returns the FolderMoID for the namespace.
func GetNamespaceFolderMoID(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) (string, error) {

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", err
	}

	// Note that the Folder is VC-scoped, but we store the Folder MoID in each Zone CR
	// so we can return the first match.
	for _, zone := range availabilityZones {
		nsInfo, ok := zone.Spec.Namespaces[namespace]
		if ok {
			return nsInfo.FolderMoId, nil
		}
	}

	return "", fmt.Errorf("unable to get FolderMoID for namespace %s", namespace)
}

// GetAvailabilityZones returns a list of the AvailabilityZone resources.
func GetAvailabilityZones(
	ctx context.Context,
	client ctrlclient.Client) ([]topologyv1.AvailabilityZone, error) {

	availabilityZoneList := &topologyv1.AvailabilityZoneList{}
	if err := client.List(ctx, availabilityZoneList); err != nil {
		return nil, err
	}

	if len(availabilityZoneList.Items) == 0 {
		return nil, ErrNoAvailabilityZones
	}

	zones := make([]topologyv1.AvailabilityZone, len(availabilityZoneList.Items))
	copy(zones, availabilityZoneList.Items)

	return zones, nil
}

// GetAvailabilityZone returns a named AvailabilityZone resource.
func GetAvailabilityZone(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName string) (topologyv1.AvailabilityZone, error) {

	var availabilityZone topologyv1.AvailabilityZone
	err := client.Get(ctx, ctrlclient.ObjectKey{Name: availabilityZoneName}, &availabilityZone)
	return availabilityZone, err
}
