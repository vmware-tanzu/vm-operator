// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman

import (
	"errors"
)

var (
	// ErrMissingNextRotation is returned if the webhook server secret is
	// missing the next-rotation annotation after it should already exist.
	// rotate-epoch annotation after it should already exist.
	ErrMissingNextRotation = errors.New("next-rotation annotation is missing")

	// ErrInvalidNextRotation is returned if the value in the next-rotation
	// annotation cannot be parsed as a UNIX epoch.
	ErrInvalidNextRotation = errors.New("next-rotation annotation value is invalid")

	// ErrSecretNotFound is returned if the webhook server secret does not
	// exist.
	ErrSecretNotFound = errors.New("webhook server certificate should always exist")
)
