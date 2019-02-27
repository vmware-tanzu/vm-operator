/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	_ "github.com/go-openapi/loads"
	"github.com/golang/glog"
	"net/http"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/rest"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"

	// Make sure dep tools picks up these dependencies
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/cmd/server"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth

	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/openapi"
)

func runHealthServer() {
	// Setup health check handler and corresponding listener on a custom port
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("ok"))
		if err != nil {
			glog.Fatalf("ResponseWriter error: %s", err)
		}
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		glog.Fatalf("ListenAndServe error: %s", err)
	}
}

func main() {
	version := "v0"

	// Init the vsphere provider
	if err := vsphere.InitProvider(nil); err != nil {
		glog.Fatalf("Failed to initialize vSphere provider: %s", err)
	}

	// Get a vmprovider instance
	vmprovider, err := vmprovider.NewVmProvider()
	if err != nil {
		glog.Fatalf("Failed to find vmprovider: %s", err)
	}

	// Provide the vm provider interface to the custom REST implementations
	err = v1alpha1.RegisterRestProvider(rest.NewVirtualMachineImagesREST(vmprovider))
	if err != nil {
		glog.Fatalf("Failed to register REST provider: %s", err)
	}

	go runHealthServer()

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
