// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	cnsoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetCnsNodeVMBatchAttachment(ctx context.Context, client ctrlclient.Client, ns, name string) (*cnsoperatorv1alpha1.CnsNodeVMBatchAttachment, error) {
	cnsNodeVMBatchAttachment := &cnsoperatorv1alpha1.CnsNodeVMBatchAttachment{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, cnsNodeVMBatchAttachment)
	if err != nil {
		return nil, err
	}

	return cnsNodeVMBatchAttachment, nil
}
