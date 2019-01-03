
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package main

import (
	_ "github.com/go-openapi/loads"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"

	// Make sure dep tools picks up these dependencies
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/cmd/server"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth

	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/openapi"
)

func main() {
	version := "v0"

	// Init the vsphere provider
	vsphere.InitProvider()

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
