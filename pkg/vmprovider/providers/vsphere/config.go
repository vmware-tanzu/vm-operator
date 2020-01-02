/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

// Configuration for a Vsphere VM Provider instance.  Contains information enabling integration with a backend
// vSphere instance for VM management.
type VSphereVmProviderConfig struct {
	VcPNID                      string
	VcPort                      string
	VcCreds                     *VSphereVmProviderCredentials
	Datacenter                  string
	ResourcePool                string
	Folder                      string
	Datastore                   string
	ContentSource               string
	Network                     string
	StorageClassRequired        bool
	UseInventoryAsContentSource bool
}

const (
	VmopNamespaceEnv         = "POD_NAMESPACE"
	VSphereConfigMapName     = "vsphere.provider.config.vmoperator.vmware.com"
	NameServersConfigMapName = "vmoperator-network-config"

	vcPNIDKey            = "VcPNID"
	vcPortKey            = "VcPort"
	vcCredsSecretNameKey = "VcCredsSecretName" // nolint:gosec
	datacenterKey        = "Datacenter"
	resourcePoolKey      = "ResourcePool"
	folderKey            = "Folder"
	datastoreKey         = "Datastore"
	contentSourceKey     = "ContentSource"
	networkNameKey       = "Network"
	scRequiredKey        = "StorageClassRequired"
	useInventoryKey      = "UseInventoryAsContentSource"

	NamespaceRPAnnotationKey     = "vmware-system-resource-pool"
	NamespaceFolderAnnotationKey = "vmware-system-vm-folder"
)

func ConfigMapToProviderConfig(configMap *v1.ConfigMap, vcCreds *VSphereVmProviderCredentials) (*VSphereVmProviderConfig, error) {
	if configMap == nil {
		return nil, errors.Errorf("Error getting the provider config from ConfigMap. ConfigMap is nil")
	}

	dataMap := make(map[string]string)

	if vcCreds == nil {
		return nil, errors.Errorf("VcCreds is unset")
	}

	for key, value := range configMap.Data {
		dataMap[key] = value
	}

	vcPNID, ok := dataMap[vcPNIDKey]
	if !ok {
		return nil, errors.Errorf("missing configMap data field %s", vcPNIDKey)
	}

	vcPort, ok := dataMap[vcPortKey]
	if !ok {
		vcPort = "443"
	}

	scRequired := false
	s, ok := dataMap[scRequiredKey]
	if ok {
		var err error
		scRequired, err = strconv.ParseBool(s)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse value of StorageClassRequired")
		}
	}

	useInventory := false
	u, ok := dataMap[useInventoryKey]
	if ok {
		var err error
		useInventory, err = strconv.ParseBool(u)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse value of UseInventory")
		}
	}

	ret := &VSphereVmProviderConfig{
		VcPNID:                      vcPNID,
		VcPort:                      vcPort,
		VcCreds:                     vcCreds,
		Datacenter:                  dataMap[datacenterKey],
		ResourcePool:                dataMap[resourcePoolKey],
		Folder:                      dataMap[folderKey],
		Datastore:                   dataMap[datastoreKey],
		ContentSource:               dataMap[contentSourceKey],
		Network:                     dataMap[networkNameKey],
		StorageClassRequired:        scRequired,
		UseInventoryAsContentSource: useInventory,
	}

	return ret, nil
}

func configMapToProviderCredentials(clientSet kubernetes.Interface, configMap *v1.ConfigMap) (*VSphereVmProviderCredentials, error) {
	if configMap == nil || configMap.Data[vcCredsSecretNameKey] == "" {
		return nil, errors.Errorf("%s creds secret not set in vmop system namespace", vcCredsSecretNameKey)
	}

	return GetProviderCredentials(clientSet, configMap.ObjectMeta.Namespace, configMap.Data[vcCredsSecretNameKey])
}

