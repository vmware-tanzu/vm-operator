/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package integration

import (
	"strconv"

	"github.com/golang/glog"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/client-go/kubernetes"
)

const DefaultNamespace = "default"

// Support for bootstrapping VM operator resource requirements in Kubernetes.
// Generate a fake vsphere provider config that is suitable for the integration test environment.
// Post the resultant config map to the API Master for consumption by the VM operator
func InstallVmOperatorConfig(clientSet *kubernetes.Clientset, vcAddress string, vcPort int) error {
	glog.Infof("Installing a bootstrap config map for use in integration tests.")
	return vsphere.InstallVSphereVmProviderConfig(clientSet, DefaultNamespace, *NewIntegrationVmOperatorConfig(vcAddress, vcPort))
}

func NewIntegrationVmOperatorConfig(vcAddress string, vcPort int) *vsphere.VSphereVmProviderConfig {
	// Configure for vcsim by default

	return &vsphere.VSphereVmProviderConfig{
		VcPNID:           vcAddress,
		VcPort:           strconv.Itoa(vcPort),
		VcAuthSecretName: "vmop-test-integration-auth",
		Datacenter:       "/DC0",
		ResourcePool:     "/DC0/host/DC0_C0/Resources",
		Folder:           "/DC0/vm",
		Datastore:        "/DC0/datastore/LocalDS_0",
	}
}

func NewIntegrationVmOperatorCredentials() *vsphere.VSphereVmProviderCredentials {

	// User and Pass can be anything for vcsim
	return &vsphere.VSphereVmProviderCredentials{
		Username: "Administrator@vsphere.local",
		Password: "Admin!23",
	}

}
