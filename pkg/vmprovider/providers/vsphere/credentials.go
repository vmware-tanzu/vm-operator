/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VSphereVmProviderCredentials wraps the data needed to login to vCenter.
type VSphereVmProviderCredentials struct {
	Username string
	Password string
}

func GetSecret(clientSet kubernetes.Interface, secretNamespace string, secretName string) (*v1.Secret, error) {
	secret, err := clientSet.CoreV1().Secrets(secretNamespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot find secret %s in namespace %s", secretName, secretNamespace)
	}

	return secret, nil
}

func GetProviderCredentials(clientSet kubernetes.Interface, namespace string, secretName string) (*VSphereVmProviderCredentials, error) {
	secret, err := GetSecret(clientSet, namespace, secretName)
	if err != nil {
		return nil, err
	}

	var credentials VSphereVmProviderCredentials
	credentials.Username = string(secret.Data["username"])
	credentials.Password = string(secret.Data["password"])

	if credentials.Username == "" || credentials.Password == "" {
		return nil, errors.New("vCenter username and password are missing")
	}

	return &credentials, nil
}

func ProviderCredentialsToSecret(namespace string, credentials *VSphereVmProviderCredentials, vcCredsSecretName string) *v1.Secret {
	return &v1.Secret{
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

func InstallVSphereVmProviderSecret(clientSet *kubernetes.Clientset, namespace string, credentials *VSphereVmProviderCredentials, vcCredsSecretName string) error {
	secret := ProviderCredentialsToSecret(namespace, credentials, vcCredsSecretName)

	if _, err := clientSet.CoreV1().Secrets(namespace).Get(secret.Name, metav1.GetOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		_, err = clientSet.CoreV1().Secrets(namespace).Create(secret)
		return err
	}

	_, err := clientSet.CoreV1().Secrets(namespace).Update(secret)
	return err
}
