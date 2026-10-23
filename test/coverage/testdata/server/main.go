// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
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
	port := flag.Uint("port", 0, "Loopback port (zero selects an available port)")
	flag.Parse()
	if *port > 65535 {
		os.Exit(1)
	}
	server, err := coverage.Start(context.Background(), uint16(*port))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(server.Addr, exercise())
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM, syscall.SIGINT)
	<-done
	_ = server.Close()
}
