/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package sequence

import "context"

// TODO: Return a compound error to put in status
type SequenceStep interface {
	Name() string
	Execute(ctx context.Context) error
}

type SequenceEngine interface {
	Name() string
	Execute(ctx context.Context) error
}
