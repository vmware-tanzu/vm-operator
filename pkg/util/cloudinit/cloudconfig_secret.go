// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudinit

import (
	"context"
	"encoding/json"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CloudConfigSecretData is used to provide the sensitive data that may have
// been derived from Secret resources to the encoder.
type CloudConfigSecretData struct {
	// Users is a map where the key is the user's Name.
	Users map[string]CloudConfigUserSecretData

	// WriteFiles is a map where the key is the file's Path and the value is
	// the file's contents.
	WriteFiles map[string]string
}

type CloudConfigUserSecretData struct {
	HashPasswd string
	Passwd     string
}

// GetCloudConfigSecretData returns the secret data related to inline CloudInit.
func GetCloudConfigSecretData(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in cloudinit.CloudConfig) (CloudConfigSecretData, error) {

	var result CloudConfigSecretData

	if l := len(in.Users); l > 0 {
		result.Users = map[string]CloudConfigUserSecretData{}
		for i := range in.Users {
			var outUser CloudConfigUserSecretData
			if err := getSecretDataForUser(
				ctx,
				k8sClient,
				secretNamespace,
				in.Users[i],
				&outUser); err != nil {
				return CloudConfigSecretData{}, err
			}
			if outUser.HashPasswd != "" || outUser.Passwd != "" {
				result.Users[in.Users[i].Name] = outUser
			}
		}
	}

	if l := len(in.WriteFiles); l > 0 {
		result.WriteFiles = map[string]string{}
		for i := range in.WriteFiles {
			var outContent string
			if err := getSecretDataForWriteFile(
				ctx,
				k8sClient,
				secretNamespace,
				in.WriteFiles[i],
				&outContent); err != nil {
				return CloudConfigSecretData{}, err
			}
			if outContent != "" {
				result.WriteFiles[in.WriteFiles[i].Path] = outContent
			}
		}
	}

	return result, nil
}

func getSecretDataForUser(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in cloudinit.User,
	out *CloudConfigUserSecretData) error {

	if in.HashedPasswd != nil && in.Passwd != nil {
		return fmt.Errorf(
			"cloud config specified both hashed_passwd and passwd for user %q",
			in.Name)
	}

	if v := in.HashedPasswd; v != nil {
		if err := util.GetSecretData(
			ctx, k8sClient,
			secretNamespace, v.Name, v.Key,
			&out.HashPasswd); err != nil {

			return err
		}
	}
	if v := in.Passwd; v != nil {
		if err := util.GetSecretData(
			ctx, k8sClient,
			secretNamespace, v.Name, v.Key,
			&out.Passwd); err != nil {

			return err
		}
	}

	return nil
}

func getSecretDataForWriteFile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in cloudinit.WriteFile,
	out *string) error {

	if len(in.Content) == 0 {
		return nil
	}

	// First try to unmarshal the value into a string. If that does
	// not work, try unmarshaling the data into a SecretKeySelector.
	if err := json.Unmarshal(
		in.Content,
		out); err == nil {

		return nil
	}

	*out = ""
	var sks common.SecretKeySelector
	if err := json.Unmarshal(in.Content, &sks); err != nil {
		return err
	}

	return util.GetSecretData(
		ctx,
		k8sClient,
		secretNamespace,
		sks.Name, sks.Key,
		out)
}
