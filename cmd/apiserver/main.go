/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package main

import (
	"os"

	"k8s.io/klog/klogr"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"

	_ "github.com/go-openapi/loads"
	"github.com/vmware-tanzu/vm-operator/pkg/apis"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/rest"
	"github.com/vmware-tanzu/vm-operator/pkg/openapi"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	// Make sure dep tools picks up these dependencies
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/cmd/server"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth
	clientRest "k8s.io/client-go/rest"
)

var (
	version = "v0"
	log     = klogr.New()
)

func registerVsphereVmProvider() error {
	var restConfig *clientRest.Config

	masterUrl := os.Getenv("KUBERNETES_MASTERURL")
	if masterUrl != "" {
		// Integration test environment.
		restConfig = &clientRest.Config{Host: masterUrl}
	} else {
		var err error
		restConfig, err = clientRest.InClusterConfig()
		if err != nil {
			return errors.Wrap(err, "failed to get rest client config")
		}
	}

	provider, err := vsphere.NewVSphereVmProvider(kubernetes.NewForConfigOrDie(restConfig), ncpclientset.NewForConfigOrDie(restConfig))
	if err != nil {
		return err
	}

	vmprovider.RegisterVmProvider(provider)
	return nil
}

func main() {
	// Assume this will always be our provider.
	if err := registerVsphereVmProvider(); err != nil {
		log.Error(err, "Failed to register vSphere VM provider")
		os.Exit(255)
	}

	// Use the registered VM provider in the custom REST implementations.
	provider := vmprovider.GetVmProviderOrDie()
	if err := vmoperator.RegisterRestProvider(rest.NewVirtualMachineImagesREST(provider)); err != nil {
		log.Error(err, "Failed to register REST provider")
		os.Exit(255)
	}

	server.StartApiServer("/registry/vmware.com", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}
