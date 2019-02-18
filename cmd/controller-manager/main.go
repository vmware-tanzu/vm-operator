/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"flag"

	"github.com/golang/glog"
	controllerlib "github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/controller"

	"vmware.com/kubevsphere/pkg/controller"
)

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

	// Blockforever
	select {}
}
