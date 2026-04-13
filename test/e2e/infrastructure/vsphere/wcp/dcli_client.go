// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/types"
)

// wcpDcliClient of WorkloadManagementAPI that uses DCLI + SSH to achieve the desired result.
// This is a short term workaround until a better REST client is implemented (which would do
//
//	a better job of catching breaking API contracts) while also decoupling us from generated vapi-go code.
//	(which is potentially a pain to consume from bora/VMODL)
type wcpDcliClient struct {
	dcliClient dcli.DCLICommandRunner
}

const (
	dcliVCenterPrefix                  = "com vmware vcenter "
	dcliNamespaces                     = dcliVCenterPrefix + "namespaces "
	namespaceManagement                = dcliVCenterPrefix + "namespacemanagement "
	supervisors                        = namespaceManagement + "supervisors"
	namespacesInstances                = dcliNamespaces + "instances "
	namespacesAccess                   = dcliNamespaces + "access "
	namespaceServices                  = dcliNamespaces + "supervisorservices " // for supervisor service v1 APIs
	namespaceManagementServices        = namespaceManagement + " +show-unreleased-apis supervisorservices "
	namespaceManagementClusterServices = namespaceManagementServices + " clustersupervisorservices "
	namespaceManagementSupervisors     = namespaceManagement + " supervisors"
	namespaceMgmtSupervisorsSvServices = namespaceManagementSupervisors + " supervisorservices +show-unreleased"
	namespaceManagementVMClassServices = namespaceManagement + "virtualmachineclasses +show-unreleased"
	virtualmachine                     = dcliVCenterPrefix + "vm"
	consumptiondomainsPrefix           = "consumptiondomains "
	zones                              = dcliVCenterPrefix + consumptiondomainsPrefix + "zones"
	cryptoManager                      = dcliVCenterPrefix + "cryptomanager"
	dcliContentPrefix                  = "com vmware content library"
	dcliContentLocalPrefix             = "com vmware content locallibrary"
	dcliContentSubscribedPrefix        = "com vmware content subscribedlibrary"
	dcliCISLicenseEntitlementPrefix    = "com vmware cis license subscription entitlement"
	dcliSupervisorSummary              = namespaceManagement + "supervisors summary list"
	dcliSupervisorNetworkEdges         = namespaceManagement + "supervisors networks edges list "
	jsonFormatter                      = "+formatter json"
	showUnreleased                     = "+show-unreleased"
	namespaceManagementNetworks        = namespaceManagement + "networks +show-unreleased"
	namespacesInstancesUpdate          = dcliNamespaces + "instances update +show-unreleased"
)

type DcliOperationType int

const (
	Create DcliOperationType = iota
	Set
)

func (op DcliOperationType) String() string {
	return [...]string{"create", "set"}[op]
}

// DcliError is an error type to help wrap errors that get returned from DCLI commands,
// to be able to help pass along a regular error that might have strings in the response
// we want to verify.
type DcliError struct {
	rawResponse string
	baseErr     error
}

func (e DcliError) Error() string {
	return e.baseErr.Error()
}

func (e DcliError) Unwrap() error {
	return e.baseErr
}

func (e DcliError) Response() string {
	return e.rawResponse
}

// Add these structs for network creation input.
type NamespaceNetworkConfig struct {
	Cluster         string `json:"cluster"`
	Network         string `json:"network"`
	NetworkProvider string `json:"network_provider"`

	// vSphere Network specific
	VsphereNetworkMode             string `json:"vsphere_network_mode"`
	VsphereNetworkIPAssignmentMode string `json:"vsphere_network_ip_assignment_mode"`
	VsphereNetworkPortgroup        string `json:"vsphere_network_portgroup"`
	VsphereNetworkGateway          string `json:"vsphere_network_gateway"`
	VsphereNetworkSubnetMask       string `json:"vsphere_network_subnet_mask"`
}

// NewWCPAPIClient is a public method meant to get a WorkloadManagementAPI, regardless of the backing implementation.
// The idea is to first implement a DCLI based one and then transition to a VAPI backed one, while
//
//	consumers can rely on the interface exposed by NewWCPAPIClient/WorkloadManagementAPI.
func NewWCPAPIClient(vCenterHostname string, username, password, sshUsername, sshPassword string) (WorkloadManagementAPI, error) {
	return newWcpDcliClient(vCenterHostname, username, password, sshUsername, sshPassword)
}

func newWcpDcliClient(vCenterHostname, username, password, sshUsername, sshPassword string) (WorkloadManagementAPI, error) {
	dcliClient, err := dcli.NewDCLICommandRunner(vCenterHostname, vcenter.VCSSHPort, username, password, sshUsername, sshPassword)
	if err != nil {
		return nil, err
	}

	return &wcpDcliClient{
		dcliClient: dcliClient,
	}, nil
}

