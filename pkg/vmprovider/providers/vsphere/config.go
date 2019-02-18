/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"github.com/golang/glog"
	controllerlib "github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/controller"
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
)

var vSphereVmProviderConfig *VSphereVmProviderConfig = nil

func GetVsphereVmProviderConfig() *VSphereVmProviderConfig {
	if vSphereVmProviderConfig != nil {
		glog.Info("Returning vSphere config")
		return vSphereVmProviderConfig
	}

	glog.Fatalf("vSphere config has not been set")

	return nil
}

func SetVSphereVmProviderConfig(clientSet *kubernetes.Clientset) error {
	// Get the vSphere Provider ConfigMap from the API Master.
	if clientSet == nil {
		// The Caller didn't provide a ClientSet. Thus, a default ClientSet
		// is generated from the Kubernetes config provided to this container
		// through environment variables.
		config, err := controllerlib.GetConfig("")
		if err != nil {
			glog.Fatalf("Could not retrieve the default Kubernetes config: %v", err)
		}

		clientSet = kubernetes.NewForConfigOrDie(config)

		glog.Info("Setting vSphereVmProviderConfig using the default Kubernetes config")
	} else {
		glog.Info("Setting vSphereVmProviderConfig using the provided Kubernetes config")
	}

	vSphereConfig, err := clientSet.CoreV1().ConfigMaps(vSphereConfigK8sNamespace).Get(vSphereConfigMapName, metav1.GetOptions{})

	if err != nil {
		glog.Fatalf("Could not retrieve %v configMap from API Master: %v", vSphereConfigMapName, err)
	}

	vSphereConfigData := vSphereConfig.Data

	vSphereVmProviderConfig = &VSphereVmProviderConfig{
		VcUser:       vSphereConfigData["VcUser"],
		VcPassword:   vSphereConfigData["VcPassword"],
		VcIP:         vSphereConfigData["VcIP"],
		VcUrl:        vSphereConfigData["VcUrl"],
		Datacenter:   vSphereConfigData["Datacenter"],
		ResourcePool: vSphereConfigData["ResourcePool"],
		Folder:       vSphereConfigData["Folder"],
		Datastore:    vSphereConfigData["Datastore"],
	}

	return err
}
