/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"github.com/golang/glog"
	controllerlib "github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/controller"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

/*
 * Configuration for a Vsphere VM Provider instance.  Contains information enabling integration with a backend
 * vSphere instance for VM management.
 */
type VSphereVmProviderConfig struct {
	VcUser       string
	VcPassword   string
	VcIP         string
	VcUrl        string
	Datacenter   string
	ResourcePool string
	Folder       string
	Datastore    string
}

const (
	vSphereConfigMapName      = "vsphere.provider.config.vmoperator.vmware.com"
	vSphereConfigK8sNamespace = "default"

	vcUserKey       = "VcUser"
	vcPasswordKey   = "VcPassword"
	vcIpKey         = "VcIP"
	vcUrlKey        = "VcUrl"
	datacenterKey   = "Datacenter"
	resourcePoolKey = "ResourcePool"
	folderKey       = "Folder"
	datastoreKey    = "Datastore"
)

func configMapToProviderConfig(configMap *v1.ConfigMap) *VSphereVmProviderConfig {
	dataMap := configMap.Data

	return &VSphereVmProviderConfig{
		VcUser:       dataMap[vcUserKey],
		VcPassword:   dataMap[vcPasswordKey],
		VcIP:         dataMap[vcIpKey],
		VcUrl:        dataMap[vcUrlKey],
		Datacenter:   dataMap[datacenterKey],
		ResourcePool: dataMap[resourcePoolKey],
		Folder:       dataMap[folderKey],
		Datastore:    dataMap[datastoreKey],
	}
}

func providerConfigToConfigMap(config VSphereVmProviderConfig) *v1.ConfigMap {
	dataMap := make(map[string]string)

	dataMap[vcUserKey] = config.VcUser
	dataMap[vcPasswordKey] = config.VcPassword
	dataMap[vcIpKey] = config.VcIP
	dataMap[vcUrlKey] = config.VcUrl
	dataMap[datacenterKey] = config.Datacenter
	dataMap[resourcePoolKey] = config.ResourcePool
	dataMap[folderKey] = config.Folder
	dataMap[datastoreKey] = config.Datastore

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: vSphereConfigMapName,
		},
		Data: dataMap,
	}
}

func GetProviderConfigFromConfigMap(clientSet *kubernetes.Clientset) (*VSphereVmProviderConfig, error) {
	if clientSet == nil {
		// Create the Clientset from the Kubernetes config provided to this container
		// through environment variables.
		// TODO(bryanv) Have all callers create their ClientSet.
		config, err := controllerlib.GetConfig("")
		if err != nil {
			glog.Fatalf("Could not retrieve the default Kubernetes config: %v", err)
		}

		clientSet = kubernetes.NewForConfigOrDie(config)
	}

	// Get the vSphere Provider ConfigMap from the API Master.
	vSphereConfig, err := clientSet.CoreV1().ConfigMaps(vSphereConfigK8sNamespace).Get(vSphereConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get provider ConfigMap %q", vSphereConfigMapName)
	}

	return configMapToProviderConfig(vSphereConfig), nil
}

// Install the Config Map for the VM operator in the API master
func InstallVSphereVmProviderConfig(clientSet *kubernetes.Clientset, config VSphereVmProviderConfig) error {
	// TODO: Fix hardcoded namespace
	configMap := providerConfigToConfigMap(config)
	_, err := clientSet.CoreV1().ConfigMaps(vSphereConfigK8sNamespace).Update(configMap)
	return err
}
