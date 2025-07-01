/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"

	"k8s.io/apimachinery/pkg/util/validation"
)

func FormatValue(str string) (string, error) {
	// Check if it is a standard Kubernetes label value
	if len(validation.IsValidLabelValue(str)) == 0 {
		return str, nil
	}

	hasher := fnv.New32a()
	if _, err := hasher.Write([]byte(str)); err != nil {
		// At time of writing the implementation of fnv's Write function can never return an error.
		// If a future Go version changes the implementation, this code panics.
		return "", err
	}

	return fmt.Sprintf("hash_%s_z", base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hasher.Sum(nil))), nil
}

// MustFormatValue returns the passed value if it meets the standards for a
// Kubernetes label value. Otherwise, it returns a hash which meets the
// requirements.
// A copy of the method used by cluster-api for MachineSets.
func MustFormatValue(str string) string {
	val, err := FormatValue(str)
	if err != nil {
		panic(err)
	}

	return val
}
