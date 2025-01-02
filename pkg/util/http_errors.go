// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"net/http"
	"strings"
)

func IsNotFoundError(err error) bool {
	return strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}