func (d *wcpDcliClient) ListSupervisorSummary() (SupervisorSummaryList, error) {
	cmd := fmt.Sprintf("%s summary list %s", supervisors, jsonFormatter)
	retVal := SupervisorSummaryList{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) ListClusters() ([]WCPClusterDetails, error) {
	cmd := fmt.Sprintf("%s clusters list +formatter json", namespaceManagement)
	retVal := []WCPClusterDetails{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetCluster(moID string) (WCPClusterDetails, error) {
	cmd := fmt.Sprintf("%s clusters get --cluster %s +formatter json", namespaceManagement, moID)
	retVal := WCPClusterDetails{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) ListNamespaces() ([]NamespaceDetails, error) {
	cmd := fmt.Sprintf("%s list +formatter json", namespacesInstances)
	retVal := []NamespaceDetails{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetNamespace(name string) (NamespaceDetails, error) {
	cmd := fmt.Sprintf("%s get --namespace %s +formatter json", namespacesInstances, name)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return NamespaceDetails{}, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	retVal := NamespaceDetails{}
	if err = json.Unmarshal(resp, &retVal); err != nil {
		return NamespaceDetails{}, err
	}

	return retVal, err
}

func (d *wcpDcliClient) GetNamespaceV2(name string) (NamespaceDetails, error) {
	cmd := fmt.Sprintf("%s getv2 --namespace %s +formatter json", namespacesInstances, name)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return NamespaceDetails{}, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	retVal := NamespaceDetails{}
	if err = json.Unmarshal(resp, &retVal); err != nil {
		return NamespaceDetails{}, err
	}

	return retVal, nil
}

func (d *wcpDcliClient) CreateNamespacePermissions(principal Principal, namespace string, level AccessType) error {
	cmd := fmt.Sprintf("%s create --domain %s --namespace %s --subject %s --role %s --type %s",
		namespacesAccess,
		principal.Domain,
		namespace,
		principal.Name,
		level,
		principal.Type,
	)

	// We don't really care about the STDOUT of this command.
	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

func (d *wcpDcliClient) RemoveNamespacePermissions(principal Principal, namespace string) error {
	cmd := fmt.Sprintf("%s delete --domain %s --namespace %s --subject %s --type %s",
		namespacesAccess,
		principal.Domain,
		namespace,
		principal.Name,
		principal.Type,
	)
	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

func (d *wcpDcliClient) CreateNamespace(clusterMoid, namespace string) error {
	cmd := fmt.Sprintf("%s create --cluster %s --namespace %s",
		namespacesInstances,
		clusterMoid,
		namespace,
	)
	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

// buildCreateNamespaceCommand is a helper function to build the command string for creating a namespace.
func (d *wcpDcliClient) buildCreateNamespaceCommand(
	clusterMoid, namespace string,
	storageSpecs []StorageSpec,
	vmsvcSpec VMServiceSpecDetails,
	network *NameSpaceNetworkInfo) (string, error) {
	var cmd strings.Builder

	baseCmd, err := d.buildCreateNamespaceBaseCommand(clusterMoid, namespace, network)
	if err != nil {
		return "", fmt.Errorf("failed to build network-specific command: %w", err)
	}

	cmd.WriteString(baseCmd)

	if len(storageSpecs) > 0 {
		storageSpecBuffer, err := json.Marshal(storageSpecs)
		if err != nil {
			return "", fmt.Errorf("failed to marshal storage specs: %w", err)
		}

		fmt.Fprintf(&cmd, " --storage-specs '%s'", string(storageSpecBuffer))
	}

	for _, vmClass := range vmsvcSpec.VMClasses {
		fmt.Fprintf(&cmd, " --vm-service-spec-vm-classes %s", vmClass)
	}

	for _, clID := range vmsvcSpec.ContentLibraries {
		fmt.Fprintf(&cmd, " --vm-service-spec-content-libraries %s", clID)
	}

	return cmd.String(), nil
}

// buildCreateNamespaceBaseCommand builds the base command based on the network configuration.
func (d *wcpDcliClient) buildCreateNamespaceBaseCommand(
	clusterMoid, namespace string,
	network *NameSpaceNetworkInfo) (string, error) {
	// Handle VDS network
	if network != nil {
		if network.VDSNetwork != "" {
			return fmt.Sprintf(
				"%s create --cluster %s --namespace %s --networks %s",
				namespacesInstances,
				clusterMoid,
				namespace,
				network.VDSNetwork,
			), nil
		}
		// Handle VPC network with necessary validation
		if network.VPCNetwork != nil {
			if network.VPCNetwork.VPCPath == "" || network.VPCNetwork.SupervisorID == "" || network.VPCNetwork.DefaultSubnetSize == 0 {
				return "", fmt.Errorf("VPC network requires VPCPath, SupervisorID, and DefaultSubnetSize to be defined")
			}
			// com vmware vcenter namespaces instances create --nanespace tkgs-test-1
			// --supervisor  e12272e9-253d-489f-b078-f3a4a3948dba
			// --network-spec-network-provider NSX_VPC
			// --network-spec-vpc-network-vpc /orgs/default/projects/project-quality/vpcs/vpc-ext-104
			// --network-spec-vpc-network-default-subnet-size 32
			// --network-spec-vpc-network-shared-subnets /orgs/default/projects/project-quality/vpcs/vpc-ext-104/subnets/vlan-subnet-104

			// Build the base command
			cmd := fmt.Sprintf(
				"%s createv2 --namespace %s "+
					"--supervisor %s "+
					"--network-spec-network-provider NSX_VPC "+
					"--network-spec-vpc-network-default-subnet-size %d "+
					"--network-spec-vpc-network-vpc %s",
				namespacesInstances,
				namespace,
				network.VPCNetwork.SupervisorID,
				network.VPCNetwork.DefaultSubnetSize,
				network.VPCNetwork.VPCPath,
			)

			// Add shared subnets parameter only if VPCSharedSubnetPath is not empty
			// The parameter needs to be formatted as a JSON array of objects with "path" field
			if network.VPCNetwork.VPCSharedSubnetPath != "" {
				sharedSubnets := []map[string]string{
					{"path": network.VPCNetwork.VPCSharedSubnetPath},
				}

				sharedSubnetsJSON, err := json.Marshal(sharedSubnets)
				if err != nil {
					return "", fmt.Errorf("failed to marshal shared subnets: %w", err)
				}

				cmd += fmt.Sprintf(" --network-spec-vpc-network-shared-subnets '%s'", string(sharedSubnetsJSON))
			}

			return cmd, nil
		}
	}

	// Handle default case when no network is specified
	return fmt.Sprintf("%s create --cluster %s --namespace %s",
		namespacesInstances,
		clusterMoid,
		namespace,
	), nil
}

// CreateNamespaceWithSpecs creates a namespace with the given specs.
func (d *wcpDcliClient) CreateNamespaceWithSpecs(
	clusterMoid, namespace string,
	storageSpecs []StorageSpec,
	vmsvcSpec VMServiceSpecDetails) error {
	// Call the common command building function without network
	cmd, err := d.buildCreateNamespaceCommand(clusterMoid, namespace, storageSpecs, vmsvcSpec, nil)
	if err != nil {
		return err
	}

	stdout, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		if !strings.Contains(string(stdout), "errors.AlreadyExists") {
			return err
		}
	}

	return nil
}

// CreateNamespaceWithNetwork creates a namespace with the given specs and network.
func (d *wcpDcliClient) CreateNamespaceWithNetwork(
	clusterMoid, namespace string,
	storageSpecs []StorageSpec,
	vmsvcSpec VMServiceSpecDetails,
	network *NameSpaceNetworkInfo) error {
	cmd, err := d.buildCreateNamespaceCommand(clusterMoid, namespace, storageSpecs, vmsvcSpec, network)
	if err != nil {
		return err
	}

	_, err = d.dcliClient.RunDCLICommand(cmd)

	return err
}

// CreateNamespaceWithVMReservation creates a namespace with the given specs and VM reservations.
func (d *wcpDcliClient) CreateNamespaceWithVMReservation(namespace, zone, supervisorID string, storageSpecs []StorageSpec, vmsvcSpec VMServiceSpecDetails, vmClassNameToReservedCount map[string]int) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s createv2 --namespace %s --supervisor %s --zones '[{\"name\": \"%s\", \"vm_reservations\": [",
		namespacesInstances,
		showUnreleased,
		namespace,
		supervisorID,
		zone)

	for vmClass, count := range vmClassNameToReservedCount {
		fmt.Fprintf(&cmd, "{\"reserved_vm_class\": \"%s\", \"count\": %d},", vmClass, count)
	}

	// Remove the last comma in "vm_reservations" if exists.
	cmdStr := cmd.String()
	if cmdStr[len(cmdStr)-1] == ',' {
		cmd.Reset()
		cmd.WriteString(cmdStr[:len(cmdStr)-1])
	}

	cmd.WriteString("]}]'")

	// Add storage specs.
	storageSpecBuffer, err := json.Marshal(storageSpecs)
	if err != nil {
		return err
	}

	storageSpecString := string(storageSpecBuffer)
	fmt.Fprintf(&cmd, " --storage-specs '%s'", storageSpecString)

	// Add vm service specs (VM Class and Content Library).
	for _, vmClass := range vmsvcSpec.VMClasses {
		fmt.Fprintf(&cmd, " --vm-service-spec-vm-classes %s", vmClass)
	}

	for _, clID := range vmsvcSpec.ContentLibraries {
		fmt.Fprintf(&cmd, " --vm-service-spec-content-libraries %s", clID)
	}

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// ListVSphereZones list all vSphere zones.
func (d *wcpDcliClient) ListVSphereZones() (VSphereZoneList, error) {
	cmd := fmt.Sprintf("%s list +formatter json", zones)
	retVal := VSphereZoneList{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

// UpdateNamespaceWithZones binds a namespace with the given namespace scoped Zones.
func (d *wcpDcliClient) UpdateNamespaceWithZones(
	namespace string,
	zones []ZoneSpec) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s update --namespace %s",
		namespacesInstances,
		showUnreleased,
		namespace)

	zoneSpec, err := json.Marshal(zones)
	if err != nil {
		return err
	}

	zoneSpecString := string(zoneSpec)
	fmt.Fprintf(&cmd, " --zones '%s'", zoneSpecString)

	_, err = d.dcliClient.RunDCLICommand(cmd.String())

	return err
}

// DeleteZoneFromNamespace delete a zone from namespace (Mark Zone for removal).
func (d *wcpDcliClient) DeleteZoneFromNamespace(
	namespace string,
	zone string) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones delete --namespace %s --zone %s",
		namespacesInstances,
		showUnreleased,
		namespace,
		zone)

	_, err := d.dcliClient.RunDCLICommand(cmd.String())

	return err
}

// GetZonesBoundWithSupervisor gets all zones bound with a supervisor.
func (d *wcpDcliClient) GetZonesBoundWithSupervisor(
	supervisorID string) (ZoneList, error) {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones bindings list --supervisor %s %s",
		namespaceManagementSupervisors,
		showUnreleased,
		supervisorID,
		jsonFormatter)

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return ZoneList{}, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	retVal := ZoneList{}
	if err = json.Unmarshal(resp, &retVal); err != nil {
		return ZoneList{}, err
	}

	return retVal, err
}

func (d *wcpDcliClient) CreateZoneBindingsWithSupervisor(supervisorID string, zoneBindingSpecs []ZoneBindingSpecs) error {
	specs, err := json.Marshal(zoneBindingSpecs)
	Expect(err).NotTo(HaveOccurred())

	zoneBindingSpecString := string(specs)

	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones bindings create --supervisor %s --specs '%s'",
		namespaceManagementSupervisors,
		showUnreleased,
		supervisorID,
		zoneBindingSpecString)

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteZoneBindingsWithSupervisor(supervisorID string, zone string) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones bindings delete --supervisor %s %s",
		namespaceManagementSupervisors,
		showUnreleased,
		supervisorID,
		jsonFormatter)

	fmt.Fprintf(&cmd, " --zone '%s'", zone)

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// UpdateZoneBindingsWithVMReservation updates the zone bindings with the given VM reservations
// and waits for the zone to be ready after the update.
func (d *wcpDcliClient) UpdateZoneBindingsWithVMReservation(supervisorID, zoneID string, reservedVMClassToCount map[string]int) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones bindings update --supervisor %s --zone %s --resource-allocation-vm-reservations '[",
		namespaceManagementSupervisors,
		showUnreleased,
		supervisorID,
		zoneID)

	for vmClass, count := range reservedVMClassToCount {
		fmt.Fprintf(&cmd, "{\"reserved_vm_class\": \"%s\", \"count\": %d},", vmClass, count)
	}

	// Remove the last comma in "resource-allocation-vm-reservations" if exists.
	cmdStr := cmd.String()
	cmdStr = strings.TrimSuffix(cmdStr, ",")
	cmdStr += "]'"

	resp, err := d.dcliClient.RunDCLICommand(cmdStr)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	timeout := time.Now().Add(60 * time.Second)
	for time.Now().Before(timeout) {
		zList, err := d.GetZonesBoundWithSupervisor(supervisorID)
		if err != nil {
			return fmt.Errorf("failed to get zones bound with supervisor %s: %w", supervisorID, err)
		}

		for _, z := range zList.Zones {
			if z.Zone == zoneID && z.Status == "READY" {
				return nil
			}
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("zone %s is not ready after update", zoneID)
}

func (d *wcpDcliClient) DeleteNamespace(namespace string) error {
	cmd := fmt.Sprintf("%s delete --namespace %s",
		namespacesInstances,
		namespace,
	)
	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

func (d *wcpDcliClient) GetVirtualMachine(vmMoid string) (VirtualMachineDetails, error) {
	cmd := fmt.Sprintf("%s get --vm %s +formatter json", virtualmachine, vmMoid)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return VirtualMachineDetails{}, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	retVal := VirtualMachineDetails{}
	if err = json.Unmarshal(resp, &retVal); err != nil {
		return VirtualMachineDetails{}, err
	}

	return retVal, err
}

type StorageSpec struct {
	// This is the ID of the storage policy to use.
	Policy string `json:"policy"`
	Limit  int64  `json:"limit,omitempty"`
}

type ZoneSpec struct {
	Name string `json:"name"`
}

type ZoneBindingSpecs struct {
	Zone string `json:"zone"`
	Type string `json:"type"`
}

func (d *wcpDcliClient) SetNamespaceStorageSpecs(namespace string, specs []StorageSpec) error {
	storageSpecBuffer, err := json.Marshal(specs)
	Expect(err).NotTo(HaveOccurred())

	storageSpecString := string(storageSpecBuffer)
	cmd := fmt.Sprintf("%s update --namespace %s --storage-specs '%s'",
		namespacesInstances,
		namespace,
		storageSpecString,
	)
	_, err = d.dcliClient.RunDCLICommand(cmd)

	return err
}

var svcInstallTimeout = 120 * time.Second
var svcInstallInterval = 20 * time.Second

// CreateOrSetClusterSupervisorService creates or sets the Supervisor Service Version in the cluster.
// The function retries on failures until timeout. Service installation is performed by wcpsvc, which
// depends on resources asynchronously reconciled by appplatform-operator. Retries handle the transient
// failures that occur before those resources become ready.
func (d *wcpDcliClient) CreateOrSetClusterSupervisorService(op DcliOperationType, cluster, serviceID, version string, serviceConfig map[string]string) error {
	cmd := fmt.Sprintf("%s %s --cluster %s --supervisor-service %s --version %s",
		namespaceManagementClusterServices, op.String(), cluster, serviceID, version)

	if serviceConfig != nil {
		configJSON, err := json.Marshal(serviceConfig)
		if err != nil {
			return fmt.Errorf("error converting service_config to json %w", err)
		}

		cmd = fmt.Sprintf("%s --service-config '%s'", cmd, string(configJSON))
	}

	var (
		resp    []byte
		lastErr error
	)

	timeout := time.Now().Add(svcInstallTimeout)
	for time.Now().Before(timeout) {
		resp, lastErr = d.dcliClient.RunDCLICommand(cmd)
		if lastErr != nil {
			// Continue retrying on error
			time.Sleep(svcInstallInterval)
			continue
		}
		// Success, stop retrying
		return nil
	}

	if lastErr != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     lastErr,
		}
	}

	return nil
}

// CreateOrSetClusterSupervisorServiceWithYamlConfig creates or sets the Supervisor Service Version in the cluster with YAML Config.
// The function retries on failures until timeout. Service installation is performed by wcpsvc, which
// depends on resources asynchronously reconciled by appplatform-operator. Retries handle the transient
// failures that occur before those resources become ready.
func (d *wcpDcliClient) CreateOrSetClusterSupervisorServiceWithYamlConfig(op DcliOperationType, cluster, serviceID, version, yamlServiceConfig string) error {
	cmd := fmt.Sprintf("%s %s --cluster %s --supervisor-service %s --version %s --yaml-service-config %s",
		namespaceManagementClusterServices, op.String(), cluster, serviceID, version, yamlServiceConfig)

	var (
		resp    []byte
		lastErr error
	)

	timeout := time.Now().Add(svcInstallTimeout)
	for time.Now().Before(timeout) {
		resp, lastErr = d.dcliClient.RunDCLICommand(cmd)
		if lastErr != nil {
			// Continue retrying on error
			time.Sleep(svcInstallInterval)
			continue
		}
		// Success, stop retrying
		return nil
	}

	if lastErr != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     lastErr,
		}
	}

	return nil
}

