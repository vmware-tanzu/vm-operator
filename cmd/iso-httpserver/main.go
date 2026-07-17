// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/isohttpserver"
)

var (
	defaultServerPort        = 8080
	defaultServerRoot        = "/data"
	defaultServerBindAddress = ""
)

func init() {
	if v := os.Getenv("SERVER_ROOT"); v != "" {
		defaultServerRoot = v
	}
	if v, err := strconv.Atoi(os.Getenv("SERVER_PORT")); err == nil {
		defaultServerPort = v
	}
	if v := os.Getenv("SERVER_BIND_ADDRESS"); v != "" {
		defaultServerBindAddress = v
	}
}

func main() {
	log.Printf("VM Operator ISO HTTP server info: version=%s buildnumber=%s buildtype=%s commit=%s",
		pkg.BuildVersion, pkg.BuildNumber, pkg.BuildType, pkg.BuildCommit)

	serverPort := flag.Int(
		"server-port",
		defaultServerPort,
		"The port on which the iso-httpserver listens for incoming requests.",
	)
	serverRoot := flag.String(
		"server-root",
		defaultServerRoot,
		"The root directory of files to serve.",
	)
	serverBindAddress := flag.String(
		"server-bind-address",
		defaultServerBindAddress,
		"The IP address to bind to.",
	)

	flag.Parse()

	addr := *serverBindAddress + ":" + strconv.Itoa(*serverPort)
	server, err := isohttpserver.NewServer(addr, *serverRoot)
	if err != nil {
		log.Fatalf("Failed to initialize iso-httpserver: %v", err)
	}

	log.Printf("Starting iso-httpserver: addr=%s root=%s", addr, *serverRoot)
	if err := server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to run iso-httpserver: %v", err)
	}
}
