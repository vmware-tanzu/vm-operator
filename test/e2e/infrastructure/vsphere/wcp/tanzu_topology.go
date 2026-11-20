// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"context"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ListZonesByNamespace(ctx context.Context, client ctrlclient.Client, ns string) (*topologyv1.ZoneList, error) {
	listOptions := &ctrlclient.ListOptions{Namespace: ns}

	zoneList := &topologyv1.ZoneList{}

	err := client.List(ctx, zoneList, listOptions)
	if err != nil {
		return nil, err
	}

	return zoneList, nil
}
