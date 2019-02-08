/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	_ "github.com/go-openapi/loads"
	"github.com/golang/glog"
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

func main() {
	version := "v0"

	// Init the vsphere provider
	vsphere.InitProvider()

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

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
