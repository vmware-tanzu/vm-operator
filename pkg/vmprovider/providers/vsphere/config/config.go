// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/credentials"
)

var log = logf.Log.WithName("vsphere").WithName("config")

// Configuration for a Vsphere VM Provider instance.  Contains information enabling integration with a backend
// vSphere instance for VM management.
type VSphereVmProviderConfig struct {
	VcPNID                      string
	VcPort                      string
	VcCreds                     *credentials.VSphereVmProviderCredentials
	Datacenter                  string
	Cluster                     string
	ResourcePool                string
	Folder                      string
	Datastore                   string
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

	ProviderConfigMapName = "vsphere.provider.config.vmoperator.vmware.com"
	// Keys in provider ConfigMap
	vcPNIDKey                    = "VcPNID"
	vcPortKey                    = "VcPort"
	vcCredsSecretNameKey         = "VcCredsSecretName" // nolint:gosec
	datacenterKey                = "Datacenter"
	clusterKey                   = "Cluster"
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

	NetworkConfigMapName = "vmoperator-network-config"
	// Keys in the NetworkConfigMapName
	NameserversKey = "nameservers"
)

func ConfigMapToProviderConfig(
	configMap *v1.ConfigMap,
	vcCreds *credentials.VSphereVmProviderCredentials) (*VSphereVmProviderConfig, error) {

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
		Cluster:                     dataMap[clusterKey],
		ResourcePool:                dataMap[resourcePoolKey],
		Folder:                      dataMap[folderKey],
		Datastore:                   dataMap[datastoreKey],
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

func configMapToProviderCredentials(
	client ctrlruntime.Client,
	configMap *v1.ConfigMap) (*credentials.VSphereVmProviderCredentials, error) {

	if configMap.Data[vcCredsSecretNameKey] == "" {
		return nil, errors.Errorf("%s creds secret not set in vmop system namespace", vcCredsSecretNameKey)
	}

	return credentials.GetProviderCredentials(
		client,
		configMap.ObjectMeta.Namespace,
		configMap.Data[vcCredsSecretNameKey])
}

func GetNameserversFromConfigMap(client ctrlruntime.Client) ([]string, error) {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return nil, err
	}

	configMap := &v1.ConfigMap{}
	configMapKey := types.NamespacedName{Name: NetworkConfigMapName, Namespace: vmopNamespace}
	if err := client.Get(context.Background(), configMapKey, configMap); err != nil {
		return nil, errors.Wrapf(err, "cannot retrieve %v ConfigMap", NetworkConfigMapName)
	}

	nameservers, ok := configMap.Data[NameserversKey]
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
func GetProviderConfigFromConfigMap(
	ctx context.Context,
	client ctrlruntime.Client,
	zone, namespace string) (*VSphereVmProviderConfig, error) {

	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return nil, err
	}

	configMap := &v1.ConfigMap{}
	configMapKey := types.NamespacedName{Name: ProviderConfigMapName, Namespace: vmopNamespace}
	err = client.Get(ctx, configMapKey, configMap)
	if err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		return nil, errors.Wrapf(err, "error retrieving the provider ConfigMap %s", configMapKey)
	}

	vcCreds, err := configMapToProviderCredentials(client, configMap)
	if err != nil {
		return nil, err
	}

	providerConfig, err := ConfigMapToProviderConfig(configMap, vcCreds)
	if err != nil {
		return nil, err
	}

	// If the FSS is not enabled and no zone was specified then assume the
	// default zone.
	if !lib.IsWcpFaultDomainsFSSEnabled() {
		if zone == "" {
			zone = topology.DefaultAvailabilityZoneName
		}
	}

	if zone != "" && namespace != "" {
		if err := UpdateProviderConfigFromZoneAndNamespace(
			ctx, client, zone, namespace, providerConfig); err != nil {

			return nil, err
		}

		// Preserve the existing behavior but this isn't quite right. We assume
		// in various places that we always have a ResourcePool, but we only
		// check that in the namespace case. Whatever we happen to currently do
		// with a client without it set doesn't need it so it "works".
		if providerConfig.ResourcePool == "" || providerConfig.Folder == "" {
			return nil, fmt.Errorf(
				"missing ResourcePool and Folder in ProviderConfig. "+
					"ResourcePool: %v, Folder: %v",
				providerConfig.ResourcePool,
				providerConfig.Folder)
		}
	}

	return providerConfig, nil
}

