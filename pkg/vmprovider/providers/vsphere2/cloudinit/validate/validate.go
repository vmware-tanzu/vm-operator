// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validate

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
)

const (
	invalidRunCmd           = "value must be a string or list of strings"
	invalidWriteFileContent = "value must be a string, multi-line string, or SecretKeySelector"
)

// CloudConfig returns any errors encountered when validating the raw JSON
// portions of a CloudConfig.
func CloudConfig(
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
