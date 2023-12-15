// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validate

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	cloudinitschema "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/schema"
)

const (
	invalidRunCmd           = "value must be a string or list of strings"
	invalidWriteFileContent = "value must be a string, multi-line string, or SecretKeySelector"
)

// CloudConfigJSONRawMessage returns any errors encountered when validating the
// json.RawMessage portions of a CloudConfig.
func CloudConfigJSONRawMessage(
	fieldPath *field.Path,
	in cloudinit.CloudConfig) field.ErrorList {

	var allErrs field.ErrorList

	if fieldPath == nil {
		fieldPath = field.NewPath("cloudConfig")
	} else {
		fieldPath = fieldPath.Child("cloudConfig")
	}

	allErrs = append(allErrs, validateRunCmds(fieldPath, in.RunCmd)...)
	allErrs = append(allErrs, validateWriteFiles(fieldPath, in.WriteFiles)...)

	return allErrs
}

func validateRunCmds(
	fieldPath *field.Path,
	in []json.RawMessage) field.ErrorList {

	var allErrs field.ErrorList

	fieldPath = fieldPath.Child("runcmds")

	for i := range in {
		fieldPath := fieldPath.Index(i)

		// First try to unmarshal the value into a string. If that does
		// not work, try unmarshaling the data into a list of strings.
		var singleString string
		if err := yaml.Unmarshal(
			in[i],
			&singleString); err != nil {

			var listOfStrings []string
			if err := yaml.Unmarshal(
				in[i],
				&listOfStrings); err != nil {

				allErrs = append(
					allErrs,
					field.Invalid(
						fieldPath,
						string(in[i]),
						invalidRunCmd))
			}
		}
	}

	return allErrs
}

func validateWriteFiles(
	fieldPath *field.Path,
	in []cloudinit.WriteFile) field.ErrorList {

	var allErrs field.ErrorList

	fieldPath = fieldPath.Child("write_files")

	for i := range in {
		fieldPath := fieldPath.Key(in[i].Path)

		// First try to unmarshal the value into a string. If that does
		// not work, try unmarshaling the data into a SecretKeySelector object.
		var singleString string
		if err := yaml.Unmarshal(
			in[i].Content,
			&singleString); err != nil {

			var sks common.SecretKeySelector
			if err := yaml.Unmarshal(
				in[i].Content,
				&sks); err != nil {

				allErrs = append(
					allErrs,
					field.Invalid(
						fieldPath,
						string(in[i].Content),
						invalidWriteFileContent))
			}
		}
	}

	return allErrs
}

// CloudConfigYAML returns an error if the provided CloudConfig YAML is not
// valid according to the CloudConfig schema.
//
// Please note the up-to-date schemas related to Cloud-Init may be found at
// https://github.com/canonical/cloud-init/tree/main/cloudinit/config/schemas.
func CloudConfigYAML(in string) error {
	// The cloudinitschema.UnmarshalCloudconfig function only supports JSON
	// input, so first we need to convert the CloudConfig YAML to JSON.
	data := map[string]any{}
	if err := yaml.Unmarshal([]byte(in), &data); err != nil {
		return err
	}
	out, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Validate the JSON CloudConfig.
	if _, err := cloudinitschema.UnmarshalCloudconfig(out); err != nil {
		return err
	}

	return nil
}
