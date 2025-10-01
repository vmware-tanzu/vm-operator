// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package sysprep

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type SecretData struct {
	ProductID, Password, DomainPassword, ScriptText string
}

func getSysprepSecretsImpl(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *sysprep.Sysprep,
	setData bool) (SecretData, map[string]*corev1.Secret, error) {

	secretData := SecretData{}
	secrets := map[string]*corev1.Secret{}

	getSecret := func(name, key string, outData *string) error {
		if name == "" {
			return nil
		}

		// Our ctrl-runtime client does not cache secrets, so avoid repeated
		// fetches of the same secret.
		secret, ok := secrets[name]
		if !ok {
			var err error
			secret, err = util.GetSecretResource(ctx, k8sClient, secretNamespace, name)
			if err != nil {
				return err
			}
			secrets[name] = secret
		}

		if setData {
			data := secret.Data[key]
			if len(data) == 0 {
				return fmt.Errorf("no data found for key %q for secret %s/%s", key, secretNamespace, name)
			}
			*outData = string(data)
		}

		return nil
	}

	if productID := in.UserData.ProductID; productID != nil {
		if err := getSecret(productID.Name, productID.Key, &secretData.ProductID); err != nil {
			return SecretData{}, nil, err
		}
	}

	if guiUnattended := in.GUIUnattended; guiUnattended != nil {
		if pw := guiUnattended.Password; pw != nil {
			if err := getSecret(pw.Name, pw.Key, &secretData.Password); err != nil {
				return SecretData{}, nil, err
			}
		}
	}

	if identification := in.Identification; identification != nil {
		if dap := identification.DomainAdminPassword; dap != nil {
			if err := getSecret(dap.Name, dap.Key, &secretData.DomainPassword); err != nil {
				return SecretData{}, nil, err
			}
		}
	}

	if scriptText := in.ScriptText; scriptText != nil {
		if scriptText.From != nil {
			if err := getSecret(scriptText.From.Name, scriptText.From.Key, &secretData.ScriptText); err != nil {
				return SecretData{}, nil, err
			}
		} else if setData {
			secretData.ScriptText = ptr.DerefWithDefault(scriptText.Value, "")
		}
	}

	return secretData, secrets, nil
}

func GetSysprepSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *sysprep.Sysprep) (SecretData, error) {

	secretData, _, err := getSysprepSecretsImpl(ctx, k8sClient, secretNamespace, in, true)
	return secretData, err
}

func GetSecretResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *sysprep.Sysprep) ([]ctrlclient.Object, error) {

	_, secrets, err := getSysprepSecretsImpl(ctx, k8sClient, secretNamespace, in, false)
	if err != nil {
		return nil, err
	}

	objs := make([]ctrlclient.Object, 0, len(secrets))
	for _, s := range secrets {
		objs = append(objs, s)
	}
	return objs, nil
}
