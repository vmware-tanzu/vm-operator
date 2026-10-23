// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vmware-tanzu/vm-operator/test/coverage"
)

func exercise() string {
	if os.Getenv("EXERCISE_ALL") == "true" {
		return "both branches"
	}
	return "one branch"
}

func main() {
	server, err := coverage.Start(context.Background())
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(exercise())
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM, syscall.SIGINT)
	<-done
	_ = server.Close()
}
