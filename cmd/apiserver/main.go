/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"k8s.io/klog/klogr"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/cmd/server"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
)

var (
	apiVersion = "v0"
)

func main() {
	logf.SetLogger(klogr.New())
	log := logf.Log.WithName("apiserver-entrypoint")

	log.Info("Starting vm-operator apiserver", "version", pkg.BuildVersion,
		"buildnumber", pkg.BuildNumber, "buildtype", pkg.BuildType)

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", apiVersion)
}
