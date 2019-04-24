/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"github.com/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"io"
	"net/http"

	_ "github.com/go-openapi/loads"
	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/rest"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"

	// Make sure dep tools picks up these dependencies
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/cmd/server"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth
	clientRest "k8s.io/client-go/rest"
)

func runHealthServer() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	if err := http.ListenAndServe(":49200", nil); err != nil {
		glog.Fatalf("ListenAndServe error: %v", err)
	}
}

// Assume this will always be our provider.
func registerVsphereVmProvider() error {
	restConfig, err := clientRest.InClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get rest client config")
	}
	clientSet := kubernetes.NewForConfigOrDie(restConfig)

	provider, err := vsphere.NewVSphereVmProvider(clientSet)
	if err != nil {
		return err
	}

	vmprovider.RegisterVmProvider(provider)
	return nil
}

func main() {
	version := "v0"

	if err := registerVsphereVmProvider(); err != nil {
		glog.Fatalf("Failed to register vSphere VM provider: %v", err)
	}

	// Use the registered VM provider in the custom REST implementations.
	provider := vmprovider.GetVmProviderOrDie()
	if err := v1alpha1.RegisterRestProvider(rest.NewVirtualMachineImagesREST(provider)); err != nil {
		glog.Fatalf("Failed to register REST provider: %v", err)
	}

	go runHealthServer()

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
