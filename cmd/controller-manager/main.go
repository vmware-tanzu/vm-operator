/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"flag"
	"io"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"

	"github.com/golang/glog"
	controllerlib "github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/controller"
	"k8s.io/client-go/rest"

	"vmware.com/kubevsphere/pkg/controller"
)

func runHealthServer() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	if err := http.ListenAndServe(":8081", nil); err != nil {
		glog.Fatalf("ListenAndServe error: %s", err)
	}
}

// Assume this will always be our provider.
func registerVsphereVmProvider(restConfig *rest.Config) error {
	clientSet := kubernetes.NewForConfigOrDie(restConfig)

	provider, err := vsphere.NewVSphereVmProvider(clientSet)
	if err != nil {
		return err
	}

	vmprovider.RegisterVmProvider(provider)
	return nil
}

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig")
	flag.Parse()

	restConfig, err := controllerlib.GetConfig(*kubeconfig)
	if err != nil {
		glog.Fatalf("Failed to get rest client config: %v", err)
	}

	if err := registerVsphereVmProvider(restConfig); err != nil {
		glog.Fatalf("Failed to register vSphere VM provider: %v", err)
	}

	controllers, _ := controller.GetAllControllers(restConfig)
	controllerlib.StartControllerManager(controllers...)

	go runHealthServer()

	// Block forever.
	select {}
}
