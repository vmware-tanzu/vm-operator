// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package exit

import (
	"os"
)

// Exit is used in testing to assert the capability controller exits the process
// when the capabilities have changed.
var Exit func()

func init() {
	Exit = func() { os.Exit(1) }
}
