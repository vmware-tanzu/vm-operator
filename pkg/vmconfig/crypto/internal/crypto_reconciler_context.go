// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type ContextKeyType uint8

const ContextKeyValue ContextKeyType = 0

type State struct {
	Operation string
}

func FromContext(ctx context.Context) State {
	return ctxgen.FromContext(
		ctx,
		ContextKeyValue,
		func(val State) State {
			return val
		})
}

func SetOperation(ctx context.Context, op string) {
	ctxgen.SetContext(
		ctx,
		ContextKeyValue,
		func(val State) State {
			val.Operation = op
			return val
		})
}
