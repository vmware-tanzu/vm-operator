/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"k8s.io/klog/klogr"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/cmd/server"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
)

var (
	apiVersion = "v0"
	log        = klogr.New()
)

func main() {
	log.Info("Starting vm-operator apiserver", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType)

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", apiVersion)
}
