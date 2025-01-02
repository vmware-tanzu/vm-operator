// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace, secretName, secretKey string,
	out *string) error {

	secret, err := GetSecretResource(ctx, k8sClient, secretNamespace, secretName)
	if err != nil {
		return err
	}
	data := secret.Data[secretKey]
	if len(data) == 0 {
		return fmt.Errorf(
			"no data found for key %q for secret %s/%s",
			secretKey, secretNamespace, secretName)
	}
	*out = string(data)
	return nil
}

func GetSecretResource(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace, secretName string) (*corev1.Secret, error) {

	var secret corev1.Secret
	key := ctrlclient.ObjectKey{Name: secretName, Namespace: secretNamespace}
	if err := k8sClient.Get(ctx, key, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}
