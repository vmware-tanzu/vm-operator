// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validate

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	cloudinitschema "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/schema"
)

const (
	invalidRunCmd           = "value must be a list"
	invalidRunCmdElement    = "value must be a string or list of strings"
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
	in json.RawMessage) field.ErrorList {

	if len(in) == 0 {
		return nil
	}

	var allErrs field.ErrorList

	fieldPath = fieldPath.Child("runcmds")

	var rawCommands []json.RawMessage
	if err := json.Unmarshal(in, &rawCommands); err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				fieldPath,
				string(in),
				invalidRunCmd))
		return allErrs
	}

	for i := range rawCommands {
		fieldPath := fieldPath.Index(i)

		// First try to unmarshal the value into a string. If that does
		// not work, try unmarshaling the data into a list of strings.
		var singleString string
		if err := json.Unmarshal(
			rawCommands[i],
			&singleString); err != nil {

			var listOfStrings []string
			if err := json.Unmarshal(
				rawCommands[i],
				&listOfStrings); err != nil {

				allErrs = append(
					allErrs,
					field.Invalid(
						fieldPath,
						string(rawCommands[i]),
						invalidRunCmdElement))
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

		content := in[i].Content
		if len(content) == 0 {
			content = []byte(`""`)
		}

		// First try to unmarshal the value into a string. If that does
		// not work, try unmarshaling the data into a SecretKeySelector object.
		var singleString string
		if err := json.Unmarshal(
			content,
			&singleString); err != nil {

			var sks common.SecretKeySelector
			if err := json.Unmarshal(
				content,
				&sks); err != nil {

				allErrs = append(
					allErrs,
					field.Invalid(
						fieldPath,
						string(content),
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
