// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package cloudinit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/validate"
)

// cloudConfig provides support for marshalling the object to a valid
// CloudConfig YAML document. The type from the VM Operator APIs cannot be
// marshalled as-is due to the fields that reference external, Secret resources
// for sensitive data. The data from these Secret resources must be fetched
// prior to marshalling the structure to YAML and set inline within the struct
// itself.
type cloudConfig struct {
	Timezone   string              `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Users      *cloudConfigUsers   `json:"users,omitempty" yaml:"users,omitempty"`
	RunCmd     []cloudConfigRunCmd `json:"runcmd,omitempty" yaml:"runcmd,omitempty"`
	WriteFiles []writeFile         `json:"write_files,omitempty" yaml:"write_files,omitempty"`
	SSHPwdAuth *bool               `json:"ssh_pwauth,omitempty" yaml:"ssh_pwauth,omitempty"`
}

type cloudConfigUsers struct {
	defaultUser bool
	users       []user
}

type cloudConfigRunCmd struct {
	singleString  string
	listOfStrings []string
}

type user struct {
	CreateGroups      *bool    `json:"create_groups,omitempty" yaml:"create_groups,omitempty"`
	ExpireDate        *string  `json:"expiredate,omitempty" yaml:"expiredate,omitempty"`
	Gecos             *string  `json:"gecos,omitempty" yaml:"gecos,omitempty"`
	Groups            []string `json:"groups,omitempty" yaml:"groups,omitempty"`
	HashedPasswd      string   `json:"hashed_passwd,omitempty" yaml:"hashed_passwd,omitempty"`
	Homedir           *string  `json:"homedir,omitempty" yaml:"homedir,omitempty"`
	Inactive          *string  `json:"inactive,omitempty" yaml:"inactive,omitempty"`
	LockPasswd        *bool    `json:"lock_passwd,omitempty" yaml:"lock_passwd,omitempty"`
	Name              string   `json:"name" yaml:"name"`
	NoCreateHome      *bool    `json:"no_create_home,omitempty" yaml:"no_create_home,omitempty"`
	NoLogInit         *bool    `json:"no_log_init,omitempty" yaml:"no_log_init,omitempty"`
	NoUserGroup       *bool    `json:"no_user_group,omitempty" yaml:"no_user_group,omitempty"`
	Passwd            string   `json:"passwd,omitempty" yaml:"passwd,omitempty"`
	PrimaryGroup      *string  `json:"primary_group,omitempty" yaml:"primary_group,omitempty"`
	SELinuxUser       *string  `json:"selinux_user,omitempty" yaml:"selinux_user,omitempty"`
	Shell             *string  `json:"shell,omitempty" yaml:"shell,omitempty"`
	SnapUser          *string  `json:"snapuser,omitempty" yaml:"snapuser,omitempty"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty" yaml:"ssh_authorized_keys,omitempty"`
	SSHImportID       []string `json:"ssh_import_id,omitempty" yaml:"ssh_import_id,omitempty"`
	SSHRedirectUser   *bool    `json:"ssh_redirect_user,omitempty" yaml:"ssh_redirect_user,omitempty"`
	Sudo              *string  `json:"sudo,omitempty" yaml:"sudo,omitempty"`
	System            *bool    `json:"system,omitempty" yaml:"system,omitempty"`
	UID               *int64   `json:"uid,omitempty" yaml:"uid,omitempty"`
}

type writeFile struct {
	Append      bool   `json:"append,omitempty" yaml:"append,omitempty"`
	Content     string `json:"content,omitempty" yaml:"content,omitempty"`
	Defer       bool   `json:"defer,omitempty" yaml:"defer,omitempty"`
	Encoding    string `json:"encoding,omitempty" yaml:"encoding,omitempty"`
	Owner       string `json:"owner,omitempty" yaml:"owner,omitempty"`
	Path        string `json:"path" yaml:"path"`
	Permissions string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

const emptyYAMLObject = "{}\n"

// MarshalYAML marshals the provided CloudConfig and secret data to a valid,
// YAML CloudConfig document.
func MarshalYAML(
	in cloudinit.CloudConfig,
	secret CloudConfigSecretData) (string, error) {

	out := cloudConfig{Timezone: in.Timezone}

	if l := len(in.Users); l > 0 {
		out.Users = &cloudConfigUsers{
			defaultUser: in.DefaultUserEnabled,
			users:       make([]user, l),
		}
		for i := range in.Users {
			copyUser(
				in.Users[i],
				&out.Users.users[i],
				secret.Users[in.Users[i].Name])
		}
	}

	if l := len(in.RunCmd); l > 0 {
		var rawCommands []json.RawMessage
		if err := json.Unmarshal(in.RunCmd, &rawCommands); err != nil {
			return "", err
		}

		out.RunCmd = make([]cloudConfigRunCmd, len(rawCommands))
		for i := range rawCommands {

			// First try to unmarshal the value into a string. If that does
			// not work, try unmarshaling the data into a list of strings.
			if err := json.Unmarshal(
				rawCommands[i],
				&out.RunCmd[i].singleString); err != nil {

				out.RunCmd[i].singleString = ""

				if err := json.Unmarshal(
					rawCommands[i],
					&out.RunCmd[i].listOfStrings); err != nil {

					return "", err

				}
			}
		}
	}

	if l := len(in.WriteFiles); l > 0 {
		out.WriteFiles = make([]writeFile, l)

		for i := range in.WriteFiles {

			// If the content was not derived from a secret, then get it as
			// a string from the Content field.
			content := secret.WriteFiles[in.WriteFiles[i].Path]
			if content == "" {
				inContent := in.WriteFiles[i].Content
				if len(inContent) == 0 {
					inContent = []byte(`""`)
				}
				if err := json.Unmarshal(inContent, &content); err != nil {
					return "", err
				}
			}

			copyWriteFile(
				in.WriteFiles[i],
				&out.WriteFiles[i],
				content)
		}
	}

	if in.SSHPwdAuth != nil {
		out.SSHPwdAuth = in.SSHPwdAuth
	}

	var w1 bytes.Buffer
	fmt.Fprintln(&w1, "## template: jinja")
	fmt.Fprintln(&w1, "#cloud-config")
	fmt.Fprintln(&w1, "")

	w2, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}

	data := string(w2)
	if data == emptyYAMLObject {
		return "", nil
	}

	if _, err := w1.WriteString(data); err != nil {
		return "", err
	}

	data = w1.String()

	// Validate the produced CloudConfig YAML using the CloudConfig schema.
	if err := validate.CloudConfigYAML(data); err != nil {
		return "", err
	}

	return data, nil
}

