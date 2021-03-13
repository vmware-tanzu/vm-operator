// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package sequence

import (
	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("sequence")

// TODO: Return a compound error to put in status
type SequenceStep interface {
	Name() string
	Execute(ctx context.Context) error
}

type SequenceEngine interface {
	Name() string
	Execute(ctx context.Context) error
}