// EnableV1SupervisorService enables PSP supervisor service in the cluster using v1 "set" API.
func (d *wcpDcliClient) EnableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error {
	return setV1SupervisorService(true, cluster, serviceID, version, serviceConfig, d.dcliClient)
}

// DisableV1SupervisorService disables PSP supervisor service in the cluster using v1 "set" API.
func (d *wcpDcliClient) DisableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error {
	return setV1SupervisorService(false, cluster, serviceID, version, serviceConfig, d.dcliClient)
}

// setV1SupervisorService enables or disables PSP supervisor service.
func setV1SupervisorService(enabled bool, cluster, serviceID string, version *string, serviceConfig map[string]string, dcliClient dcli.DCLICommandRunner) error {
	cmd := fmt.Sprintf("%s %s set --enabled %t --cluster %s --service-id %s",
		namespaceServices, showUnreleased, enabled, cluster, serviceID) // the v1 "set" API is an internal API, thus requires +show

	// v1alpha1 PSP services are *known* to only include a single version, so the version parameter can be omitted.
	if version != nil {
		cmd = fmt.Sprintf("%s --version '%s'", cmd, *version)
	}

	if serviceConfig != nil {
		configJSON, err := json.Marshal(serviceConfig)
		if err != nil {
			return fmt.Errorf("error converting service_config to json %w", err)
		}

		cmd = fmt.Sprintf("%s --service-config '%s'", cmd, string(configJSON))
	}

	resp, err := dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// DeleteClusterSupervisorService deletes the Supervisor Service from the cluster.
func (d *wcpDcliClient) DeleteClusterSupervisorService(cluster, serviceID string) error {
	cmd := fmt.Sprintf("%s delete --cluster %s --supervisor-service %s",
		namespaceManagementClusterServices, cluster, serviceID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// DeleteSupervisorService removes a Service.
func (d *wcpDcliClient) DeleteSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s delete --supervisor-service %s", namespaceManagementServices, serviceID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// DeleteSupervisorServiceVersion deletes a specific version of a Supervisor Service.
func (d *wcpDcliClient) DeleteSupervisorServiceVersion(serviceID, version string) error {
	cmd := fmt.Sprintf("%s versions delete --supervisor-service %s --version %s", namespaceManagementServices, serviceID, version)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) ActivateSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s activate --supervisor-service %s", namespaceManagementServices, serviceID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeactivateSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s deactivate --supervisor-service %s", namespaceManagementServices, serviceID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) ActivateSupervisorServiceVersion(serviceID, versionID string) error {
	cmd := fmt.Sprintf("%s versions activate --supervisor-service %s --version %s", namespaceManagementServices, serviceID, versionID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeactivateSupervisorServiceVersion(serviceID, versionID string) error {
	cmd := fmt.Sprintf("%s versions deactivate --supervisor-service %s --version %s", namespaceManagementServices, serviceID, versionID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// RegisterCarvelService creates a Carvel service with one version.
func (d *wcpDcliClient) RegisterCarvelService(carvelYaml []byte) error {
	content := b64.StdEncoding.EncodeToString(carvelYaml)
	cmd := fmt.Sprintf("%s create --carvel-spec-version-spec-content %s "+
		" +formatter json", namespaceManagementServices, content)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// RegisterCarvelServiceVersion creates a Carvel service version.
func (d *wcpDcliClient) RegisterCarvelServiceVersion(serviceID string, carvelYaml []byte) error {
	content := b64.StdEncoding.EncodeToString(carvelYaml)
	cmd := fmt.Sprintf("%s versions create --supervisor-service %s "+
		" --carvel-spec-content %s "+
		" +formatter json ", namespaceManagementServices, serviceID, content)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// Precheck initiates a pre-check task for a Supervisor Service version against a Supervisor.
func (d *wcpDcliClient) ServicePrecheck(serviceID, version, supervisor string) error {
	cmd := fmt.Sprintf("%s precheck --supervisor %s --supervisor-service %s --target-version %s"+
		" +formatter json ", namespaceMgmtSupervisorsSvServices, supervisor, serviceID, version)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// NewClientUsingKubeconfig uses a kubeconfig pointed at a supervisor cluster context with an
//
//	administrator user to return a WCP client that points at the VC corresponding to that
//	supervisor cluster.
func NewClientUsingKubeconfig(ctx context.Context, kubeconfig string) WorkloadManagementAPI {
	return NewClientUsingKubeconfigFile(ctx, kubeconfig)
}

func NewClientUsingKubeconfigFile(ctx context.Context, path string) WorkloadManagementAPI {
	vCenterHostname := vcenter.GetVCPNIDFromKubeconfigFile(ctx, path)
	Expect(vCenterHostname).NotTo(Equal(""), "Unable to determine VC PNID")

	// TODO (GCM-3023) Figure out how to get these from E2E test config.
	//  Hardcoding is a short term fix until then.
	vCenterAdminUser := testbed.AdminUsername
	vCenterAdminPassword := testbed.AdminPassword

	wcpClient, err := NewWCPAPIClient(vCenterHostname, vCenterAdminUser, vCenterAdminPassword, testbed.RootUsername, testbed.RootPassword)
	Expect(err).NotTo(HaveOccurred())

	return wcpClient
}

// NewClientUsingKubeconfigWithCredentials uses a kubeconfig pointed at a supervisor cluster context with an
//
//	administrator user to return a WCP client that points at the VC corresponding to that
//	supervisor cluster.
func NewClientUsingKubeconfigWithCredentials(ctx context.Context, kubeconfig string, user string, password string) WorkloadManagementAPI {
	return NewClientUsingKubeconfigFileWithCredentials(ctx, kubeconfig, user, password)
}

func NewClientUsingKubeconfigFileWithCredentials(ctx context.Context, path string, sshUser string, sshPassword string) WorkloadManagementAPI {
	vCenterHostname := vcenter.GetVCPNIDFromKubeconfigFile(ctx, path)
	Expect(vCenterHostname).NotTo(Equal(""), "Unable to determine VC PNID")
	wcpClient, err := NewWCPAPIClient(vCenterHostname, testbed.AdminUsername, testbed.AdminPassword, sshUser, sshPassword)
	Expect(err).NotTo(HaveOccurred())

	return wcpClient
}

func generateVirtualDevicesCommand(devices VirtualDevices) string {
	var result strings.Builder
	if devices.VGPUDevices != nil {
		result.WriteString(" --devices-vgpu-devices '[")
		for index, val := range devices.VGPUDevices {
			_, _ = fmt.Fprintf(&result, `{"profile_name":%q}`, val.ProfileName)
			if index != len(devices.VGPUDevices)-1 {
				result.WriteString(",")
			}
		}

		result.WriteString("]' ")
	}

	if devices.DynamicDirectPathIODevices != nil {
		result.WriteString(" --devices-dynamic-direct-path-io-devices '[")
		for index, val := range devices.DynamicDirectPathIODevices {
			_, _ = fmt.Fprintf(
				&result,
				`{"vendor_id":%d,"device_id":%d,"custom_label":%q}`,
				val.VendorID, val.DeviceID, val.CustomLabel,
			)
			if index != len(devices.DynamicDirectPathIODevices)-1 {
				result.WriteString(",")
			}
		}

		result.WriteString("]' ")
	}

	return result.String()
}

func generateInstanceStorageCommand(instanceStorage InstanceStorage) (string, error) {
	var result string

	if len(instanceStorage.Volumes) > 0 && instanceStorage.StoragePolicy != "" {
		volumes, err := json.Marshal(instanceStorage.Volumes)
		if err != nil {
			return "", err
		}

		result = fmt.Sprintf(" --instance-storage-volumes '%s' --instance-storage-policy \"%s\"",
			string(volumes), instanceStorage.StoragePolicy)
	}

	return result, nil
}

// GenerateVMClassSpecCmd is a helper function to create cmd for VMClass spec fields.
func GenerateVMClassSpecCmd(vmClassSpec VMClassSpec) (string, error) {
	var cmd string
	if vmClassSpec.CPUCount != nil {
		cmd += fmt.Sprintf(" --cpu-count %d", *vmClassSpec.CPUCount)
	}

	if vmClassSpec.MemoryMB != nil {
		cmd += fmt.Sprintf(" --memory-mb %d", *vmClassSpec.MemoryMB)
	}

	if vmClassSpec.CPUReservation != nil {
		cmd += fmt.Sprintf(" --cpu-reservation %d", *vmClassSpec.CPUReservation)
	}

	if vmClassSpec.MemoryReservation != nil {
		cmd += fmt.Sprintf(" --memory-reservation %d", *vmClassSpec.MemoryReservation)
	}

	if vmClassSpec.Description != nil {
		cmd += fmt.Sprintf(" --description \"%s\"", *vmClassSpec.Description)
	}

	if vmClassSpec.ConfigSpec != nil {
		var w bytes.Buffer

		enc := types.NewJSONEncoder(&w)

		err := enc.Encode(vmClassSpec.ConfigSpec)
		if err != nil {
			return "", err
		}

		cmd += fmt.Sprintf(
			` --config-spec '%s'`,
			strings.TrimSuffix(w.String(), "\n"),
		)
	}

	cmd += generateVirtualDevicesCommand(vmClassSpec.Devices)

	commandIS, err := generateInstanceStorageCommand(vmClassSpec.InstanceStorage)
	if err != nil {
		return "", err
	}

	cmd += commandIS

	return cmd, nil
}

func (d *wcpDcliClient) GetVMClassInfo(vmClass string) (VMClassInfo, error) {
	operation := fmt.Sprintf("get --vm-class %s", vmClass)
	s := []string{namespaceManagementVMClassServices, operation, jsonFormatter}
	cmd := strings.Join(s, " ")

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return VMClassInfo{}, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	dec := types.NewJSONDecoder(bytes.NewReader(resp))

	var obj VMClassInfo
	if err := dec.Decode(&obj); err != nil {
		return VMClassInfo{}, err
	}

	return obj, nil
}

func (d *wcpDcliClient) ListContentLibraries() ([]string, error) {
	operation := "list"
	s := []string{dcliContentPrefix, operation, jsonFormatter}
	cmd := strings.Join(s, " ")

	var retval []string

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retval)

	return retval, err
}

func (d *wcpDcliClient) GetContentLibrary(id string) (ContentLibraryInfo, error) {
	cmd := fmt.Sprintf("%s %s get --library-id \"%s\" %s", showUnreleased, dcliContentPrefix, id, jsonFormatter)

	retVal := ContentLibraryInfo{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) FetchContentLibraryIDByName(name string, libraries []string) (string, error) {
	operation := "get"
	for _, libraryID := range libraries {
		s := []string{dcliContentPrefix, operation, "--library-id", libraryID, jsonFormatter}
		cmd := strings.Join(s, " ")
		retVal := ContentLibraryInfo{}

		err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)
		if err == nil {
			if retVal.Name == name {
				return libraryID, nil
			}
		} else {
			return "", err
		}
	}

	return "", nil
}

func (d *wcpDcliClient) ListVMClasses() ([]VMClassInfo, error) {
	operation := "list"
	s := []string{namespaceManagementVMClassServices, operation, jsonFormatter}
	cmd := strings.Join(s, " ")

	var retVal []VMClassInfo

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) CreateVMClass(createSpec VMClassSpec) error {
	// CPUCount and MemoryMB are compulsory fields for VMClass creation.
	Expect(createSpec.CPUCount).NotTo(Equal(BeNil()))
	Expect(createSpec.MemoryMB).NotTo(Equal(BeNil()))
	operation := fmt.Sprintf("create --id %s", createSpec.ID)

	vmClassSpecCmd, err := GenerateVMClassSpecCmd(createSpec)
	if err != nil {
		return err
	}

	s := []string{namespaceManagementVMClassServices, operation, vmClassSpecCmd, jsonFormatter}
	cmd := strings.Join(s, " ")

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) UpdateVMClass(updateSpec VMClassSpec) error {
	operation := fmt.Sprintf("update --vm-class %s", updateSpec.ID)

	vmClassSpecCmd, err := GenerateVMClassSpecCmd(updateSpec)
	if err != nil {
		return err
	}

	s := []string{namespaceManagementVMClassServices, operation, vmClassSpecCmd, jsonFormatter}
	cmd := strings.Join(s, " ")

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteVMClass(vmClass string) error {
	operation := fmt.Sprintf("delete --vm-class %s", vmClass)
	s := []string{namespaceManagementVMClassServices, operation, jsonFormatter}
	cmd := strings.Join(s, " ")
	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

func (d *wcpDcliClient) UpdateNamespaceVMServiceSpec(namespaceName string, updateSpec NamespaceUpdateVMserviceSpec) error {
	var (
		vmClassSpec          string
		contentLibrariesSpec strings.Builder
	)

	if updateSpec.VMClasses != nil {
		for _, vmClass := range *updateSpec.VMClasses {
			vmClassSpec += fmt.Sprintf("--vm-service-spec-vm-classes %s ", vmClass)
		}
	}

	if updateSpec.ContentLibraries != nil {
		for _, cl := range *updateSpec.ContentLibraries {
			_, _ = fmt.Fprintf(&contentLibrariesSpec, "--vm-service-spec-content-libraries %s ", cl)
		}
	}

	operation := fmt.Sprintf("update --namespace %s +show-unreleased", namespaceName)
	s := []string{namespacesInstances, operation, vmClassSpec, contentLibrariesSpec.String()}
	cmd := strings.Join(s, " ")

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) ListDatastores() ([]Datastore, error) {
	cmd := fmt.Sprintf("%s %s datastore list", jsonFormatter, dcliVCenterPrefix)
	retVal := []Datastore{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) AddCLTrustedCertificate(trustedCertificate string) (string, error) {
	cmd := fmt.Sprintf("%s com vmware content trustedcertificates create --cert-text '%s'", jsonFormatter, trustedCertificate)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), err
}

func (d *wcpDcliClient) DeleteCLTrustedCertificate(trustedCertificateID string) error {
	cmd := fmt.Sprintf("%s com vmware content trustedcertificates delete --certificate %s", jsonFormatter, trustedCertificateID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) ListCLSecurityPolicies() ([]SecurityPolicyInfo, error) {
	cmd := fmt.Sprintf("%s com vmware content securitypolicies list", jsonFormatter)
	retVal := []SecurityPolicyInfo{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) CreateLocalContentLibrary(name string, storageBackings StorageBackingInfo) (string, error) {
	storageBackingsJSON, err := json.Marshal(storageBackings.StorageBackings)
	Expect(err).NotTo(HaveOccurred())

	cmd := fmt.Sprintf("%s %s create --name \"%s\" --storage-backings '%s'", showUnreleased, dcliContentLocalPrefix, name, storageBackingsJSON)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) DeleteLocalContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s delete --library-id %s", showUnreleased, dcliContentLocalPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteLocalContentLibraryByForce(id string) error {
	cmd := fmt.Sprintf("%s %s forcedelete --library-id %s", showUnreleased, dcliContentLocalPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) UpdateContentLibrary(contentLibraryID, description string, securityPolicyID string) error {
	cmd := fmt.Sprintf("%s %s update --library-id %s --description \"%s\"", showUnreleased, dcliContentPrefix, contentLibraryID, description)
	if securityPolicyID != "" {
		cmd = fmt.Sprintf("%s --security-policy-id %s", cmd, securityPolicyID)
	}

	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) UpdateLocalContentLibrary(contentLibraryID string, enablePublishing bool) error {
	cmd := fmt.Sprintf("%s %s update --library-id %s --publish-info-published %t", showUnreleased, dcliContentLocalPrefix, contentLibraryID, enablePublishing)

	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) CreateSubscribedContentLibrary(name, subscriptionURL, thumbprint string, onDemand bool, storageBackings StorageBackingInfo) (string, error) {
	storageBackingsJSON, err := json.Marshal(storageBackings.StorageBackings)
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("%s %s create --name \"%s\" --subscription-info-subscription-url \"%s\" --subscription-info-on-demand %t --subscription-info-authentication-method %s --subscription-info-automatic-sync-enabled %t --storage-backings '%s'",
		showUnreleased, dcliContentSubscribedPrefix, name, subscriptionURL, onDemand, "NONE", false, storageBackingsJSON)
	if thumbprint != "" {
		cmd += fmt.Sprintf(" --subscription-info-ssl-thumbprint \"%s\"", thumbprint)
	}

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) SyncSubscribedContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s sync --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteSubscribedContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s delete --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteSubscribedContentLibraryByForce(id string) error {
	cmd := fmt.Sprintf("%s %s forcedelete --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) CreateContentLibraryItem(contentLibraryID, name string, itemType ContentLibraryItemType) (string, error) {
	cmd := fmt.Sprintf("%s %s item create --library-id %s --name %s --type %s", showUnreleased, dcliContentPrefix, contentLibraryID, name, string(itemType))

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) CreateContentLibraryOVFTemplateItemByPull(contentLibraryID, name, ovfFileURL string) (string, error) {
	cmd := fmt.Sprintf("%s %s item create --library-id %s --name %s --type %s", showUnreleased, dcliContentPrefix, contentLibraryID, name, "ovf")

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	libraryItemID := strings.TrimSpace(string(resp))

	// create update session to add OVF template files by URL
	updateCmd := fmt.Sprintf("%s %s item updatesession create --library-item-id %s", showUnreleased, dcliContentPrefix, libraryItemID)

	resp, err = d.dcliClient.RunDCLICommand(updateCmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	updateSessionID := strings.TrimSpace(string(resp))

	probeCmd := fmt.Sprintf("%s %s item updatesession file probe --uri %s +formatter json", showUnreleased, dcliContentPrefix, ovfFileURL)
	probeResult := ProbeResult{}

	err = d.dcliClient.RunCommandAndUnmarshalJSONResult(probeCmd, &probeResult)
	if err != nil {
		return "", DcliError{
			rawResponse: "",
			baseErr:     err,
		}
	}

	sslThumbprint := strings.TrimSpace(probeResult.SSLThumbprint)

	fileName := path.Base(ovfFileURL)
	fileCmd := fmt.Sprintf("%s %s item updatesession file add --name %s --source-type %s --update-session-id %s --source-endpoint-uri %s --source-endpoint-ssl-certificate-thumbprint %s",
		showUnreleased, dcliContentPrefix, fileName, "PULL", updateSessionID, ovfFileURL, sslThumbprint)

	resp, err = d.dcliClient.RunDCLICommand(fileCmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	completeCmd := fmt.Sprintf("%s %s item updatesession complete --update-session-id %s", showUnreleased, dcliContentPrefix, updateSessionID)

	resp, err = d.dcliClient.RunDCLICommand(completeCmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return libraryItemID, nil
}

func (d *wcpDcliClient) ListContentLibraryItems(contentLibraryID string) ([]string, error) {
	cmd := fmt.Sprintf("%s %s item list --library-id %s", showUnreleased, dcliContentPrefix, contentLibraryID)
	cmd += " " + jsonFormatter

	var retVal []string

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetContentLibraryItem(id string) (ContentLibraryItemInfo, error) {
	cmd := fmt.Sprintf("%s %s item get --library-item-id %s", showUnreleased, dcliContentPrefix, id)
	cmd += " " + jsonFormatter

	retVal := ContentLibraryItemInfo{}
	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) UpdateContentLibraryItem(id, description string) error {
	cmd := fmt.Sprintf("%s %s item update --library-item-id %s --description \"%s\"", showUnreleased, dcliContentPrefix, id, description)
	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteContentLibraryItem(id string) error {
	cmd := fmt.Sprintf("%s %s item delete --library-item-id %s", showUnreleased, dcliContentPrefix, id)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) AssociateImageRegistryContentLibrariesToNamespace(namespace string, contentLibraries ...ContentLibrarySpec) error {
	if len(contentLibraries) == 0 {
		return fmt.Errorf("expected at least one content library")
	}

	namespaceDetails, err := d.GetNamespaceV2(namespace)
	if err != nil {
		return fmt.Errorf("error getting namespace %s: %w", namespace, err)
	}

	contentLibraries = append(contentLibraries, namespaceDetails.ContentLibraries...)

	cmd := fmt.Sprintf("%s %s update --namespace %s --content-libraries '[", showUnreleased, namespacesInstances, namespace)

	uniqueCLIDs := make(map[string]struct{})
	for _, cl := range contentLibraries {
		if _, ok := uniqueCLIDs[cl.ContentLibrary]; !ok {
			uniqueCLIDs[cl.ContentLibrary] = struct{}{}
			cmd += fmt.Sprintf("{\"content_library\": \"%s\", \"writable\": %t, \"allow_import\": %t, \"resource_naming_strategy\": \"%s\"},", cl.ContentLibrary, cl.Writable, cl.AllowImport, cl.ResourceNamingStrategy)
		}
	}

	cmd = cmd[:len(cmd)-1]
	cmd += "]'"

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DisassociateImageRegistryContentLibrariesFromNamespace(namespace string, contentLibraryIDs ...string) error {
	if len(contentLibraryIDs) == 0 {
		return fmt.Errorf("expected at least one content library ID")
	}

	namespaceDetails, err := d.GetNamespaceV2(namespace)
	if err != nil {
		return fmt.Errorf("error getting namespace %s: %w", namespace, err)
	}

	// Run update with *all existing content libraries, except the provided ones.
	clsToKeep := []ContentLibrarySpec{}

	for _, cl := range namespaceDetails.ContentLibraries {
		match := slices.Contains(contentLibraryIDs, cl.ContentLibrary)

		if !match {
			clsToKeep = append(clsToKeep, cl)
		}
	}

	cmd := fmt.Sprintf("%s %s update --namespace %s --content-libraries '[", showUnreleased, namespacesInstances, namespace)

	if len(clsToKeep) > 0 {
		for _, cl := range clsToKeep {
			cmd += fmt.Sprintf("{\"content_library\": \"%s\", \"writable\": %t, \"allow_import\": %t, \"resource_naming_strategy\": \"%s\"},", cl.ContentLibrary, cl.Writable, cl.AllowImport, cl.ResourceNamingStrategy)
		}

		cmd = cmd[:len(cmd)-1]
	}

	cmd += "]'"

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) AssociateContentLibrariesToCluster(cluster string, contentLibraries ...ClusterContentLibrarySpec) error {
	if len(contentLibraries) == 0 {
		return fmt.Errorf("expected at least one content library ID")
	}

	// Get cluster details to get the existing content libraries, so we ensure we retain them after update.
	clusterDetails, err := d.GetCluster(cluster)
	if err != nil {
		return fmt.Errorf("error getting current content libraries on cluster %s: %w", cluster, err)
	}

	// Only retain existing CLs.
	for _, clSpec := range clusterDetails.ContentLibraries {
		_, err := d.GetContentLibrary(clSpec.ContentLibrary)
		if err == nil {
			contentLibraries = append(contentLibraries, clSpec)
		}
	}

	cmd := fmt.Sprintf("%s %s clusters update --cluster %s --content-libraries '[", showUnreleased, namespaceManagement, cluster)

	uniqueCLIDs := make(map[string]struct{})
	for _, cl := range contentLibraries {
		// Avoid duplicate content library error in running the dcli command.
		if _, ok := uniqueCLIDs[cl.ContentLibrary]; !ok {
			uniqueCLIDs[cl.ContentLibrary] = struct{}{}

			supervisorServices := "[]"
			if len(cl.SupervisorServices) > 0 {
				supervisorServices = fmt.Sprintf("[\"%s\"]", strings.Join(cl.SupervisorServices, "\",\""))
			}

			cmd += fmt.Sprintf("{\"content_library\": \"%s\", \"supervisor_services\": %s, \"resource_naming_strategy\": \"%s\"},", cl.ContentLibrary, supervisorServices, cl.ResourceNamingStrategy)
		}
	}

	cmd = cmd[:len(cmd)-1]
	cmd += "]'"

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DisassociateContentLibrariesFromCluster(cluster string, contentLibraryIDs ...string) error {
	if len(contentLibraryIDs) == 0 {
		return fmt.Errorf("expected at least one content library ID")
	}

	// Get cluster details to get the existing content libraries, so we ensure we retain them after update.
	clusterDetails, err := d.GetCluster(cluster)
	if err != nil {
		return fmt.Errorf("error getting current content libraries on cluster %s: %w", cluster, err)
	}

	// Run update with all existing content libraries, except the provided ones.
	var clsToKeep []ClusterContentLibrarySpec

	for _, clSpec := range clusterDetails.ContentLibraries {
		match := false

		for _, delID := range contentLibraryIDs {
			if clSpec.ContentLibrary == delID {
				match = true
			}
		}

		if !match {
			// Make sure that the ContentLibrary *exists* to update with.
			_, err := d.GetContentLibrary(clSpec.ContentLibrary)
			if err == nil {
				clsToKeep = append(clsToKeep, clSpec)
			}
		}
	}

	// Run an update, omitting the passed in contentLibraryIDs, but keeping other added content libraries
	// (to preserve state away from tests).
	cmd := fmt.Sprintf("%s %s clusters update --cluster %s --content-libraries '[", showUnreleased, namespaceManagement, cluster)

	if len(clsToKeep) > 0 {
		for _, cl := range clsToKeep {
			supervisorServices := "[]"
			if len(cl.SupervisorServices) > 0 {
				supervisorServices = fmt.Sprintf("[\"%s\"]", strings.Join(cl.SupervisorServices, "\",\""))
			}

			cmd += fmt.Sprintf("{\"content_library\": \"%s\", \"supervisor_services\": %s, \"resource_naming_strategy\": \"%s\"},", cl.ContentLibrary, supervisorServices, cl.ResourceNamingStrategy)
		}

		cmd = cmd[:len(cmd)-1]
	}

	cmd += "]'"

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) RegisterVM(namespace, vmMoID string) (string, error) {
	cmd := fmt.Sprintf("%s %s registervm --namespace %s --vm %s", showUnreleased, namespacesInstances, namespace, vmMoID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) UpdateClusterProxyConfig(clusterMoid string, proxyConfig ClusterProxyConfig) error {
	flagMap := map[string]string{
		"cluster-proxy-config-http-proxy-config":  proxyConfig.HTTPProxyConfig,
		"cluster-proxy-config-https-proxy-config": proxyConfig.HTTPSProxyConfig,
		"cluster-proxy-config-no-proxy-config":    strings.Join(proxyConfig.NoProxyConfig, ","),
		"cluster-proxy-config-tls-root-ca-bundle": proxyConfig.TLSRootCABundle,
	}

	var cmd strings.Builder
	_, _ = fmt.Fprintf(&cmd, "%s clusters update --cluster %v --cluster-proxy-config-proxy-settings-source %v", namespaceManagement, clusterMoid, proxyConfig.ProxySettingsSource)

	switch proxyConfig.ProxySettingsSource {
	case VcInherited:
		// For VcInherited, we don't need to pass any other flags
	case ClusterConfigured:
		// The only time we need to pass flags is when the source is ClusterConfigured
		for flag, value := range flagMap {
			// Filter out empty proxies, they cause an error in dcli
			if value != "" {
				flagStr := fmt.Sprintf(" --%s '%s'", flag, value)
				cmd.WriteString(flagStr)
			}
		}
	case None:
		// For None, we don't need to pass any other flags
	default:
		return fmt.Errorf("invalid proxy source: %s", proxyConfig.ProxySettingsSource)
	}

	if resp, err := d.dcliClient.RunDCLICommand(cmd.String()); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) GetClusterProxyConfig(clusterMoid string) (ClusterProxyConfig, error) {
	details, err := d.GetCluster(clusterMoid)
	if err != nil {
		return ClusterProxyConfig{}, err
	}

	return details.ClusterProxyConfig, nil
}

func (d *wcpDcliClient) CreateContainerImageRegistry(supervisorID string, registry ContainerImageRegistry) (ContainerImageRegistryInfo, error) {
	cmd := fmt.Sprintf("%s containerimageregistries create --name '%v' --supervisor '%v' ", namespaceManagementSupervisors, registry.Name, supervisorID)

	if registry.ImageRegistry.CertificateChain != "" {
		cmd += fmt.Sprintf("--image-registry-certificate-chain '%v' ", registry.ImageRegistry.CertificateChain)
	}

	if registry.ImageRegistry.Password != "" {
		cmd += fmt.Sprintf("--image-registry-password '%v' ", registry.ImageRegistry.Password)
	}

	if registry.ImageRegistry.Username != "" {
		cmd += fmt.Sprintf("--image-registry-username '%v' ", registry.ImageRegistry.Username)
	}

	if registry.ImageRegistry.Port != 0 {
		cmd += fmt.Sprintf("--image-registry-port %v ", registry.ImageRegistry.Port)
	}

	cmd += fmt.Sprintf("--image-registry-hostname '%v' ", registry.ImageRegistry.Hostname)
	cmd += fmt.Sprintf("--default-registry '%v' ", registry.DefaultRegistry)

	cmd += jsonFormatter

	result := ContainerImageRegistryInfo{}

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &result)
	if err != nil {
		return result, DcliError{
			rawResponse: "",
			baseErr:     err,
		}
	}

	return result, nil
}

func (d *wcpDcliClient) GetContainerImageRegistry(supervisorID, registryName string) (ContainerImageRegistryInfo, error) {
	registries, err := d.listContainerImageRegistries(supervisorID)
	if err != nil {
		return ContainerImageRegistryInfo{}, err
	}

	for _, registry := range registries {
		if registry.Name == registryName {
			return registry, nil
		}
	}

	return ContainerImageRegistryInfo{}, fmt.Errorf("image registry with name %s not found", registryName)
}

func (d *wcpDcliClient) listContainerImageRegistries(supervisorID string) ([]ContainerImageRegistryInfo, error) {
	cmd := fmt.Sprintf("%s containerimageregistries list --supervisor '%v' +formatter json", namespaceManagementSupervisors, supervisorID)

	result := []ContainerImageRegistryInfo{}

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &result)
	if err != nil {
		return result, DcliError{
			rawResponse: "",
			baseErr:     err,
		}
	}

	return result, nil
}

func (d *wcpDcliClient) DeleteContainerImageRegistry(supervisorID, registryID string) error {
	cmd := fmt.Sprintf("%s containerimageregistries delete --container-image-registry '%v' --supervisor '%v' ", namespaceManagementSupervisors, registryID, supervisorID)
	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) CreateKeyProvider(provider string) error {
	cmd := fmt.Sprintf("%s kms providers create --provider %s", cryptoManager, provider)
	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) DeleteKeyProvider(provider string) error {
	cmd := fmt.Sprintf("%s kms providers delete --provider %s", cryptoManager, provider)
	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// AssignLicenseEntitlement assigns the given license entitlement to the cluster using the dcli cis command.
func (d *wcpDcliClient) AssignLicenseEntitlement(signedEntitlement string) (string, error) {
	cmd := fmt.Sprintf("%s %s entitlements update-task --other-vc-usages '[]' --configuration '%s'", showUnreleased, dcliCISLicenseEntitlementPrefix, signedEntitlement)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) GetSupervisorLoadBalancerProvider(supervisorID string) (string, error) {
	cmd := fmt.Sprintf("%s --supervisor %s %s", dcliSupervisorNetworkEdges, supervisorID, jsonFormatter)

	var result struct {
		Edges []struct {
			Provider string `json:"provider"`
		} `json:"edges"`
	}

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &result)
	if err != nil {
		return "", DcliError{
			rawResponse: "",
			baseErr:     fmt.Errorf("failed to run DCLI command or parse result: %w", err),
		}
	}

	if len(result.Edges) == 0 || result.Edges[0].Provider == "" {
		return "", fmt.Errorf("no provider found in command output")
	}

	return result.Edges[0].Provider, nil
}

func (d *wcpDcliClient) UpdateWorkerDNS(clusterMoid string, dnsServerIPs ...string) error {
	if len(dnsServerIPs) == 0 {
		// nothing to update
		return nil
	}

	var dnsServerIPArgs strings.Builder

	for _, dnsServerIP := range dnsServerIPs {
		_, _ = fmt.Fprintf(&dnsServerIPArgs, " --worker-dns %s", dnsServerIP)
	}

	cmd := fmt.Sprintf("%s clusters update --cluster %s%s", namespaceManagement, clusterMoid, dnsServerIPArgs.String())

	if resp, err := d.dcliClient.RunDCLICommand(cmd); err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

func (d *wcpDcliClient) GetWorkerDNS(clusterMoid string) ([]string, error) {
	cmd := fmt.Sprintf("%s clusters get --cluster %s %s", namespaceManagement, clusterMoid, jsonFormatter)

	var result struct {
		WorkerDNS []string `json:"worker_DNS"`
	}

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &result)
	if err != nil {
		return nil, DcliError{
			rawResponse: "",
			baseErr:     fmt.Errorf("failed to run DCLI command or parse result: %w", err),
		}
	}

	return result.WorkerDNS, nil
}

// CreateNamespaceNetwork creates a new vSphere network for use in WCP namespaces.
// If the network already exists, this function returns nil (idempotent operation).
func (d *wcpDcliClient) CreateNamespaceNetwork(config NamespaceNetworkConfig) error {
	cmd := fmt.Sprintf(
		"%s create "+
			"--cluster %s "+
			"--network %s "+
			"--network-provider %s "+
			"--vsphere-network-mode %s "+
			"--vsphere-network-ip-assignment-mode %s "+
			"--vsphere-network-portgroup %s "+
			"--vsphere-network-gateway '%s' "+
			"--vsphere-network-subnet-mask '%s' %s",
		namespaceManagementNetworks,
		config.Cluster,
		config.Network,
		config.NetworkProvider,
		config.VsphereNetworkMode,
		config.VsphereNetworkIPAssignmentMode,
		config.VsphereNetworkPortgroup,
		config.VsphereNetworkGateway,
		config.VsphereNetworkSubnetMask,
		jsonFormatter,
	)

	stdout, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		// If the network already exists, treat it as success (idempotent operation)
		if strings.Contains(string(stdout), "errors.AlreadyExists") {
			return nil
		}

		return err
	}

	return nil
}

// GetNamespaceNetwork retrieves details about a vSphere network configured for WCP.
func (d *wcpDcliClient) GetNamespaceNetwork(cluster, network string) (map[string]any, error) {
	cmd := fmt.Sprintf(
		"%s get --cluster %s --network %s %s",
		namespaceManagementNetworks,
		cluster,
		network,
		jsonFormatter,
	)

	var result map[string]any

	err := d.dcliClient.RunCommandAndUnmarshalJSONResult(cmd, &result)

	return result, err
}

// UpdateNamespaceWithNetworks adds one or more networks to an existing namespace.
func (d *wcpDcliClient) UpdateNamespaceWithNetworks(namespaceName, networkProvider, networkToAdd string) error {
	cmd := fmt.Sprintf(
		"%s --namespace %s "+
			"--network-spec-network-provider %s "+
			"--network-spec-vsphere-network-config-networks-to-add %s %s",
		namespacesInstancesUpdate,
		namespaceName,
		networkProvider,
		networkToAdd,
		jsonFormatter,
	)

	_, err := d.dcliClient.RunDCLICommand(cmd)

	return err
}

// CreateTagCategory creates a new tag category with the given name and description.
func (d *wcpDcliClient) CreateTagCategory(name, description string) (string, error) {
	cmd := fmt.Sprintf("com vmware cis tagging category create --cardinality MULTIPLE --name %s --description %s", name, description)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

// CreateTag creates a new tag with the given name, description and category ID.
func (d *wcpDcliClient) CreateTag(name, description, categoryID string) (string, error) {
	cmd := fmt.Sprintf("com vmware cis tagging tag create --name %s --description %s --category-id %s", name, description, categoryID)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

// AssignTagsToHost assigns multiple tags to a host.
func (d *wcpDcliClient) AssignTagsToHost(tagIDs []string, hostID string) error {
	if len(tagIDs) == 0 {
		return nil
	}

	var cmd strings.Builder
	cmd.WriteString("com vmware cis tagging tagassociation attachmultipletagstoobject")

	for _, tagID := range tagIDs {
		fmt.Fprintf(&cmd, " --tag-ids %s", tagID)
	}

	fmt.Fprintf(&cmd, " --id %s --type HostSystem", hostID)

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// CreateComputePolicy creates a compute policy with the given spec and returns the created compute policy ID.
func (d *wcpDcliClient) CreateComputePolicy(spec ComputePolicySpec) (string, error) {
	cmd := fmt.Sprintf("%s %s compute policies create --capability %s --name %s --description \"%s\" --host-tag %s --vm-tag %s",
		dcliVCenterPrefix,
		showUnreleased,
		spec.Capability,
		spec.Name,
		spec.Description,
		spec.HostTagID,
		spec.VMTagID,
	)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return "", DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return strings.TrimSpace(string(resp)), nil
}

// CreateInfraPolicy creates an infrastructure policy with the given spec.
func (d *wcpDcliClient) CreateInfraPolicy(spec InfraPolicySpec) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s infrastructurepolicies create --policy %s --description \"%s\" --compute-policy-id %s ",
		namespaceManagement,
		showUnreleased,
		spec.Name,
		spec.Description,
		spec.ComputePolicyID)

	if spec.EnforcementMode != "" {
		fmt.Fprintf(&cmd, " --enforcement-mode %s", spec.EnforcementMode)
	}

	if spec.MatchGuestIDValue != "" {
		fmt.Fprintf(&cmd, " --match-workload-guest-guest-id-value %s", spec.MatchGuestIDValue)
	}

	if len(spec.MatchWorkloadLabel) > 0 {
		var labels []string

		for key, value := range spec.MatchWorkloadLabel {
			if value == "" {
				labels = append(labels, fmt.Sprintf(`{"key":"%s","operator":"EXISTS"}`, key))
			} else {
				labels = append(labels, fmt.Sprintf(`{"key":"%s","operator":"IS_IN","values":["%s"]}`, key, value))
			}
		}

		fmt.Fprintf(&cmd, " --match-workload-labels '[%s]'", strings.Join(labels, ","))
	}

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// UpdateNamespaceWithInfraPolicies updates a namespace with the given infrastructure policies.
func (d *wcpDcliClient) UpdateNamespaceWithInfraPolicies(namespace string, policyNames ...string) error {
	if len(policyNames) == 0 {
		return nil
	}

	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s update --namespace %s", namespacesInstances, showUnreleased, namespace)

	for _, policyName := range policyNames {
		fmt.Fprintf(&cmd, " --infrastructure-policies %s", policyName)
	}

	resp, err := d.dcliClient.RunDCLICommand(cmd.String())
	if err != nil {
		return DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return nil
}

// ListHostIDs lists all host IDs in the vCenter.
func (d *wcpDcliClient) ListHostIDs() ([]string, error) {
	cmd := fmt.Sprintf("%s %s host list +formatter json", dcliVCenterPrefix, showUnreleased)

	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return nil, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	var hosts []struct {
		Host string `json:"host"`
	}
	if err = json.Unmarshal(resp, &hosts); err != nil {
		return nil, err
	}

	hostIDs := make([]string, len(hosts))
	for i, h := range hosts {
		hostIDs[i] = h.Host
	}

	return hostIDs, nil
}