func copyUser(
	in cloudinit.User,
	out *user,
	secret CloudConfigUserSecretData) {

	out.CreateGroups = in.CreateGroups
	out.ExpireDate = in.ExpireDate
	out.Gecos = in.Gecos

	if l := len(in.Groups); l > 0 {
		out.Groups = make([]string, l)
		copy(out.Groups, in.Groups)
	}

	out.HashedPasswd = secret.HashPasswd
	out.Homedir = in.Homedir

	if v := in.Inactive; v != nil {
		s := strconv.Itoa(int(*v))
		out.Inactive = &s
	}

	out.LockPasswd = in.LockPasswd
	out.Name = in.Name
	out.NoCreateHome = in.NoCreateHome
	out.NoLogInit = in.NoLogInit
	out.NoUserGroup = in.NoUserGroup
	out.Passwd = secret.Passwd
	out.PrimaryGroup = in.PrimaryGroup
	out.SELinuxUser = in.SELinuxUser
	out.Shell = in.Shell
	out.SnapUser = in.SnapUser

	if l := len(in.SSHAuthorizedKeys); l > 0 {
		out.SSHAuthorizedKeys = make([]string, l)
		copy(out.SSHAuthorizedKeys, in.SSHAuthorizedKeys)
	}

	if l := len(in.SSHImportID); l > 0 {
		out.SSHImportID = make([]string, l)
		copy(out.SSHImportID, in.SSHImportID)
	}

	out.SSHRedirectUser = in.SSHRedirectUser
	out.Sudo = in.Sudo
	out.System = in.System
	out.UID = in.UID
}

func copyWriteFile(
	in cloudinit.WriteFile,
	out *writeFile,
	content string) {

	out.Append = in.Append
	out.Content = content
	out.Defer = in.Defer
	out.Encoding = string(in.Encoding)
	out.Owner = in.Owner
	out.Path = in.Path
	out.Permissions = in.Permissions
}

func (ccu cloudConfigUsers) MarshalJSON() ([]byte, error) {
	if len(ccu.users) == 0 {
		return nil, nil
	}
	var result []any //nolint:prealloc
	if ccu.defaultUser {
		result = append(result, "default")
	}
	for i := range ccu.users {
		result = append(result, ccu.users[i])
	}
	return json.Marshal(result)
}

func (ccu cloudConfigRunCmd) MarshalJSON() ([]byte, error) {
	if ccu.singleString != "" {
		return json.Marshal(ccu.singleString)
	} else if len(ccu.listOfStrings) > 0 {
		return json.Marshal(ccu.listOfStrings)
	}
	return nil, nil
}

// GetSecretResources returns a list of the Secret resources referenced by the
// provided CloudConfig.
func GetSecretResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	secretNamespace string,
	in cloudinit.CloudConfig) ([]ctrlclient.Object, error) {

	uniqueSecrets := map[string]struct{}{}
	var result []ctrlclient.Object

	captureSecret := func(s ctrlclient.Object, name string) {
		// Only return the secret if it has not already been captured.
		if _, ok := uniqueSecrets[name]; !ok {
			result = append(result, s)
			uniqueSecrets[name] = struct{}{}
		}
	}

	for i := range in.Users {
		if v := in.Users[i].HashedPasswd; v != nil {
			s, err := util.GetSecretResource(
				ctx,
				k8sClient,
				secretNamespace,
				v.Name)
			if err != nil {
				return nil, err
			}
			captureSecret(s, v.Name)
		}
		if v := in.Users[i].Passwd; v != nil {
			s, err := util.GetSecretResource(
				ctx,
				k8sClient,
				secretNamespace,
				v.Name)
			if err != nil {
				return nil, err
			}
			captureSecret(s, v.Name)
		}
	}

	for i := range in.WriteFiles {
		if v := in.WriteFiles[i].Content; len(v) > 0 {
			var sks common.SecretKeySelector
			if err := yaml.Unmarshal(v, &sks); err == nil {
				s, err := util.GetSecretResource(
					ctx,
					k8sClient,
					secretNamespace,
					sks.Name)
				if err != nil {
					return nil, err
				}
				captureSecret(s, sks.Name)
			}
		}
	}

	return result, nil
}
