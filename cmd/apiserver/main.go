
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package main

import (
	// Make sure dep tools picks up these dependencies
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "github.com/go-openapi/loads"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/cmd/server"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth

	"vmware.com/kubevsphere/pkg/apis"
	"vmware.com/kubevsphere/pkg/openapi"
)

func main() {
	version := "v0"
	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
