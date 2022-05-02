// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"net/http"
	"os"
	"strconv"

	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/hostvalidation"
)

var (
	defaultServerPort = 9868
	defaultServerPath = "/hostvalidation"
)

func init() {
	if v := os.Getenv("SERVER_PATH"); v != "" {
		defaultServerPath = v
	}
	if v, err := strconv.Atoi(os.Getenv("SERVER_PORT")); err == nil {
		defaultServerPort = v
	}
}

func main() {
	// Using the same type of logger as in the controller-manager.
	klog.InitFlags(nil)
	ctrllog.SetLogger(klogr.New())
	setupLog := ctrllog.Log.WithName("entrypoint")

	setupLog.Info("VM Operator host validation server info", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType, "commit", pkg.BuildCommit)

	serverPort := flag.Int(
		"server-port",
		defaultServerPort,
		"The port on which host validation server to listen for incoming requests.",
	)
	serverPath := flag.String(
		"server-path",
		defaultServerPath,
		"The pattern path to handle the host validation requests.",
	)
	flag.Parse()

	setupLog.Info("Starting the host validation server", "port", *serverPort, "path", *serverPath)
	err := hostvalidation.RunServer(":"+strconv.Itoa(*serverPort), *serverPath)
	if err != nil && err != http.ErrServerClosed {
		setupLog.Error(err, "Error occurred while running the host validation server!")
	}
}
