// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"net/http"
	"os"
	"strconv"

	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/webconsolevalidation"
)

var (
	defaultServerPort = 9868
	defaultServerPath = "/validate"
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
	ctrllog.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))
	logger := ctrllog.Log.WithName("entrypoint")

	logger.Info("VM Operator web-console validation server info", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType, "commit", pkg.BuildCommit)

	serverPort := flag.Int(
		"server-port",
		defaultServerPort,
		"The port on which web-console validation server to listen for incoming requests.",
	)
	serverPath := flag.String(
		"server-path",
		defaultServerPath,
		"The pattern path to handle the web-console validation requests.",
	)

	flag.Parse()

	if initErr := webconsolevalidation.InitServer(); initErr != nil {
		logger.Error(initErr, "Failed to initialize web-console validation server")
		os.Exit(1)
	}

	logger.Info("Starting the web-console validation server", "port", *serverPort, "path", *serverPath)

	// Pass serverPath to the RunServer so one can check what path the server is listening on
	// by looking at the commands specified in the server deployment spec.
	runErr := webconsolevalidation.RunServer(":"+strconv.Itoa(*serverPort), *serverPath)
	if runErr != nil && runErr != http.ErrServerClosed {
		logger.Error(runErr, "Error occurred while running the web-console validation server!")
	}
}
