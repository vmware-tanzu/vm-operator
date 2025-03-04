// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	// KubernetesTopologyZoneLabelKey is the label used to specify the
	// availability zone for a VM.
	KubernetesTopologyZoneLabelKey = "topology.kubernetes.io/zone"
)

var (
	// ErrNoAvailabilityZones occurs when no availability zones are detected.
	ErrNoAvailabilityZones = errors.New("no availability zones")

	ErrNoZones = errors.New("no zones in specified namespace")
)

// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones/status,verbs=get;list;watch

// LookupZoneForClusterMoID returns the zone for the given Cluster MoID.
func LookupZoneForClusterMoID(
	ctx context.Context,
	client ctrlclient.Client,
	clusterMoID string) (string, error) {

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", err
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

	return "", fmt.Errorf("failed to find availability zone for cluster MoID %s", clusterMoID)
}

// GetNamespaceFolderAndRPMoID returns the Folder and ResourcePool MoID for the zone and namespace.
func GetNamespaceFolderAndRPMoID(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName, namespace string) (string, string, error) {

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zone, err := GetZone(ctx, client, availabilityZoneName, namespace)
		if err != nil {
			return "", "", err
		}
		if len(zone.Spec.ManagedVMs.PoolMoIDs) != 0 {
			return zone.Spec.ManagedVMs.FolderMoID, zone.Spec.ManagedVMs.PoolMoIDs[0], nil
		}
		return zone.Spec.ManagedVMs.FolderMoID, "", nil
	}

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

	var folderMoID string
	var rpMoIDs []string

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zones, err := GetZones(ctx, client, namespace)
		// If no Zones found in namespace, do not return err.
		if err != nil && !errors.Is(err, ErrNoZones) {
			return "", nil, err
		}

		for _, zone := range zones {
			if folderMoID == "" {
				folderMoID = zone.Spec.ManagedVMs.FolderMoID
			}
			rpMoIDs = append(rpMoIDs, zone.Spec.ManagedVMs.PoolMoIDs...)
		}

		return folderMoID, rpMoIDs, nil
	}

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", nil, err
	}

	for _, az := range availabilityZones {
		if nsInfo, ok := az.Spec.Namespaces[namespace]; ok {
			if folderMoID == "" {
				folderMoID = nsInfo.FolderMoId
			}
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

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zones, err := GetZones(ctx, client, namespace)
		// If no Zones found in namespace, do not return err here.
		if err != nil && !errors.Is(err, ErrNoZones) {
			return "", err
		}
		// Note that the Folder is VC-scoped, but we store the Folder MoID in each Zone CR
		// so we can return the first match.
		for _, zone := range zones {
			return zone.Spec.ManagedVMs.FolderMoID, nil
		}
		return "", fmt.Errorf("unable to get FolderMoID for namespace %s", namespace)
	}

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

	return availabilityZoneList.Items, nil
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

// GetZones returns a list of the Zone resources in a namespace.
func GetZones(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) ([]topologyv1.Zone, error) {

	zoneList := &topologyv1.ZoneList{}
	if err := client.List(ctx, zoneList, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}

	if len(zoneList.Items) == 0 {
		return nil, ErrNoZones
	}

	return zoneList.Items, nil
}

// GetZone returns a namespaced Zone resource.
func GetZone(
	ctx context.Context,
	client ctrlclient.Client,
	zoneName string,
	namespace string) (topologyv1.Zone, error) {

	var zone topologyv1.Zone
	err := client.Get(ctx, ctrlclient.ObjectKey{Name: zoneName, Namespace: namespace}, &zone)
	return zone, err
}

// GetZoneNames returns a set of zone names.
func GetZoneNames(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) (sets.Set[string], error) {

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zones, err := GetZones(ctx, client, namespace)
		if err != nil && !errors.Is(err, ErrNoZones) {
			return nil, err
		}
		s := sets.Set[string]{}
		for i := range zones {
			s[zones[i].Name] = sets.Empty{}
		}
		return s, nil
	}

	zones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return nil, err
	}
	s := sets.Set[string]{}
	for i := range zones {
		s[zones[i].Name] = sets.Empty{}
	}
	return s, nil
}
