// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
)

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
