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
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type SecretData struct {
	Password   string
	ScriptText string
}

func GetLinuxPrepSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	linuxPrep *vmopv1.VirtualMachineBootstrapLinuxPrepSpec) (SecretData, error) {

	data := SecretData{}

	if pw := linuxPrep.Password; pw != nil {
		key := ctrlclient.ObjectKey{Name: pw.Name, Namespace: secretNamespace}
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, key, secret); err != nil {
			return SecretData{}, fmt.Errorf("failed to get secret for password: %w", err)
		}

		password, ok := secret.Data[pw.Key]
		if !ok {
			err := fmt.Errorf("secret %s does not contain required password key %q", key.Name, pw.Key)
			return SecretData{}, err
		}

		data.Password = string(password)
	}

	if st := linuxPrep.ScriptText; st != nil {
		if st.From != nil {
			key := ctrlclient.ObjectKey{Name: st.From.Name, Namespace: secretNamespace}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, key, secret); err != nil {
				return SecretData{}, fmt.Errorf("failed to get secret for script text: %w", err)
			}

			text, ok := secret.Data[st.From.Key]
			if !ok {
				err := fmt.Errorf("secret %q does not contain required script text key %q", key, st.From.Key)
				return SecretData{}, err
			}

			data.ScriptText = string(text)
		} else {
			data.ScriptText = ptr.DerefWithDefault(st.Value, "")
		}
	}

	return data, nil
}
