/* **********************************************************
 * Copyright 2018-2020 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	InsecureSkipTLSVerify       bool
	CAFilePath                  string
	CtrlVmVmAntiAffinityTag     string
	WorkerVmVmAntiAffinityTag   string
	TagCategoryName             string
}

const (
	DefaultVCPort = "443"

	VSphereConfigMapName = "vsphere.provider.config.vmoperator.vmware.com"
	// Keys in VSphereConfigMapName
	vcPNIDKey                    = "VcPNID"
	vcPortKey                    = "VcPort"
	vcCredsSecretNameKey         = "VcCredsSecretName" // nolint:gosec
	datacenterKey                = "Datacenter"
	resourcePoolKey              = "ResourcePool"
	folderKey                    = "Folder"
	datastoreKey                 = "Datastore"
	networkNameKey               = "Network"
	scRequiredKey                = "StorageClassRequired"
	useInventoryKey              = "UseInventoryAsContentSource"
	insecureSkipTLSVerifyKey     = "InsecureSkipTLSVerify"
	caFilePathKey                = "CAFilePath"
	ContentSourceKey             = "ContentSource"
	CtrlVmVmAntiAffinityTagKey   = "CtrlVmVmAATag"
	WorkerVmVmAntiAffinityTagKey = "WorkerVmVmAATag"
	ProviderTagCategoryNameKey   = "VmVmAntiAffinityTagCategoryName"

	// Namespace annotations set by WCP
	NamespaceRPAnnotationKey     = "vmware-system-resource-pool"
	NamespaceFolderAnnotationKey = "vmware-system-vm-folder"

	NetworkConfigMapName = "vmoperator-network-config"
	// Keys in the NetworkConfigMapName
	NameserversKey = "nameservers"
)

const (
	WcpClusterConfigFileName     = "wcp-cluster-config.yaml"
	WcpClusterConfigMapNamespace = "kube-system"
	WcpClusterConfigMapName      = "wcp-cluster-config"
	VmOpSecretName               = "wcp-vmop-sa-vc-auth" // nolint:gosec
)

type WcpClusterConfig struct {
	VcPNID string `yaml:"vc_pnid"`
	VcPort string `yaml:"vc_port"`
}

// BuildNewWcpClusterConfig builds and returns Config object from given config file.
func BuildNewWcpClusterConfig(wcpClusterCfgData map[string]string) (*WcpClusterConfig, error) {
	cfgData, ok := wcpClusterCfgData[WcpClusterConfigFileName]
	if !ok {
		return nil, errors.Errorf("Key %s not found", WcpClusterConfigFileName)
	}

	wcpClusterConfig := &WcpClusterConfig{}
	err := yaml.Unmarshal([]byte(cfgData), wcpClusterConfig)
	if err != nil {
		return nil, err
	}

	return wcpClusterConfig, nil
}

func BuildNewWcpClusterConfigMap(wcpClusterConfig *WcpClusterConfig) (v1.ConfigMap, error) {
	bytes, err := yaml.Marshal(wcpClusterConfig)
	if err != nil {
		return v1.ConfigMap{}, nil
	}

	dataMap := make(map[string]string)
	dataMap[WcpClusterConfigFileName] = string(bytes)

	return v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WcpClusterConfigMapName,
			Namespace: WcpClusterConfigMapNamespace,
		},
		Data: dataMap,
	}, nil
}

func ConfigMapToProviderConfig(configMap *v1.ConfigMap, vcCreds *VSphereVmProviderCredentials) (*VSphereVmProviderConfig, error) {
	dataMap := make(map[string]string)

	for key, value := range configMap.Data {
		dataMap[key] = value
	}

	vcPNID, ok := dataMap[vcPNIDKey]
	if !ok {
		return nil, errors.New("missing configMap data field VcPNID")
	}

	vcPort, ok := dataMap[vcPortKey]
	if !ok {
		vcPort = DefaultVCPort
	}

	scRequired := false
	if s, ok := dataMap[scRequiredKey]; ok {
		var err error
		scRequired, err = strconv.ParseBool(s)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse value of StorageClassRequired")
		}
	}

	useInventory := false
	if u, ok := dataMap[useInventoryKey]; ok {
		var err error
		useInventory, err = strconv.ParseBool(u)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse value of UseInventory")
		}
	}

	// Default to validating TLS by default.
	insecureSkipTLSVerify := false
	if v, ok := dataMap[insecureSkipTLSVerifyKey]; ok {
		var err error
		insecureSkipTLSVerify, err = strconv.ParseBool(v)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse value of InsecureSkipTLSVerify")
		}
	}
	// Only load the CA file path if we're not skipping TLS verification.
	// Pick the system root CA bundle by default, but allow the user to override this using the CAFilePath
	//  parameter
	// Golang by default uses the system root CAs.
	// ref: https://golang.org/src/crypto/x509/root_linux.go
	caFilePath := "/etc/pki/tls/certs/ca-bundle.crt"
	if !insecureSkipTLSVerify {
		if ca, ok := dataMap[caFilePathKey]; ok {
			caFilePath = ca
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
		ContentSource:               dataMap[ContentSourceKey],
		Network:                     dataMap[networkNameKey],
		StorageClassRequired:        scRequired,
		UseInventoryAsContentSource: useInventory,
		InsecureSkipTLSVerify:       insecureSkipTLSVerify,
		CAFilePath:                  caFilePath,
		CtrlVmVmAntiAffinityTag:     dataMap[CtrlVmVmAntiAffinityTagKey],
		WorkerVmVmAntiAffinityTag:   dataMap[WorkerVmVmAntiAffinityTagKey],
		TagCategoryName:             dataMap[ProviderTagCategoryNameKey],
	}

	return ret, nil
}

func configMapToProviderCredentials(clientSet kubernetes.Interface, configMap *v1.ConfigMap) (*VSphereVmProviderCredentials, error) {
	if configMap.Data[vcCredsSecretNameKey] == "" {
		return nil, errors.Errorf("%s creds secret not set in vmop system namespace", vcCredsSecretNameKey)
	}

	return GetProviderCredentials(clientSet, configMap.ObjectMeta.Namespace, configMap.Data[vcCredsSecretNameKey])
}

// UpdateProviderConfigFromNamespace updates provider config for this specific namespace
func UpdateProviderConfigFromNamespace(clientSet kubernetes.Interface, namespace string, providerConfig *VSphereVmProviderConfig) error {
	ns, err := clientSet.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not get the namespace: %s", namespace)
	}

	resourcePool := ns.ObjectMeta.Annotations[NamespaceRPAnnotationKey]
	vmFolder := ns.ObjectMeta.Annotations[NamespaceFolderAnnotationKey]

	if resourcePool != "" && vmFolder != "" {
		providerConfig.ResourcePool = resourcePool
		providerConfig.Folder = vmFolder
	} else {
		log.Info("Incomplete namespace resource annotations", "namespace", namespace,
			"resourcePool", resourcePool, "vmFolder", vmFolder)
	}

	return nil
}

func GetNameserversFromConfigMap(clientSet kubernetes.Interface) ([]string, error) {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return nil, err
	}
	nameserversConfigMap, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(NetworkConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot retrieve %v ConfigMap", NetworkConfigMapName)
	}

	nameservers, ok := nameserversConfigMap.Data[NameserversKey]
	if !ok {
		return nil, errors.Wrapf(err, "invalid %v ConfigMap, missing key nameservers", NetworkConfigMapName)
	}

	nameserverList := strings.Fields(nameservers)
	if len(nameserverList) == 0 {
		return nil, errors.Errorf("No nameservers in %v ConfigMap", NetworkConfigMapName)
	}

	if len(nameserverList) == 1 && nameserverList[0] == "<worker_dns>" {
		return nil, errors.Errorf("No valid nameservers in %v ConfigMap. It still contains <worker_dns> key", NetworkConfigMapName)
	}

	// do we need to validate that these look like valid ipv4 addresses?
	return nameserverList, nil
}

// GetProviderConfigFromConfigMap returns a provider config constructed from vSphere Provider ConfigMap in the VM operator namespace.
func GetProviderConfigFromConfigMap(clientSet kubernetes.Interface, namespace string) (*VSphereVmProviderConfig, error) {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return nil, err
	}

	configMap, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(VSphereConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving the provider ConfigMap %v/%v", vmopNamespace, VSphereConfigMapName)
	}

	vcCreds, err := configMapToProviderCredentials(clientSet, configMap)
	if err != nil {
		return nil, err
	}

	providerConfig, err := ConfigMapToProviderConfig(configMap, vcCreds)
	if err != nil {
		return nil, err
	}

	if namespace != "" {
		if err := UpdateProviderConfigFromNamespace(clientSet, namespace, providerConfig); err != nil {
			return nil, errors.Wrapf(err, "error updating provider config from namespace")
		}

		// Preserve the existing behavior but this isn't quite right. We assume in various places that
		// we always have a ResourcePool, but we only check that in the namespace case. Whatever we
		// happen to currently do with a client without it set doesn't need it so it "works".
		if providerConfig.ResourcePool == "" || providerConfig.Folder == "" {
			return nil, fmt.Errorf("missing ResourcePool and Folder in ProviderConfig. "+
				"ResourcePool: %v, Folder: %v", providerConfig.ResourcePool, providerConfig.Folder)
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
	dataMap[ContentSourceKey] = config.ContentSource
	dataMap[scRequiredKey] = strconv.FormatBool(config.StorageClassRequired)
	dataMap[useInventoryKey] = strconv.FormatBool(config.UseInventoryAsContentSource)
	dataMap[caFilePathKey] = config.CAFilePath
	dataMap[insecureSkipTLSVerifyKey] = strconv.FormatBool(config.InsecureSkipTLSVerify)
	dataMap[CtrlVmVmAntiAffinityTagKey] = config.CtrlVmVmAntiAffinityTag
	dataMap[WorkerVmVmAntiAffinityTagKey] = config.WorkerVmVmAntiAffinityTag

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
		log.Info("Updating VM Operator configmap as it already exists")
		if _, err := clientSet.CoreV1().ConfigMaps(namespace).Update(configMap); err != nil {
			return err
		}
	}

	return InstallVSphereVmProviderSecret(clientSet, namespace, config.VcCreds, vcCredsSecretName)
}

// PatchVcURLInConfigMap updates the ConfigMap with the new vSphere PNID and Port.
func PatchVcURLInConfigMap(clientSet kubernetes.Interface, config *WcpClusterConfig) error {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return err
	}

	patch := map[string]map[string]string{
		"data": {
			vcPNIDKey: config.VcPNID,
			vcPortKey: config.VcPort,
		},
	}
	marshaledPatch, err := json.Marshal(patch)
	if err != nil {
		log.Error(err, "error creating the JSON patch", "patch", patch)
		return err
	}

	_, err = clientSet.CoreV1().ConfigMaps(vmopNamespace).Patch(VSphereConfigMapName, types.StrategicMergePatchType, marshaledPatch)
	if err != nil {
		log.Error(err, "Failed to apply patch for ConfigMap", "name", VSphereConfigMapName, "namespace", vmopNamespace, "patch", patch)
		return err
	}

	return nil
}

// Install the Network Config Map for the VM operator in the API master
func InstallNetworkConfigMap(clientSet *kubernetes.Clientset, nameservers string) error {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return err
	}

	dataMap := make(map[string]string)

	dataMap[NameserversKey] = nameservers

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkConfigMapName,
			Namespace: vmopNamespace,
		},
		Data: dataMap,
	}
	if _, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Get(configMap.Name, metav1.GetOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		if _, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Create(configMap); err != nil {
			return err
		}
	} else {
		log.Info("Updating VM Operator network configmap as it already exists")
		if _, err := clientSet.CoreV1().ConfigMaps(vmopNamespace).Update(configMap); err != nil {
			return err
		}
	}
	return nil
}
