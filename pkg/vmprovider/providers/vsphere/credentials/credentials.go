// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
)

// VSphereVMProviderCredentials wraps the data needed to login to vCenter.
type VSphereVMProviderCredentials struct {
	Username string
	Password string
}

func GetProviderCredentials(client ctrlruntime.Client, namespace string, secretName string) (*VSphereVMProviderCredentials, error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: namespace, Name: secretName}
	if err := client.Get(context.Background(), secretKey, secret); err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, errors.Wrapf(err, "cannot find secret for provider credentials: %s", secretKey)
	}

	var credentials VSphereVMProviderCredentials
	credentials.Username = string(secret.Data["username"])
	credentials.Password = string(secret.Data["password"])

	if credentials.Username == "" || credentials.Password == "" {
		return nil, errors.New("vCenter username and password are missing")
	}

	return &credentials, nil
}

func ProviderCredentialsToSecret(namespace string, credentials *VSphereVMProviderCredentials, vcCredsSecretName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vcCredsSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(credentials.Username),
			"password": []byte(credentials.Password),
		},
	}
}

func InstallVSphereVMProviderSecret(client ctrlruntime.Client, namespace string, credentials *VSphereVMProviderCredentials, vcCredsSecretName string) error {
	secret := ProviderCredentialsToSecret(namespace, credentials, vcCredsSecretName)

	if err := client.Create(context.Background(), secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		return client.Update(context.Background(), secret)
	}

	return nil
}
