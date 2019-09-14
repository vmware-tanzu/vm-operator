/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"os"

	"k8s.io/klog"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

/*
 * Configuration for a Vsphere VM Provider instance.  Contains information enabling integration with a backend
 * vSphere instance for VM management.
 */
type VSphereVmProviderConfig struct {
	VcPNID        string
	VcPort        string
	VcCreds       *VSphereVmProviderCredentials
	Datacenter    string
	ResourcePool  string
	Folder        string
	Datastore     string
	ContentSource string
	Network       string
}

const (
	VmopNamespaceEnv     = "POD_NAMESPACE"
	VSphereConfigMapName = "vsphere.provider.config.vmoperator.vmware.com"

	vcPNIDKey            = "VcPNID"
	vcPortKey            = "VcPort"
	vcCredsSecretNameKey = "VcCredsSecretName" // nolint:gosec
	datacenterKey        = "Datacenter"
	resourcePoolKey      = "ResourcePool"
	folderKey            = "Folder"
	datastoreKey         = "Datastore"
	contentSourceKey     = "ContentSource"
	networkNameKey       = "Network"

	NamespaceRPAnnotationKey     = "vmware-system-resource-pool"
	NamespaceFolderAnnotationKey = "vmware-system-vm-folder"
)

func ConfigMapsToProviderConfig(baseConfigMap *v1.ConfigMap, nsConfigMap *v1.ConfigMap, vcCreds *VSphereVmProviderCredentials) (*VSphereVmProviderConfig, error) {
	dataMap := make(map[string]string)

	if vcCreds == nil {
		return nil, errors.Errorf("VcCreds is unset")
	}

	if baseConfigMap != nil {
		for key, value := range baseConfigMap.Data {
			dataMap[key] = value
		}
	}

	if nsConfigMap != nil {
		for key, value := range nsConfigMap.Data {
			dataMap[key] = value
		}
	}

	vcPNID, ok := dataMap[vcPNIDKey]
	if !ok {
		return nil, errors.Errorf("missing configMap data field %s", vcPNIDKey)
	}

	vcPort, ok := dataMap[vcPortKey]
	if !ok {
		vcPort = "443"
	}

	ret := &VSphereVmProviderConfig{
		VcPNID:        vcPNID,
		VcPort:        vcPort,
		VcCreds:       vcCreds,
		Datacenter:    dataMap[datacenterKey],
		ResourcePool:  dataMap[resourcePoolKey],
		Folder:        dataMap[folderKey],
		Datastore:     dataMap[datastoreKey],
		ContentSource: dataMap[contentSourceKey],
		Network:       dataMap[networkNameKey],
	}

	return ret, nil
}

func configMapsToProviderCredentials(clientSet kubernetes.Interface, baseConfigMap *v1.ConfigMap, nsConfigMap *v1.ConfigMap) (vcCreds *VSphereVmProviderCredentials, err error) {
	switch {
	case nsConfigMap != nil && nsConfigMap.Data[vcCredsSecretNameKey] != "":
		vcCreds, err = GetProviderCredentials(clientSet, nsConfigMap.ObjectMeta.Namespace, nsConfigMap.Data[vcCredsSecretNameKey])
	case baseConfigMap != nil && baseConfigMap.Data[vcCredsSecretNameKey] != "":
		vcCreds, err = GetProviderCredentials(clientSet, baseConfigMap.ObjectMeta.Namespace, baseConfigMap.Data[vcCredsSecretNameKey])
	default:
		err = errors.Errorf("%s creds secret not set in ($%s) nor per-namespace ", vcCredsSecretNameKey, VmopNamespaceEnv)
	}
	return
}

