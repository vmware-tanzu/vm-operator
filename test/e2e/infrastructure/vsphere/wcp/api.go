// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

// Missing features tracked here
// - Resource quotas (CPU/memory). Don't think we need them in GC yet, but maybe nice to have.
// - WCP Enablement/disablement. Not something we currently use/intend to test.

// WorkloadManagementAPI exposes methods to interact with wcpsvc on a vCenter. This is not meant to be a faithful implementation
//
//	of all WCP VAPI methods, but a minimal subset to make writing gce2e tests/enhancing the framework easier.
//
// For an exhaustive list of WCP VMODL methods, see https://vcode.eng.vmware.com/home/explorerepo/blob/@latest/bora.main.perforce.1666/bora/main/vpx/wcp/wcpsvc/vmodl
// ListClusters lists all WCP enabled clusters.
// GetClusters gets information about a specific cluster, given the cluster MoID.
// ListNamespaces lists all WCP enabled namespaces.
// GetNamespace returns information about a specific namespace, given the name.
// CreateNamespace creates a namespace in the given cluster (by MoID) with the given name.
// DeleteNamespace deletes the given namespace.
// SetNamespaceStorageSpecs associates a list of storage policies (with optional limits) with a given namespace,
//
//	functioning as a PUT/Set() operation (ie. removing any old associations)
//
// CreateNamespacePermissions grants a principal (user/group) access (either edit or view) to a given namespace.
//
//	This assumes the user already has access to the namespace.
//	RemoveNamespacePermissions removes all privileges the principal has on a given namespace.
type WorkloadManagementAPI interface {
	// Supervisor Cluster CRUD
	ListSupervisorSummary() (SupervisorSummaryList, error)

	// Namespace CRUD
	ListClusters() ([]WCPClusterDetails, error)
	GetCluster(string) (WCPClusterDetails, error)
	ListNamespaces() ([]NamespaceDetails, error)
	GetNamespace(string) (NamespaceDetails, error)
	GetNamespaceV2(string) (NamespaceDetails, error)
	CreateNamespace(clusterMoid, namespace string) error
	CreateNamespaceWithSpecs(clusterMoid, namespace string, storageSpecs []StorageSpec, vmsvcSpec VMServiceSpecDetails) error
	CreateNamespaceWithVMReservation(namespace, zone, supervisorID string, storageSpecs []StorageSpec, vmsvcSpec VMServiceSpecDetails, vmClassNameToReservedCount map[string]int) error
	CreateNamespaceWithNetwork(clusterMoid, namespace string, storageSpecs []StorageSpec, vmsvcSpec VMServiceSpecDetails, network *NameSpaceNetworkInfo) error
	DeleteNamespace(namespace string) error
	SetNamespaceStorageSpecs(namespace string, specs []StorageSpec) error

	CreateNamespaceNetwork(config NamespaceNetworkConfig) error
	GetNamespaceNetwork(cluster, network string) (map[string]any, error)
	UpdateNamespaceWithNetworks(namespaceName, networkProvider, networkToAdd string) error

	GetVirtualMachine(vmMoid string) (VirtualMachineDetails, error)

	// Namespaces Authz
	CreateNamespacePermissions(user Principal, namespace string, level AccessType) error
	RemoveNamespacePermissions(principal Principal, namespace string) error

	// SupervisorServices
	DeleteSupervisorService(serviceID string) error

	// ActivateSupervisorService sets the state of a service, and all its versions, to ACTIVATED state.
	ActivateSupervisorService(serviceID string) error

	// DeactivateSupervisorService sets the state of a service, and all its versions, to DEACTIVATED state.
	DeactivateSupervisorService(serviceID string) error

	// CreateOrSetClusterSupervisorService creates or sets specified Supervisor Service Version in the cluster
	CreateOrSetClusterSupervisorService(op DcliOperationType, cluster, serviceID, version string, serviceConfig map[string]string) error
	CreateOrSetClusterSupervisorServiceWithYamlConfig(op DcliOperationType, cluster, serviceID, version, yamlServiceConfig string) error

	// Enables/Disables PSP supervisor service in the cluster using v1 "set" API.
	EnableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error
	DisableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error

	// DeleteClusterSupervisorService deletes specified Supervisor Service from the cluster
	DeleteClusterSupervisorService(cluster, serviceID string) error

	// ActivateSupervisorService sets the state of a service version to ACTIVATED state.
	ActivateSupervisorServiceVersion(serviceID, version string) error

	// DeactivateSupervisorService sets the state of a service version to DEACTIVATED state.
	DeactivateSupervisorServiceVersion(serviceID, version string) error

	// DeleteSupervisorServiceVersion deletes a specific version of a Supervisor Service.
	DeleteSupervisorServiceVersion(serviceID, version string) error

	// Carvel Service
	RegisterCarvelService(carvelYaml []byte) error
	RegisterCarvelServiceVersion(serviceID string, carvelYaml []byte) error

	// ServicePrecheck initiates a pre-check task for a Supervisor Service version against a Supervisor.
	ServicePrecheck(serviceID, version, supervisor string) error

	// VMService APIs.
	// GetVMClassInfo returns info about a given VMClass.
	GetVMClassInfo(vmClass string) (VMClassInfo, error)

	// ListVMClasses lists all the VMClasses in the VC.
	ListVMClasses() ([]VMClassInfo, error)

	// CreateVMClass creates VMClass with given spec in the VC.
	CreateVMClass(createSpec VMClassSpec) error

	// UpdateVMClass updates an existing VMClass in the VC.
	UpdateVMClass(updateSpec VMClassSpec) error

	// DeleteVMClass deletes specified VMClass from the VC.
	DeleteVMClass(vmClass string) error

	// UpdateNamespaceVMServiceSpec updates namespace instance for changes made w.r.t. VMService specs.
	UpdateNamespaceVMServiceSpec(namespaceName string, updateSpec NamespaceUpdateVMserviceSpec) error

	// ListContentLibraries lists all the content libraries in the VC
	ListContentLibraries() ([]string, error)

	// GetContentLibrary returns a ContentLibrary via an ID.
	GetContentLibrary(id string) (ContentLibraryInfo, error)

	// FetchContentLibraryIDByName fetches a content library ID given name of CL ID
	FetchContentLibraryIDByName(name string, libraries []string) (string, error)

	// ListDatastores lists all datastores in vCenter.
	ListDatastores() ([]Datastore, error)

	// AddCLTrustedCertificate adds trusted certificate to Content Library.
	AddCLTrustedCertificate(trustedCertificate string) (string, error)

	// DeleteCLTrustedCertificate deletes a trusted certificate from Content Library.
	DeleteCLTrustedCertificate(trustedCertificateID string) error

	// ListCLSecurityPolicies lists all security policies in CL.
	ListCLSecurityPolicies() ([]SecurityPolicyInfo, error)

	// CreateLocalContentLibrary creates a local Content Library with the associated storage backing info.
	CreateLocalContentLibrary(name string, storageBackings StorageBackingInfo) (string, error)

	// DeleteLocalContentLibrary deletes a local Content Library.
	DeleteLocalContentLibrary(id string) error

	// DeleteLocalContentLibraryByForce force deletes a local Content Library
	DeleteLocalContentLibraryByForce(id string) error

	// UpdateContentLibrary updates common aspects of a Content Library.
	UpdateContentLibrary(id, description, securityPolicyID string) error

	// UpdateLocalContentLibrary updates aspects of a local Content Library.
	UpdateLocalContentLibrary(id string, enablePublishing bool) error

	// CreateSubscribedContentLibrary creates a subscribed Content Library with the given subscription URL and storage backing info.
	CreateSubscribedContentLibrary(name, subscriptionURL, thumbprint string, onDemand bool, storageBackings StorageBackingInfo) (string, error)

	// SyncSubscribedContentLibrary synchronizes the specified content library.
	SyncSubscribedContentLibrary(id string) error

	// DeleteSubscribedContentLibrary deletes a subscribed Content Library.
	DeleteSubscribedContentLibrary(id string) error

	// DeleteSubscribedContentLibraryByForce force deletes a subscribed Content Library.
	DeleteSubscribedContentLibraryByForce(id string) error

	// CreateContentLibraryItem creates a new Content Library item
	CreateContentLibraryItem(contentLibraryID, name string, itemType ContentLibraryItemType) (string, error)

	// CreateContentLibraryOVFTemplateItemByPull creates a new content library item in OVF by pulling the given OVF URL.
	// It does not wait for the content upload completion, client should wait and check for item ready status.
	CreateContentLibraryOVFTemplateItemByPull(contentLibraryID, name, ovfFileURL string) (string, error)

	// ListContentLibraryItems returns a list of library item identifiers.
	ListContentLibraryItems(contentLibraryID string) ([]string, error)

	// GetContentLibraryItem returns a Content Library Item
	GetContentLibraryItem(id string) (ContentLibraryItemInfo, error)

	// UpdateContentLibraryItem updates aspects of a Content Library Item.
	UpdateContentLibraryItem(id, description string) error

	// DeleteContentLibraryItem deletes a Content Library Item
	DeleteContentLibraryItem(id string) error

	// AssociateContentLibrariesToCluster associates a list of content libraries to a Supervisor Cluster.
	AssociateContentLibrariesToCluster(cluster string, contentLibraries ...ClusterContentLibrarySpec) error

	// DisassociateContentLibrariesFromCluster disassociates the provided list of content libraries from a Supervisor Cluster.
	// If they were not already associated, this will be accepted as success.
	DisassociateContentLibrariesFromCluster(cluster string, contentLibraryIDs ...string) error

	// AssociateImageRegistryContentLibrariesToNamespace associates a list of Image Registry content libraries to a Supervisor Namespace.
	AssociateImageRegistryContentLibrariesToNamespace(namespace string, contentLibraries ...ContentLibrarySpec) error

	// DisassociateImageRegistryContentLibrariesFromNamespace disassociates the provided list of content libraries from a Supervisor namespace.
	// If they were not already associated, this will be accepted as success.
	DisassociateImageRegistryContentLibrariesFromNamespace(namespace string, contentLibraryIDs ...string) error

	// RegisterVM registers an existing virtual machine as VM Service managed VM.
	RegisterVM(namespace string, vmMoID string) (string, error)

	// GetWorkerDNS returns the set worker DNS server IPs
	GetWorkerDNS(clusterMoid string) ([]string, error)

	// UpdateWorkerDNS updates the worker DNS server IPs on the cluster.
	UpdateWorkerDNS(clusterMoid string, dnsServerIPs ...string) error

	// CRUD APIs for configuring cluster proxy settings on the cluster
	// see: https://opengrok2.eng.vmware.com/xref/main.perforce.1666/bora/vpx/wcp/wcpsvc/vmodl/namespace_management/ProxyConfiguration.vmodl
	UpdateClusterProxyConfig(clusterMoid string, proxyConfig ClusterProxyConfig) error
	GetClusterProxyConfig(clusterMoid string) (ClusterProxyConfig, error)

	// CRUD APIs for configuring container image registries
	CreateContainerImageRegistry(supervisorID string, registry ContainerImageRegistry) (ContainerImageRegistryInfo, error)
	GetContainerImageRegistry(supervisorID, registryName string) (ContainerImageRegistryInfo, error)
	DeleteContainerImageRegistry(supervisorID, registryID string) error

	// CRUD APIs for configuring Key Providers
	CreateKeyProvider(provider string) error
	DeleteKeyProvider(provider string) error

	// CRUD APIs for Zones
	ListVSphereZones() (VSphereZoneList, error)
	GetZonesBoundWithSupervisor(supervisorID string) (ZoneList, error)
	UpdateNamespaceWithZones(namespace string, zones []ZoneSpec) error
	DeleteZoneFromNamespace(namespace string, zone string) error
	CreateZoneBindingsWithSupervisor(supervisorID string, zoneBindingSpecs []ZoneBindingSpecs) error
	DeleteZoneBindingsWithSupervisor(supervisorID string, zones string) error
	UpdateZoneBindingsWithVMReservation(supervisorID, zone string, reservedVMClassToCount map[string]int) error

	// CIS APIs.
	AssignLicenseEntitlement(signedEntitlement string) (string, error)
	CreateTagCategory(name, description string) (string, error)
	CreateTag(name, description, categoryID string) (string, error)
	AssignTagsToHost(tagIDs []string, hostID string) error

	// IaaS Policy APIs.
	CreateComputePolicy(spec ComputePolicySpec) (string, error)
	CreateInfraPolicy(spec InfraPolicySpec) error
	UpdateNamespaceWithInfraPolicies(namespace string, policyNames ...string) error
	ListComputePolicyTagUsage(categoryName, tagName string) ([]TagUsageEntry, error)
	GetVMPolicyCompliance(policyID, vmMoid string) (VMPolicyComplianceStatus, error)

	// NSX APIs
	GetSupervisorLoadBalancerProvider(supervisorID string) (string, error)

	// Host APIs.
	ListHostIDs() ([]string, error)
}
