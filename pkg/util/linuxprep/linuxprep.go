// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package linuxprep

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type SecretData struct {
	Password   string
	ScriptText string
}

func getLinuxPrepSecretsImpl(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *vmopv1.VirtualMachineBootstrapLinuxPrepSpec,
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

	if pw := in.Password; pw != nil {
		if err := getSecret(pw.Name, pw.Key, &secretData.Password); err != nil {
			return SecretData{}, nil, err
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

func GetLinuxPrepSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	linuxPrep *vmopv1.VirtualMachineBootstrapLinuxPrepSpec) (SecretData, error) {

	secretData, _, err := getLinuxPrepSecretsImpl(ctx, k8sClient, secretNamespace, linuxPrep, true)
	return secretData, err
}

func GetSecretResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in *vmopv1.VirtualMachineBootstrapLinuxPrepSpec) ([]ctrlclient.Object, error) {

	_, secrets, err := getLinuxPrepSecretsImpl(ctx, k8sClient, secretNamespace, in, false)
	if err != nil {
		return nil, err
	}

	objs := make([]ctrlclient.Object, 0, len(secrets))
	for _, s := range secrets {
		objs = append(objs, s)
	}

	return objs, nil
}
