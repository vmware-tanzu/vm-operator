// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

// Key is used to store/retrieve information in/from a VM Operator context.
type Key uint

const (
	// MaxDeployThreadsContextKey is the context key that stores the maximum
	// number of threads allowed used to deploy a VM.
	MaxDeployThreadsContextKey Key = iota
)
