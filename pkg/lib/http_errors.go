// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package lib

import (
	"net/http"
	"strings"
)

func IsNotFoundError(err error) bool {
	return strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}

func IsUnAuthorizedError(err error) bool {
	return strings.HasSuffix(err.Error(), http.StatusText(http.StatusUnauthorized))
}