// UpdateVMFolderAndRPInProviderConfig updates the RP and vm folder in the provider config from the namespace annotation.
func UpdateVMFolderAndRPInProviderConfig(clientSet kubernetes.Interface, namespace string, providerConfig *VSphereVmProviderConfig) error {
	ns, err := clientSet.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not find the namespace: %s", namespace)
	}

	resourcePool := ns.ObjectMeta.Annotations[NamespaceRPAnnotationKey]
	vmFolder := ns.ObjectMeta.Annotations[NamespaceFolderAnnotationKey]

	if resourcePool == "" || vmFolder == "" {
		log.Info("Incomplete namespace resource annotations", "namespace", namespace,
			"resourcePool", resourcePool, "vmFolder", vmFolder)
	} else {
		providerConfig.ResourcePool = resourcePool
		providerConfig.Folder = vmFolder
	}

	if providerConfig.ResourcePool == "" || providerConfig.Folder == "" {
		return errors.Errorf("Invalid resourcepool/folder in providerConfig. ResourcePool: %v, Folder: %v", providerConfig.ResourcePool, providerConfig.Folder)
	}

	return nil
}

func GetNameserversFromConfigMap(clientSet kubernetes.Interface) ([]string, error) {
	vmopNamespace, vmopNamespaceExists := os.LookupEnv(VmopNamespaceEnv)
	if !vmopNamespaceExists {
		return nil, errors.Errorf("Cannot retrieve %v ConfigMap: unset env", NameServersConfigMapName)
	}

	nameserversConfigMap, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(NameServersConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot retrieve %v ConfigMap", NameServersConfigMapName)
	}

	nameservers, ok := nameserversConfigMap.Data["nameservers"]
	if !ok {
		return nil, errors.Wrapf(err, "invalid %v ConfigMap, missing key nameservers", NameServersConfigMapName)
	}

	nameserverList := strings.Fields(nameservers)
	if len(nameserverList) == 0 {
		return nil, errors.Errorf("No nameservers in %v ConfigMap", NameServersConfigMapName)
	}

	if len(nameserverList) == 1 && nameserverList[0] == "<worker_dns>" {
		return nil, errors.Errorf("No valid nameservers in %v ConfigMap. It still contains <worker_dns> key.", NameServersConfigMapName)
	}

	// do we need to validate that these look like valid ipv4 addresses?
	return nameserverList, nil
}

// GetProviderConfigFromConfigMap returns a provider config constructed from vSphere Provider ConfigMap in the VM operator namespace.
func GetProviderConfigFromConfigMap(clientSet kubernetes.Interface, namespace string) (*VSphereVmProviderConfig, error) {

	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract the VM operator namespace from env %v", VmopNamespaceEnv)
	}

	configMap, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(VSphereConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving the provider ConfigMap %v/%v", vmopNamespace, VSphereConfigMapName)
	}

	// Get VcCreds from the configMap
	vcCreds, err := configMapToProviderCredentials(clientSet, configMap)
	if err != nil {
		return nil, err
	}

	providerConfig, err := ConfigMapToProviderConfig(configMap, vcCreds)
	if err != nil {
		return nil, err
	}

	if namespace != "" {
		if err := UpdateVMFolderAndRPInProviderConfig(clientSet, namespace, providerConfig); err != nil {
			return nil, errors.Wrapf(err, "error in updating RP and VM folder")
		}
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
	dataMap[scRequiredKey] = strconv.FormatBool(config.StorageClassRequired)
	dataMap[useInventoryKey] = strconv.FormatBool(config.UseInventoryAsContentSource)

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
	if _, err := clientSet.CoreV1().ConfigMaps(namespace).Get(configMap.Name, metav1.GetOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		if _, err := clientSet.CoreV1().ConfigMaps(namespace).Create(configMap); err != nil {
			return err
		}
	} else {
		if _, err := clientSet.CoreV1().ConfigMaps(namespace).Update(configMap); err != nil {
			return err
		}
	}

	return InstallVSphereVmProviderSecret(clientSet, namespace, config.VcCreds, vcCredsSecretName)
}