// UpdateVMFolderAndRPInProviderConfig updates the RP and vm folder in the provider config from the namespace annotation.
func UpdateVMFolderAndRPInProviderConfig(clientSet kubernetes.Interface, namespace string, providerConfig *VSphereVmProviderConfig) error {
	var ns *v1.Namespace
	var err error
	if ns, err = clientSet.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{}); err != nil {
		return errors.Wrapf(err, "could not find the namespace: %s", namespace)
	}
	if len(ns.ObjectMeta.Annotations) == 0 ||
		ns.ObjectMeta.Annotations[NamespaceRPAnnotationKey] == "" ||
		ns.ObjectMeta.Annotations[NamespaceFolderAnnotationKey] == "" {
		klog.Warningf("Namespace %s has incomplete RP/VM folder annotations. "+
			"RP: %s, VM-folder: %s", ns.ObjectMeta.Name,
			ns.ObjectMeta.Annotations[NamespaceRPAnnotationKey],
			ns.ObjectMeta.Annotations[NamespaceFolderAnnotationKey])
	} else {
		providerConfig.ResourcePool = ns.ObjectMeta.Annotations[NamespaceRPAnnotationKey]
		providerConfig.Folder = ns.ObjectMeta.Annotations[NamespaceFolderAnnotationKey]
	}
	return nil
}

// GetProviderConfigFromConfigMap gets the vSphere Provider ConfigMap from the API Master.
func GetProviderConfigFromConfigMap(clientSet kubernetes.Interface, namespace string) (*VSphereVmProviderConfig, error) {
	var baseConfigMap, nsConfigMap *v1.ConfigMap
	var err error

	vmopNamespace, vmopNamespaceExists := os.LookupEnv(VmopNamespaceEnv)
	if vmopNamespaceExists {
		baseConfigMap, err = clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(VSphereConfigMapName, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			log.Info("could not find base provider ConfigMap", "namespace", vmopNamespace, "configMapName", VSphereConfigMapName)
		} else if err != nil {
			return nil, errors.Wrapf(err, "could not get base provider ConfigMap %v/%v", vmopNamespace, VSphereConfigMapName)
		}
	} else {
		log.Info("unset env, will fallback to exclusively using per-namespace configuration", "namespaceEnv", VmopNamespaceEnv)
	}

	nsConfigMap, err = clientSet.CoreV1().ConfigMaps(namespace).Get(VSphereConfigMapName, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		log.Info("could not find per-namespace provider ConfigMap", "namespace", namespace, "configMapName", VSphereConfigMapName)
	} else if err != nil {
		return nil, errors.Wrapf(err, "could not get per-namespace provider ConfigMap %v/%v", namespace, VSphereConfigMapName)
	}

	if baseConfigMap == nil && nsConfigMap == nil {
		return nil, errors.Errorf("neither base ($%s/%s/%s) nor per-namespace (%s/%s) provider configMaps are set",
			VmopNamespaceEnv, vmopNamespace, VSphereConfigMapName, namespace, VSphereConfigMapName)
	}

	// Get VcCreds from per-namespace or base configMap
	vcCreds, err := configMapsToProviderCredentials(clientSet, nsConfigMap, baseConfigMap)
	if err != nil {
		return nil, err
	}

	providerConfig, err := ConfigMapsToProviderConfig(baseConfigMap, nsConfigMap, vcCreds)
	if err != nil {
		return nil, err
	}

	if err := UpdateVMFolderAndRPInProviderConfig(clientSet, namespace, providerConfig); err != nil {
		return nil, errors.Wrapf(err, "error in updaing RP and VM folder")
	}

	return providerConfig, nil
}

func ProviderConfigToConfigMap(namespace string, config *VSphereVmProviderConfig, vcCredsSecretName string) *v1.ConfigMap {
	dataMap := make(map[string]string)

	dataMap[vcPNIDKey] = config.VcPNID
	dataMap[vcPortKey] = config.VcPort
	dataMap[vcCredsSecretNameKey] = vcCredsSecretName
	dataMap[datacenterKey] = config.Datacenter
	dataMap[resourcePoolKey] = config.ResourcePool
	dataMap[folderKey] = config.Folder
	dataMap[datastoreKey] = config.Datastore
	dataMap[contentSourceKey] = config.ContentSource

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VSphereConfigMapName,
			Namespace: namespace,
		},
		Data: dataMap,
	}
}

// Install the Config Map for the VM operator in the API master
func InstallVSphereVmProviderConfig(clientSet *kubernetes.Clientset, namespace string, config *VSphereVmProviderConfig, vcCredsSecretName string) error {
	configMap := ProviderConfigToConfigMap(namespace, config, vcCredsSecretName)
	if _, err := clientSet.CoreV1().ConfigMaps(namespace).Update(configMap); err != nil {
		return err
	}
	return InstallVSphereVmProviderSecret(clientSet, namespace, config.VcCreds, vcCredsSecretName)
}
