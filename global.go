/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package kubevsphere

import (
	"path/filepath"
	"runtime"
)

var (
	// Acquire pathname to current directory
	_, b, _, _ = runtime.Caller(0)
	Rootpath   = filepath.Dir(b)
)
