// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// esxHostMoIDNodeAnnotationKey is the annotation a Supervisor Node object
// carries naming the MOID of the ESXi host it corresponds to.
const esxHostMoIDNodeAnnotationKey = "vmware-system-esxi-node-moid"

// GetESXHostInfoForNode returns the ESXi HostSystem MoID and availability
// zone for the Supervisor Node with the given name. It returns an error if
// the Node does not exist or does not carry the ESXi host MOID annotation.
func GetESXHostInfoForNode(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	nodeName string) (hostMoID, zoneName string, err error) {

	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: nodeName}, node); err != nil {
		return "", "", fmt.Errorf("failed to get Node %q: %w", nodeName, err)
	}

	hostMoID = node.Annotations[esxHostMoIDNodeAnnotationKey]
	if hostMoID == "" {
		return "", "", fmt.Errorf(
			"node %q does not have the %q annotation",
			nodeName, esxHostMoIDNodeAnnotationKey)
	}

	return hostMoID, node.Labels[corev1.LabelTopologyZone], nil
}
