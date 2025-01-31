// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package sysprep

import (
	"context"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

type SecretData struct {
	ProductID, Password, DomainPassword string
}

func GetSysprepSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *sysprep.Sysprep) (SecretData, error) {

	var (
		productID, password, domainPwd string
	)

	if in.UserData.ProductID != nil {
		// this is an optional secret key selector even when FullName or OrgName are set.
		if err := util.GetSecretData(
			ctx,
			k8sClient,
			secretNamespace,
			in.UserData.ProductID.Name,
			in.UserData.ProductID.Key,
			&productID); err != nil {

			return SecretData{}, err
		}
	}

	if guiUnattended := in.GUIUnattended; guiUnattended != nil && guiUnattended.AutoLogon {
		err := util.GetSecretData(ctx,
			k8sClient,
			secretNamespace,
			guiUnattended.Password.Name,
			guiUnattended.Password.Key,
			&password)
		if err != nil {
			return SecretData{}, err
		}
	}

	if identification := in.Identification; identification != nil {
		if dap := identification.DomainAdminPassword; dap != nil && dap.Name != "" {
			err := util.GetSecretData(ctx,
				k8sClient,
				secretNamespace,
				dap.Name,
				dap.Key,
				&domainPwd)
			if err != nil {
				return SecretData{}, err
			}
		}
	}

	return SecretData{
		ProductID:      productID,
		Password:       password,
		DomainPassword: domainPwd,
	}, nil
}

func GetSecretResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *sysprep.Sysprep) ([]ctrlclient.Object, error) {

	uniqueSecrets := map[string]struct{}{}
	var result []ctrlclient.Object

	captureSecret := func(s ctrlclient.Object, name string) {
		// Only return the secret if it has not already been captured.
		if _, ok := uniqueSecrets[name]; !ok {
			result = append(result, s)
			uniqueSecrets[name] = struct{}{}
		}
	}

	if in.UserData.ProductID != nil {
		s, err := util.GetSecretResource(
			ctx,
			k8sClient,
			secretNamespace,
			in.UserData.ProductID.Name)
		if err != nil {
			return nil, err
		}
		captureSecret(s, in.UserData.ProductID.Name)
	}

	if guiUnattended := in.GUIUnattended; guiUnattended != nil && guiUnattended.AutoLogon {
		s, err := util.GetSecretResource(
			ctx,
			k8sClient,
			secretNamespace,
			guiUnattended.Password.Name)
		if err != nil {
			return nil, err
		}
		captureSecret(s, guiUnattended.Password.Name)
	}

	if identification := in.Identification; identification != nil {
		if dap := identification.DomainAdminPassword; dap != nil && dap.Name != "" {
			s, err := util.GetSecretResource(
				ctx,
				k8sClient,
				secretNamespace,
				dap.Name)
			if err != nil {
				return nil, err
			}
			captureSecret(s, dap.Name)
		}
	}

	return result, nil
}
