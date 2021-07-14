// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"context"

	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
)

func CreateZones(
	ctx context.Context,
	client ctrlclient.Client,
	zones ...topologyv1.AvailabilityZone) {

	for _, az := range zones {
		// capture the loop variable
		az := az
		Expect(client.Create(ctx, &az)).ToNot(HaveOccurred())
	}
}