// UpdateProviderConfigFromZoneAndNamespace updates provider config for the specified zone
// and namespace.
func UpdateProviderConfigFromZoneAndNamespace(
	ctx context.Context,
	client ctrlruntime.Client,
	zone, namespace string,
	providerConfig *VSphereVmProviderConfig) error {

	ns := &v1.Namespace{}
	if err := client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return errors.Wrapf(err, "could not get the namespace: %s", namespace)
	}

	availabilityZone, err := topology.GetAvailabilityZone(ctx, client, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("missing zone/namespace information", "zone", zone, "namespace", namespace)
			return nil
		}
		return err
	}

	if nsInfo, ok := availabilityZone.Spec.Namespaces[namespace]; !ok {
		log.Info(
			"Missing availability zone for namespace",
			"zone", zone,
			"namespace", namespace)
	} else {
		if nsInfo.PoolMoId == "" || nsInfo.FolderMoId == "" {
			log.Info(
				"Incomplete namespace resources",
				"zone", zone,
				"namespace", namespace,
				"resourcePool", nsInfo.PoolMoId,
				"vmFolder", nsInfo.FolderMoId)
		} else {
			providerConfig.ResourcePool = nsInfo.PoolMoId
			providerConfig.Folder = nsInfo.FolderMoId
		}
	}

	return nil
}

func ProviderConfigToConfigMap(namespace string, config *VSphereVmProviderConfig, vcCredsSecretName string) *v1.ConfigMap {
	dataMap := make(map[string]string)

	dataMap[vcPNIDKey] = config.VcPNID
	dataMap[vcPortKey] = config.VcPort
	dataMap[vcCredsSecretNameKey] = vcCredsSecretName
	dataMap[datacenterKey] = config.Datacenter
	dataMap[clusterKey] = config.Cluster
	dataMap[resourcePoolKey] = config.ResourcePool
	dataMap[folderKey] = config.Folder
	dataMap[datastoreKey] = config.Datastore
	dataMap[scRequiredKey] = strconv.FormatBool(config.StorageClassRequired)
	dataMap[useInventoryKey] = strconv.FormatBool(config.UseInventoryAsContentSource)
	dataMap[caFilePathKey] = config.CAFilePath
	dataMap[insecureSkipTLSVerifyKey] = strconv.FormatBool(config.InsecureSkipTLSVerify)
	dataMap[CtrlVmVmAntiAffinityTagKey] = config.CtrlVmVmAntiAffinityTag
	dataMap[WorkerVmVmAntiAffinityTagKey] = config.WorkerVmVmAntiAffinityTag

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProviderConfigMapName,
			Namespace: namespace,
		},
		Data: dataMap,
	}
}

// Install the Config Map for the VM operator in the API master
// Used only in testing.
func InstallVSphereVmProviderConfig(client ctrlruntime.Client, namespace string, config *VSphereVmProviderConfig, vcCredsSecretName string) error {
	configMap := ProviderConfigToConfigMap(namespace, config, vcCredsSecretName)

	if err := client.Create(context.Background(), configMap); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		log.Info("Updating VM Operator ConfigMap as it already exists")
		if err := client.Update(context.Background(), configMap); err != nil {
			return err
		}
	}

	return credentials.InstallVSphereVmProviderSecret(
		client, namespace, config.VcCreds, vcCredsSecretName)
}

// PatchVcURLInConfigMap updates the ConfigMap with the new vSphere PNID and Port.
// BMV: This doesn't really have to be a Patch.
func PatchVcURLInConfigMap(client ctrlruntime.Client, vcPNID, vcPort string) error {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return err
	}

	configMap := &v1.ConfigMap{}
	configMapKey := types.NamespacedName{Name: ProviderConfigMapName, Namespace: vmopNamespace}
	if err := client.Get(context.Background(), configMapKey, configMap); err != nil {
		return err
	}

	origConfigMap := configMap.DeepCopyObject().(ctrlruntime.Object)
	configMap.Data[vcPNIDKey] = vcPNID
	configMap.Data[vcPortKey] = vcPort

	err = client.Patch(context.Background(), configMap, ctrlruntime.MergeFrom(origConfigMap))
	if err != nil {
		log.Error(err, "Failed to apply patch for ConfigMap", "configMapName", configMapKey)
		return err
	}

	return nil
}

// Install the Network Config Map for the VM operator in the API master
// Used only in testing.
func InstallNetworkConfigMap(client ctrlruntime.Client, nameservers string) error {
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

	err = client.Create(context.Background(), configMap)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		log.Info("Updating VM Operator ConfigMap since it already exists")
		return client.Update(context.Background(), configMap)
	}

	return nil
}
