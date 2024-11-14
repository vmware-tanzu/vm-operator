// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package record

import (
	"context"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type contextValueType = Recorder

// FromContext returns the recorder from the specified context.
func FromContext(ctx context.Context) contextValueType {
	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(val contextValueType) contextValueType {
			return val
		})
}

// WithContext returns a new recorder context.
func WithContext(parent context.Context, val contextValueType) context.Context {
	if val == nil {
		panic("recorder is nil")
	}
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType { return val })
}
