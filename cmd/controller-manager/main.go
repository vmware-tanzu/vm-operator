/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"
	controllerlib "github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/controller"

	"vmware.com/kubevsphere/pkg/controller"
)

func runHealthServer() {
	// Setup health check handler and corresponding listener on a custom port
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
                _, err := w.Write([]byte("ok"))
                if err != nil {
                        glog.Fatalf("ResponseWriter error: %s", err)
                }
	})
	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		glog.Fatalf("ListenAndServe error: %s", err)
	}
}

func main() {
	// This isn't used
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig")
	flag.Parse()

	config, err := controllerlib.GetConfig(*kubeconfig)
	if err != nil {
		glog.Fatalf("Could not create Config for talking to the apiserver: %v", err)
	}

	controllers, _ := controller.GetAllControllers(config)
	controllerlib.StartControllerManager(controllers...)

	go runHealthServer()

	// Blockforever
	select {}
}
