
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package main

import (
	"flag"
	"log"
	"vmware.com/kubevsphere/pkg/vmprovider"

	controllerlib "github.com/kubernetes-incubator/apiserver-builder/pkg/controller"

	"vmware.com/kubevsphere/pkg/controller"
)

var kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig")

func main() {
	flag.Parse()
	config, err := controllerlib.GetConfig(*kubeconfig)
	if err != nil {
		log.Fatalf("Could not create Config for talking to the apiserver: %v", err)
	}

	// Initialize VM providers
	vmprovider.InitVmProvider("vsphere", "")

	controllers, _ := controller.GetAllControllers(config)
	controllerlib.StartControllerManager(controllers...)

	// Blockforever
	select {}
}
