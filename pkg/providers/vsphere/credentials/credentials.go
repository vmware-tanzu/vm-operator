// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// VSphereVMProviderCredentials wraps the data needed to login to vCenter.
type VSphereVMProviderCredentials struct {
	Username string
	Password string
}

func GetProviderCredentials(
	ctx context.Context,
	client ctrlclient.Client,
	namespace, secretName string) (VSphereVMProviderCredentials, error) {

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: namespace, Name: secretName}
	if err := client.Get(ctx, secretKey, secret); err != nil {
		return VSphereVMProviderCredentials{}, fmt.Errorf("cannot find secret for provider credentials: %s: %w", secretKey, err)
	}

	return ExtractVCCredentials(secret.Data)
}

func ExtractVCCredentials(data map[string][]byte) (VSphereVMProviderCredentials, error) {
	credentials := VSphereVMProviderCredentials{
		Username: string(data["username"]),
		Password: string(data["password"]),
	}

	if credentials.Username == "" || credentials.Password == "" {
		return VSphereVMProviderCredentials{}, errors.New("vCenter username and password are missing")
	}

	return credentials, nil
}
