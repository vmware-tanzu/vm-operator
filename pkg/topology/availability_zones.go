// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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

// GetAvailabilityZones returns a list of the AvailabilityZone resources.
func GetAvailabilityZones(
	ctx context.Context,
	client ctrlclient.Client) ([]topologyv1.AvailabilityZone, error) {
	availabilityZoneList := &topologyv1.AvailabilityZoneList{}
	if err := client.List(ctx, availabilityZoneList); err != nil {
		return nil, err
	}

	if len(availabilityZoneList.Items) == 0 {
		// If there are no AZs and the WCP Fault Domain FSS is enabled then
		// do not default to backwards compatibility mode.
		if lib.IsWcpFaultDomainsFSSEnabled() {
			return nil, ErrNoAvailabilityZones
		}

		// There are no AZs and the WCP Fault Domain FSS is disabled, so it is
		// okay to use backwards compatibility mode.
		defaultAz, err := GetDefaultAvailabilityZone(ctx, client)
		if err != nil {
			return nil, err
		}

		return []topologyv1.AvailabilityZone{defaultAz}, nil
	}

	zones := make([]topologyv1.AvailabilityZone, len(availabilityZoneList.Items))
	for i := range availabilityZoneList.Items {
		zones[i] = availabilityZoneList.Items[i]
	}

	return zones, nil
}

// GetAvailabilityZone returns a named AvailabilityZone resource.
func GetAvailabilityZone(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName string) (topologyv1.AvailabilityZone, error) {
	var availabilityZone topologyv1.AvailabilityZone
	if err := client.Get(
		ctx,
		ctrlclient.ObjectKey{Name: availabilityZoneName},
		&availabilityZone); err != nil {
		// If the AZ was not found, the WCP FaultDomains FSS is not
		// enabled, and the requested AZ matches the name of the default
		// AZ, then return the default AZ.
		if apierrors.IsNotFound(err) {
			if !lib.IsWcpFaultDomainsFSSEnabled() {
				if availabilityZoneName == DefaultAvailabilityZoneName {
					// Return the default AZ.
					return GetDefaultAvailabilityZone(ctx, client)
				}
			}
		}

		return availabilityZone, err
	}
	return availabilityZone, nil
}

// GetDefaultAvailabilityZone returns the default AvailabilityZone resource
// by inspecting the available, DevOps Namespace resources and transforming
// them into AvailabilityZone resources by virtue of the annotations on the
// Namespace that indicate the managed object IDs of the Namespace's vSphere
// Resource Pool and Folder objects.
func GetDefaultAvailabilityZone(
	ctx context.Context,
	client ctrlclient.Client) (topologyv1.AvailabilityZone, error) {
	// Please note the default AZ has no ClusterComputeResourceMoId, and this
	// is okay, because the Session.init call will grab the Cluster's MoId from
	// the resource pool.
	availabilityZone := topologyv1.AvailabilityZone{
		ObjectMeta: v1.ObjectMeta{
			Name: DefaultAvailabilityZoneName,
		},
		Spec: topologyv1.AvailabilityZoneSpec{
			Namespaces: map[string]topologyv1.NamespaceInfo{},
		},
	}

	if lib.IsWcpFaultDomainsFSSEnabled() {
		return availabilityZone, ErrWcpFaultDomainsFSSIsEnabled
	}

	// There are no AZs and the WCP Fault Domain FSS is disabled, so it is
	// okay to use backwards compatibility mode.
	namespaceList := &corev1.NamespaceList{}
	if err := client.List(ctx, namespaceList); err != nil {
		return availabilityZone, err
	}

	// Collect all the DevOps namespaces into the AvailabilityZone's Namespaces map.
	for _, ns := range namespaceList.Items {
		poolMoID := ns.Annotations[NamespaceRPAnnotationKey]
		folderMoID := ns.Annotations[NamespaceFolderAnnotationKey]
		if poolMoID != "" && folderMoID != "" {
			availabilityZone.Spec.Namespaces[ns.Name] = topologyv1.NamespaceInfo{
				PoolMoId:   poolMoID,
				FolderMoId: folderMoID,
			}
		}
	}

	return availabilityZone, nil
}
