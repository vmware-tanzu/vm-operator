/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
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
	VcUser     string
	VcPassword string
	VcPNID     string
	VcPort     string

	Datacenter   string
	ResourcePool string
	Folder       string
	Datastore    string
}

const (
	vSphereConfigMapName = "vsphere.provider.config.vmoperator.vmware.com"

	vcUserKey       = "VcUser"
	vcPasswordKey   = "VcPassword"
	vcPNIDKey       = "VcPNID"
	vcPortKey       = "VcPort"
	datacenterKey   = "Datacenter"
	resourcePoolKey = "ResourcePool"
	folderKey       = "Folder"
	datastoreKey    = "Datastore"
)

func configMapToProviderConfig(configMap *v1.ConfigMap) *VSphereVmProviderConfig {
	dataMap := configMap.Data

	port, ok := dataMap[vcPortKey]
	if !ok {
		port = "443"
	}

	return &VSphereVmProviderConfig{
		VcUser:       dataMap[vcUserKey],
		VcPassword:   dataMap[vcPasswordKey],
		VcPNID:       dataMap[vcPNIDKey],
		VcPort:       port,
		Datacenter:   dataMap[datacenterKey],
		ResourcePool: dataMap[resourcePoolKey],
		Folder:       dataMap[folderKey],
		Datastore:    dataMap[datastoreKey],
	}
}

func providerConfigToConfigMap(namespace string, config VSphereVmProviderConfig) *v1.ConfigMap {
	dataMap := make(map[string]string)

	dataMap[vcUserKey] = config.VcUser
	dataMap[vcPasswordKey] = config.VcPassword
	dataMap[vcPNIDKey] = config.VcPNID
	dataMap[vcPortKey] = config.VcPort
	dataMap[datacenterKey] = config.Datacenter
	dataMap[resourcePoolKey] = config.ResourcePool
	dataMap[folderKey] = config.Folder
	dataMap[datastoreKey] = config.Datastore

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vSphereConfigMapName,
			Namespace: namespace,
		},
		Data: dataMap,
	}
}

// GetProviderConfigFromConfigMap gets the vSphere Provider ConfigMap from the API Master.
func GetProviderConfigFromConfigMap(clientSet *kubernetes.Clientset, namespace string) (*VSphereVmProviderConfig, error) {
	vSphereConfig, err := clientSet.CoreV1().ConfigMaps(namespace).Get(vSphereConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get provider ConfigMap %v/%v", namespace, vSphereConfigMapName)
	}

	return configMapToProviderConfig(vSphereConfig), nil
}

// Install the Config Map for the VM operator in the API master
func InstallVSphereVmProviderConfig(clientSet *kubernetes.Clientset, namespace string, config VSphereVmProviderConfig) error {
	configMap := providerConfigToConfigMap(namespace, config)
	_, err := clientSet.CoreV1().ConfigMaps(namespace).Update(configMap)
	return err
}
