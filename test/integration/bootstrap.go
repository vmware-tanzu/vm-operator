/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package integration

import (
	"fmt"
	"strconv"

	"k8s.io/klog"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/client-go/kubernetes"
)

const DefaultNamespace = "default"
const SecretName = "vmop-test-integration-auth"

type VSphereVmProviderTestConfig struct {
	VcCredsSecretName string
	*vsphere.VSphereVmProviderConfig
}

// Support for bootstrapping VM operator resource requirements in Kubernetes.
// Generate a fake vsphere provider config that is suitable for the integration test environment.
// Post the resultant config map to the API Master for consumption by the VM operator
func InstallVmOperatorConfig(clientSet *kubernetes.Clientset, vcAddress string, vcPort int) error {
	klog.Infof("Installing a bootstrap config map for use in integration tests.")
	return vsphere.InstallVSphereVmProviderConfig(clientSet, DefaultNamespace, NewIntegrationVmOperatorConfig(vcAddress, vcPort), SecretName)
}

func NewIntegrationVmOperatorConfig(vcAddress string, vcPort int) *vsphere.VSphereVmProviderConfig {
	return &vsphere.VSphereVmProviderConfig{
		VcPNID:       vcAddress,
		VcPort:       strconv.Itoa(vcPort),
		VcCreds:      NewIntegrationVmOperatorCredentials(),
		Datacenter:   "/DC0",
		ResourcePool: "/DC0/host/DC0_C0/Resources",
		Folder:       "/DC0/vm",
		Datastore:    "/DC0/datastore/LocalDS_0",
	}
}

func NewIntegrationVmOperatorCredentials() *vsphere.VSphereVmProviderCredentials {
	// User and password can be anything for vcSim
	return &vsphere.VSphereVmProviderCredentials{
		Username: "Administrator@vsphere.local",
		Password: "Admin!23",
	}
}

func SetupEnv() (func(), error) {
	vcSim := NewVcSimInstance()
	address, port := vcSim.Start()

	config := NewIntegrationVmOperatorConfig(address, port)
	provider, err := vsphere.NewVSphereVmProviderFromConfig(DefaultNamespace, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vSphere provider: %v", err)
	}
	vmprovider.RegisterVmProvider(provider)

	cleanup := func() { vcSim.Stop() }

	return cleanup, nil
}
