// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
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

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error(err, "Failed to get Kubernetes in-cluster config")
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	if err := vmopv1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add vm-operator v1alpha3 scheme")
		os.Exit(1)
	}
	// NOTE: In v1a1 this CRD has a different name - WebConsoleRequest - so this
	// is still required until we stop supporting v1a1.
	if err := vmopv1a1.AddToScheme(scheme); err != nil {
		logger.Error(err, "Failed to add vm-operator v1alpha1 scheme")
		os.Exit(1)
	}

	client, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "Failed to initialize controller-runtime client")
		os.Exit(1)
	}

	server, err := webconsolevalidation.NewServer(
		":"+strconv.Itoa(*serverPort),
		*serverPath,
		client,
	)
	if err != nil {
		logger.Error(err, "Failed to initialize web-console validation server")
		os.Exit(1)
	}

	logger.Info("Starting the web-console validation server", "port", *serverPort, "path", *serverPath)
	if err := server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error(err, "Failed to run the web-console validation server")
		os.Exit(1)
	}
}
