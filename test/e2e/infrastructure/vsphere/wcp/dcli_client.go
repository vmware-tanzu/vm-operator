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

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
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
	computePoliciesPrefix              = " compute policies"
	computePolicies                    = dcliVCenterPrefix + computePoliciesPrefix
	computePolicyTagUsages             = computePolicies + " tagusage list "
	virtualmachine                     = dcliVCenterPrefix + "vm"
	virtualmachineComputePolicy        = virtualmachine + computePoliciesPrefix + " get "
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
// consumers can rely on the interface exposed by NewWCPAPIClient/WorkloadManagementAPI.
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

// vmReservation is the DCLI wire form of a VM class reservation.
type vmReservation struct {
	ReservedVMClass string `json:"reserved_vm_class"`
	Count           int    `json:"count"`
}

// vmReservations converts a VM class to reserved count map to its wire form.
// The result is never nil so it always marshals to a JSON array.
func vmReservations(reservedVMClassToCount map[string]int) []vmReservation {
	reservations := make([]vmReservation, 0, len(reservedVMClassToCount))
	for vmClass, count := range reservedVMClassToCount {
		reservations = append(reservations, vmReservation{ReservedVMClass: vmClass, Count: count})
	}

	return reservations
}

// run runs the DCLI command and returns its output. A failure is returned as a
// DcliError so callers can inspect the raw response with errors.As.
func (d *wcpDcliClient) run(cmd string) ([]byte, error) {
	resp, err := d.dcliClient.RunDCLICommand(cmd)
	if err != nil {
		return resp, DcliError{
			rawResponse: string(resp),
			baseErr:     err,
		}
	}

	return resp, nil
}

// runVoid runs the DCLI command and discards its output.
func (d *wcpDcliClient) runVoid(cmd string) error {
	_, err := d.run(cmd)
	return err
}

// runJSON runs the DCLI command and decodes its JSON output into out.
func (d *wcpDcliClient) runJSON(cmd string, out any) error {
	resp, err := d.run(cmd)
	if err != nil {
		return err
	}

	return types.NewJSONDecoder(bytes.NewReader(resp)).Decode(out)
}

// runIgnoringAlreadyExists runs the DCLI command, treating an AlreadyExists
// failure as success so creation is idempotent.
func (d *wcpDcliClient) runIgnoringAlreadyExists(cmd string) error {
	resp, err := d.run(cmd)
	if err != nil && strings.Contains(string(resp), "errors.AlreadyExists") {
		return nil
	}

	return err
}

// retry runs the DCLI command until it succeeds or the timeout expires,
// returning the last error.
func (d *wcpDcliClient) retry(cmd string, interval, timeout time.Duration) error {
	var lastErr error

	_ = wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(context.Context) (bool, error) {
		lastErr = d.runVoid(cmd)
		return lastErr == nil, nil
	})

	return lastErr
}

// shellQuote single-quotes s for a POSIX shell, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// jsonArg marshals v and shell-quotes the result for use as a flag value.
func jsonArg(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return shellQuote(string(b)), nil
}

// Close closes the underlying DCLI command runner.
func (d *wcpDcliClient) Close() error {
	return d.dcliClient.Close()
}

func (d *wcpDcliClient) ListSupervisorSummary() (SupervisorSummaryList, error) {
	cmd := fmt.Sprintf("%s summary list %s", supervisors, jsonFormatter)
	retVal := SupervisorSummaryList{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) ListClusters() ([]WCPClusterDetails, error) {
	cmd := fmt.Sprintf("%s clusters list %s", namespaceManagement, jsonFormatter)
	retVal := []WCPClusterDetails{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetCluster(moID string) (WCPClusterDetails, error) {
	cmd := fmt.Sprintf("%s clusters get --cluster %s %s", namespaceManagement, moID, jsonFormatter)
	retVal := WCPClusterDetails{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) ListNamespaces() ([]NamespaceDetails, error) {
	cmd := fmt.Sprintf("%s list %s", namespacesInstances, jsonFormatter)
	retVal := []NamespaceDetails{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetNamespace(name string) (NamespaceDetails, error) {
	return d.getNamespace("get", name)
}

func (d *wcpDcliClient) GetNamespaceV2(name string) (NamespaceDetails, error) {
	return d.getNamespace("getv2", name)
}

func (d *wcpDcliClient) getNamespace(verb, name string) (NamespaceDetails, error) {
	cmd := fmt.Sprintf("%s %s --namespace %s %s", namespacesInstances, verb, name, jsonFormatter)

	retVal := NamespaceDetails{}
	if err := d.runJSON(cmd, &retVal); err != nil {
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

	// Treat AlreadyExists as success — this can happen on FlakeAttempts retries
	// when the previous attempt created the permission before failing at a later step.
	return d.runIgnoringAlreadyExists(cmd)
}

func (d *wcpDcliClient) RemoveNamespacePermissions(principal Principal, namespace string) error {
	cmd := fmt.Sprintf("%s delete --domain %s --namespace %s --subject %s --type %s",
		namespacesAccess,
		principal.Domain,
		namespace,
		principal.Name,
		principal.Type,
	)
	return d.runVoid(cmd)
}

func (d *wcpDcliClient) CreateNamespace(clusterMoid, namespace string) error {
	cmd := fmt.Sprintf("%s create --cluster %s --namespace %s",
		namespacesInstances,
		clusterMoid,
		namespace,
	)
	return d.runVoid(cmd)
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
		storage, err := jsonArg(storageSpecs)
		if err != nil {
			return "", fmt.Errorf("failed to marshal storage specs: %w", err)
		}

		fmt.Fprintf(&cmd, " --storage-specs %s", storage)
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

				sharedSubnetsJSON, err := jsonArg(sharedSubnets)
				if err != nil {
					return "", fmt.Errorf("failed to marshal shared subnets: %w", err)
				}

				cmd += " --network-spec-vpc-network-shared-subnets " + sharedSubnetsJSON
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

	return d.runIgnoringAlreadyExists(cmd)
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

	return d.runVoid(cmd)
}

// CreateNamespaceWithVMReservation creates a namespace with the given specs and VM reservations.
func (d *wcpDcliClient) CreateNamespaceWithVMReservation(namespace, zone, supervisorID string, storageSpecs []StorageSpec, vmsvcSpec VMServiceSpecDetails, vmClassNameToReservedCount map[string]int) error {
	type zoneSpec struct {
		Name          string          `json:"name"`
		VMReservation []vmReservation `json:"vm_reservations"`
	}

	zones, err := jsonArg([]zoneSpec{{Name: zone, VMReservation: vmReservations(vmClassNameToReservedCount)}})
	if err != nil {
		return err
	}

	storage, err := jsonArg(storageSpecs)
	if err != nil {
		return err
	}

	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s createv2 --namespace %s --supervisor %s --zones %s --storage-specs %s",
		namespacesInstances,
		showUnreleased,
		namespace,
		supervisorID,
		zones,
		storage)

	// Add vm service specs (VM Class and Content Library).
	for _, vmClass := range vmsvcSpec.VMClasses {
		fmt.Fprintf(&cmd, " --vm-service-spec-vm-classes %s", vmClass)
	}

	for _, clID := range vmsvcSpec.ContentLibraries {
		fmt.Fprintf(&cmd, " --vm-service-spec-content-libraries %s", clID)
	}

	return d.runVoid(cmd.String())
}

// ListVSphereZones list all vSphere zones.
func (d *wcpDcliClient) ListVSphereZones() (VSphereZoneList, error) {
	cmd := fmt.Sprintf("%s list %s", zones, jsonFormatter)
	retVal := VSphereZoneList{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

// UpdateNamespaceWithZones binds a namespace with the given namespace scoped Zones.
func (d *wcpDcliClient) UpdateNamespaceWithZones(
	namespace string,
	zones []ZoneSpec) error {
	zoneSpec, err := jsonArg(zones)
	if err != nil {
		return err
	}

	return d.runVoid(fmt.Sprintf("%s %s update --namespace %s --zones %s",
		namespacesInstances, showUnreleased, namespace, zoneSpec))
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

	return d.runVoid(cmd.String())
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

	retVal := ZoneList{}
	if err := d.runJSON(cmd.String(), &retVal); err != nil {
		return ZoneList{}, err
	}

	return retVal, nil
}

func (d *wcpDcliClient) CreateZoneBindingsWithSupervisor(supervisorID string, zoneBindingSpecs []ZoneBindingSpecs) error {
	specs, err := jsonArg(zoneBindingSpecs)
	if err != nil {
		return err
	}

	return d.runVoid(fmt.Sprintf("%s %s zones bindings create --supervisor %s --specs %s",
		namespaceManagementSupervisors, showUnreleased, supervisorID, specs))
}

func (d *wcpDcliClient) DeleteZoneBindingsWithSupervisor(supervisorID string, zone string) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s zones bindings delete --supervisor %s %s",
		namespaceManagementSupervisors,
		showUnreleased,
		supervisorID,
		jsonFormatter)

	fmt.Fprintf(&cmd, " --zone %s", shellQuote(zone))

	return d.runVoid(cmd.String())
}

// UpdateZoneBindingsWithVMReservation updates the zone bindings with the given VM reservations
// and waits for the zone to be ready after the update.
func (d *wcpDcliClient) UpdateZoneBindingsWithVMReservation(supervisorID, zoneID string, reservedVMClassToCount map[string]int) error {
	reservations, err := jsonArg(vmReservations(reservedVMClassToCount))
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("%s %s zones bindings update --supervisor %s --zone %s --resource-allocation-vm-reservations %s",
		namespaceManagementSupervisors, showUnreleased, supervisorID, zoneID, reservations)
	if err := d.runVoid(cmd); err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (bool, error) {
		zList, err := d.GetZonesBoundWithSupervisor(supervisorID)
		if err != nil {
			return false, fmt.Errorf("failed to get zones bound with supervisor %s: %w", supervisorID, err)
		}

		return slices.ContainsFunc(zList.Zones, func(z ZoneDetails) bool {
			return z.Zone == zoneID && z.Status == "READY"
		}), nil
	})
	if err != nil && wait.Interrupted(err) {
		return fmt.Errorf("zone %s is not ready after update", zoneID)
	}

	return err
}

func (d *wcpDcliClient) DeleteNamespace(namespace string) error {
	cmd := fmt.Sprintf("%s delete --namespace %s",
		namespacesInstances,
		namespace,
	)
	return d.runVoid(cmd)
}

func (d *wcpDcliClient) GetVirtualMachine(vmMoid string) (VirtualMachineDetails, error) {
	cmd := fmt.Sprintf("%s get --vm %s %s", virtualmachine, vmMoid, jsonFormatter)

	retVal := VirtualMachineDetails{}
	if err := d.runJSON(cmd, &retVal); err != nil {
		return VirtualMachineDetails{}, err
	}

	return retVal, nil
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
	storageSpecs, err := jsonArg(specs)
	if err != nil {
		return err
	}

	return d.runVoid(fmt.Sprintf("%s update --namespace %s --storage-specs %s",
		namespacesInstances, namespace, storageSpecs))
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
		configJSON, err := jsonArg(serviceConfig)
		if err != nil {
			return fmt.Errorf("error converting service_config to json %w", err)
		}

		cmd = fmt.Sprintf("%s --service-config %s", cmd, configJSON)
	}

	return d.retry(cmd, svcInstallInterval, svcInstallTimeout)
}

// CreateOrSetClusterSupervisorServiceWithYamlConfig creates or sets the Supervisor Service Version in the cluster with YAML Config.
// The function retries on failures until timeout. Service installation is performed by wcpsvc, which
// depends on resources asynchronously reconciled by appplatform-operator. Retries handle the transient
// failures that occur before those resources become ready.
func (d *wcpDcliClient) CreateOrSetClusterSupervisorServiceWithYamlConfig(op DcliOperationType, cluster, serviceID, version, yamlServiceConfig string) error {
	cmd := fmt.Sprintf("%s %s --cluster %s --supervisor-service %s --version %s --yaml-service-config %s",
		namespaceManagementClusterServices, op.String(), cluster, serviceID, version, yamlServiceConfig)

	return d.retry(cmd, svcInstallInterval, svcInstallTimeout)
}

// EnableV1SupervisorService enables PSP supervisor service in the cluster using v1 "set" API.
func (d *wcpDcliClient) EnableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error {
	return d.setV1SupervisorService(true, cluster, serviceID, version, serviceConfig)
}

// DisableV1SupervisorService disables PSP supervisor service in the cluster using v1 "set" API.
func (d *wcpDcliClient) DisableV1SupervisorService(cluster, serviceID string, version *string, serviceConfig map[string]string) error {
	return d.setV1SupervisorService(false, cluster, serviceID, version, serviceConfig)
}

// setV1SupervisorService enables or disables PSP supervisor service.
func (d *wcpDcliClient) setV1SupervisorService(enabled bool, cluster, serviceID string, version *string, serviceConfig map[string]string) error {
	cmd := fmt.Sprintf("%s %s set --enabled %t --cluster %s --service-id %s",
		namespaceServices, showUnreleased, enabled, cluster, serviceID) // the v1 "set" API is an internal API, thus requires +show

	// v1alpha1 PSP services are *known* to only include a single version, so the version parameter can be omitted.
	if version != nil {
		cmd = fmt.Sprintf("%s --version %s", cmd, shellQuote(*version))
	}

	if serviceConfig != nil {
		configJSON, err := jsonArg(serviceConfig)
		if err != nil {
			return fmt.Errorf("error converting service_config to json %w", err)
		}

		cmd = fmt.Sprintf("%s --service-config %s", cmd, configJSON)
	}

	return d.runVoid(cmd)
}

// DeleteClusterSupervisorService deletes the Supervisor Service from the cluster.
func (d *wcpDcliClient) DeleteClusterSupervisorService(cluster, serviceID string) error {
	cmd := fmt.Sprintf("%s delete --cluster %s --supervisor-service %s",
		namespaceManagementClusterServices, cluster, serviceID)

	return d.runVoid(cmd)
}

// DeleteSupervisorService removes a Service.
func (d *wcpDcliClient) DeleteSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s delete --supervisor-service %s", namespaceManagementServices, serviceID)

	return d.runVoid(cmd)
}

// DeleteSupervisorServiceVersion deletes a specific version of a Supervisor Service.
func (d *wcpDcliClient) DeleteSupervisorServiceVersion(serviceID, version string) error {
	cmd := fmt.Sprintf("%s versions delete --supervisor-service %s --version %s", namespaceManagementServices, serviceID, version)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) ActivateSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s activate --supervisor-service %s", namespaceManagementServices, serviceID)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeactivateSupervisorService(serviceID string) error {
	cmd := fmt.Sprintf("%s deactivate --supervisor-service %s", namespaceManagementServices, serviceID)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) ActivateSupervisorServiceVersion(serviceID, versionID string) error {
	cmd := fmt.Sprintf("%s versions activate --supervisor-service %s --version %s", namespaceManagementServices, serviceID, versionID)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeactivateSupervisorServiceVersion(serviceID, versionID string) error {
	cmd := fmt.Sprintf("%s versions deactivate --supervisor-service %s --version %s", namespaceManagementServices, serviceID, versionID)

	return d.runVoid(cmd)
}

// RegisterCarvelService creates a Carvel service with one version.
func (d *wcpDcliClient) RegisterCarvelService(carvelYaml []byte) error {
	content := b64.StdEncoding.EncodeToString(carvelYaml)
	cmd := fmt.Sprintf("%s create --carvel-spec-version-spec-content %s %s",
		namespaceManagementServices, content, jsonFormatter)

	return d.runVoid(cmd)
}

// RegisterCarvelServiceVersion creates a Carvel service version.
func (d *wcpDcliClient) RegisterCarvelServiceVersion(serviceID string, carvelYaml []byte) error {
	content := b64.StdEncoding.EncodeToString(carvelYaml)
	cmd := fmt.Sprintf("%s versions create --supervisor-service %s --carvel-spec-content %s %s",
		namespaceManagementServices, serviceID, content, jsonFormatter)

	return d.runVoid(cmd)
}

// Precheck initiates a pre-check task for a Supervisor Service version against a Supervisor.
func (d *wcpDcliClient) ServicePrecheck(serviceID, version, supervisor string) error {
	cmd := fmt.Sprintf("%s precheck --supervisor %s --supervisor-service %s --target-version %s %s",
		namespaceMgmtSupervisorsSvServices, supervisor, serviceID, version, jsonFormatter)

	return d.runVoid(cmd)
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

func generateVirtualDevicesCommand(devices VirtualDevices) (string, error) {
	var result strings.Builder

	if devices.VGPUDevices != nil {
		arg, err := jsonArg(devices.VGPUDevices)
		if err != nil {
			return "", err
		}

		result.WriteString(" --devices-vgpu-devices " + arg + " ")
	}

	if devices.DynamicDirectPathIODevices != nil {
		type directPathIO struct {
			VendorID    int    `json:"vendor_id"`
			DeviceID    int    `json:"device_id"`
			CustomLabel string `json:"custom_label"`
		}

		devs := make([]directPathIO, 0, len(devices.DynamicDirectPathIODevices))
		for _, val := range devices.DynamicDirectPathIODevices {
			devs = append(devs, directPathIO{VendorID: val.VendorID, DeviceID: val.DeviceID, CustomLabel: val.CustomLabel})
		}

		arg, err := jsonArg(devs)
		if err != nil {
			return "", err
		}

		result.WriteString(" --devices-dynamic-direct-path-io-devices " + arg + " ")
	}

	return result.String(), nil
}

func generateInstanceStorageCommand(instanceStorage InstanceStorage) (string, error) {
	if len(instanceStorage.Volumes) == 0 || instanceStorage.StoragePolicy == "" {
		return "", nil
	}

	volumes, err := jsonArg(instanceStorage.Volumes)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(" --instance-storage-volumes %s --instance-storage-policy %s",
		volumes, shellQuote(instanceStorage.StoragePolicy)), nil
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
		cmd += " --description " + shellQuote(*vmClassSpec.Description)
	}

	if vmClassSpec.ConfigSpec != nil {
		var w bytes.Buffer

		enc := types.NewJSONEncoder(&w)

		err := enc.Encode(vmClassSpec.ConfigSpec)
		if err != nil {
			return "", err
		}

		cmd += " --config-spec " + shellQuote(strings.TrimSuffix(w.String(), "\n"))
	}

	commandDevices, err := generateVirtualDevicesCommand(vmClassSpec.Devices)
	if err != nil {
		return "", err
	}

	cmd += commandDevices

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

	var obj VMClassInfo
	if err := d.runJSON(cmd, &obj); err != nil {
		return VMClassInfo{}, err
	}

	return obj, nil
}

func (d *wcpDcliClient) ListContentLibraries() ([]string, error) {
	operation := "list"
	s := []string{dcliContentPrefix, operation, jsonFormatter}
	cmd := strings.Join(s, " ")

	var retval []string

	err := d.runJSON(cmd, &retval)

	return retval, err
}

func (d *wcpDcliClient) GetContentLibrary(id string) (ContentLibraryInfo, error) {
	cmd := fmt.Sprintf("%s %s get --library-id %s %s", showUnreleased, dcliContentPrefix, shellQuote(id), jsonFormatter)

	retVal := ContentLibraryInfo{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) FetchContentLibraryIDByName(name string, libraries []string) (string, error) {
	operation := "get"
	for _, libraryID := range libraries {
		s := []string{dcliContentPrefix, operation, "--library-id", libraryID, jsonFormatter}
		cmd := strings.Join(s, " ")
		retVal := ContentLibraryInfo{}

		err := d.runJSON(cmd, &retVal)
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

	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) CreateVMClass(createSpec VMClassSpec) error {
	// CPUCount and MemoryMB are compulsory fields for VMClass creation.
	Expect(createSpec.CPUCount).NotTo(BeNil())
	Expect(createSpec.MemoryMB).NotTo(BeNil())
	operation := fmt.Sprintf("create --id %s", createSpec.ID)

	vmClassSpecCmd, err := GenerateVMClassSpecCmd(createSpec)
	if err != nil {
		return err
	}

	s := []string{namespaceManagementVMClassServices, operation, vmClassSpecCmd, jsonFormatter}
	cmd := strings.Join(s, " ")

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) UpdateVMClass(updateSpec VMClassSpec) error {
	operation := fmt.Sprintf("update --vm-class %s", updateSpec.ID)

	vmClassSpecCmd, err := GenerateVMClassSpecCmd(updateSpec)
	if err != nil {
		return err
	}

	s := []string{namespaceManagementVMClassServices, operation, vmClassSpecCmd, jsonFormatter}
	cmd := strings.Join(s, " ")

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteVMClass(vmClass string) error {
	operation := fmt.Sprintf("delete --vm-class %s", vmClass)
	s := []string{namespaceManagementVMClassServices, operation, jsonFormatter}
	cmd := strings.Join(s, " ")
	return d.runVoid(cmd)
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

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) ListDatastores() ([]Datastore, error) {
	cmd := fmt.Sprintf("%s %s datastore list", jsonFormatter, dcliVCenterPrefix)
	retVal := []Datastore{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) AddCLTrustedCertificate(trustedCertificate string) (string, error) {
	cmd := fmt.Sprintf("%s com vmware content trustedcertificates create --cert-text %s", jsonFormatter, shellQuote(trustedCertificate))

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), err
}

func (d *wcpDcliClient) DeleteCLTrustedCertificate(trustedCertificateID string) error {
	cmd := fmt.Sprintf("%s com vmware content trustedcertificates delete --certificate %s", jsonFormatter, trustedCertificateID)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) ListCLSecurityPolicies() ([]SecurityPolicyInfo, error) {
	cmd := fmt.Sprintf("%s com vmware content securitypolicies list", jsonFormatter)
	retVal := []SecurityPolicyInfo{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) CreateLocalContentLibrary(name string, storageBackings StorageBackingInfo) (string, error) {
	storageBackingsJSON, err := jsonArg(storageBackings.StorageBackings)
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("%s %s create --name %s --storage-backings %s", showUnreleased, dcliContentLocalPrefix, shellQuote(name), storageBackingsJSON)

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) DeleteLocalContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s delete --library-id %s", showUnreleased, dcliContentLocalPrefix, id)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteLocalContentLibraryByForce(id string) error {
	cmd := fmt.Sprintf("%s %s forcedelete --library-id %s", showUnreleased, dcliContentLocalPrefix, id)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) UpdateContentLibrary(contentLibraryID, description string, securityPolicyID string) error {
	cmd := fmt.Sprintf("%s %s update --library-id %s --description %s", showUnreleased, dcliContentPrefix, contentLibraryID, shellQuote(description))
	if securityPolicyID != "" {
		cmd = fmt.Sprintf("%s --security-policy-id %s", cmd, securityPolicyID)
	}

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) UpdateLocalContentLibrary(contentLibraryID string, enablePublishing bool) error {
	cmd := fmt.Sprintf("%s %s update --library-id %s --publish-info-published %t", showUnreleased, dcliContentLocalPrefix, contentLibraryID, enablePublishing)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) CreateSubscribedContentLibrary(name, subscriptionURL, thumbprint string, onDemand bool, storageBackings StorageBackingInfo) (string, error) {
	storageBackingsJSON, err := jsonArg(storageBackings.StorageBackings)
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("%s %s create --name %s --subscription-info-subscription-url %s --subscription-info-on-demand %t --subscription-info-authentication-method %s --subscription-info-automatic-sync-enabled %t --storage-backings %s",
		showUnreleased, dcliContentSubscribedPrefix, shellQuote(name), shellQuote(subscriptionURL), onDemand, "NONE", false, storageBackingsJSON)
	if thumbprint != "" {
		cmd += " --subscription-info-ssl-thumbprint " + shellQuote(thumbprint)
	}

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) SyncSubscribedContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s sync --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteSubscribedContentLibrary(id string) error {
	cmd := fmt.Sprintf("%s %s delete --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteSubscribedContentLibraryByForce(id string) error {
	cmd := fmt.Sprintf("%s %s forcedelete --library-id %s", showUnreleased, dcliContentSubscribedPrefix, id)

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) CreateContentLibraryItem(contentLibraryID, name string, itemType ContentLibraryItemType) (string, error) {
	cmd := fmt.Sprintf("%s %s item create --library-id %s --name %s --type %s", showUnreleased, dcliContentPrefix, contentLibraryID, name, string(itemType))

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

func (d *wcpDcliClient) CreateContentLibraryOVFTemplateItemByPull(contentLibraryID, name, ovfFileURL string) (string, error) {
	cmd := fmt.Sprintf("%s %s item create --library-id %s --name %s --type %s", showUnreleased, dcliContentPrefix, contentLibraryID, name, "ovf")

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	libraryItemID := strings.TrimSpace(string(resp))

	// create update session to add OVF template files by URL
	updateCmd := fmt.Sprintf("%s %s item updatesession create --library-item-id %s", showUnreleased, dcliContentPrefix, libraryItemID)

	resp, err = d.run(updateCmd)
	if err != nil {
		return "", err
	}

	updateSessionID := strings.TrimSpace(string(resp))

	probeCmd := fmt.Sprintf("%s %s item updatesession file probe --uri %s %s", showUnreleased, dcliContentPrefix, ovfFileURL, jsonFormatter)
	probeResult := ProbeResult{}

	if err = d.runJSON(probeCmd, &probeResult); err != nil {
		return "", err
	}

	sslThumbprint := strings.TrimSpace(probeResult.SSLThumbprint)

	fileName := path.Base(ovfFileURL)
	fileCmd := fmt.Sprintf("%s %s item updatesession file add --name %s --source-type %s --update-session-id %s --source-endpoint-uri %s --source-endpoint-ssl-certificate-thumbprint %s",
		showUnreleased, dcliContentPrefix, fileName, "PULL", updateSessionID, ovfFileURL, sslThumbprint)

	if err = d.runVoid(fileCmd); err != nil {
		return "", err
	}

	completeCmd := fmt.Sprintf("%s %s item updatesession complete --update-session-id %s", showUnreleased, dcliContentPrefix, updateSessionID)

	if err = d.runVoid(completeCmd); err != nil {
		return "", err
	}

	return libraryItemID, nil
}

func (d *wcpDcliClient) ListContentLibraryItems(contentLibraryID string) ([]string, error) {
	cmd := fmt.Sprintf("%s %s item list --library-id %s", showUnreleased, dcliContentPrefix, contentLibraryID)
	cmd += " " + jsonFormatter

	var retVal []string

	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) GetContentLibraryItem(id string) (ContentLibraryItemInfo, error) {
	cmd := fmt.Sprintf("%s %s item get --library-item-id %s", showUnreleased, dcliContentPrefix, id)
	cmd += " " + jsonFormatter

	retVal := ContentLibraryItemInfo{}
	err := d.runJSON(cmd, &retVal)

	return retVal, err
}

func (d *wcpDcliClient) UpdateContentLibraryItem(id, description string) error {
	cmd := fmt.Sprintf("%s %s item update --library-item-id %s --description %s", showUnreleased, dcliContentPrefix, id, shellQuote(description))
	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteContentLibraryItem(id string) error {
	cmd := fmt.Sprintf("%s %s item delete --library-item-id %s", showUnreleased, dcliContentPrefix, id)

	return d.runVoid(cmd)
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

	uniqueCLIDs := make(map[string]struct{})
	specs := make([]ContentLibrarySpec, 0, len(contentLibraries))

	for _, cl := range contentLibraries {
		if _, ok := uniqueCLIDs[cl.ContentLibrary]; !ok {
			uniqueCLIDs[cl.ContentLibrary] = struct{}{}
			specs = append(specs, cl)
		}
	}

	return d.updateNamespaceContentLibraries(namespace, specs)
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
		if !slices.Contains(contentLibraryIDs, cl.ContentLibrary) {
			clsToKeep = append(clsToKeep, cl)
		}
	}

	return d.updateNamespaceContentLibraries(namespace, clsToKeep)
}

// updateNamespaceContentLibraries sets the namespace's content libraries.
func (d *wcpDcliClient) updateNamespaceContentLibraries(namespace string, contentLibraries []ContentLibrarySpec) error {
	libraries, err := jsonArg(contentLibraries)
	if err != nil {
		return err
	}

	return d.runVoid(fmt.Sprintf("%s %s update --namespace %s --content-libraries %s",
		showUnreleased, namespacesInstances, namespace, libraries))
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

	// Avoid duplicate content library error in running the dcli command.
	uniqueCLIDs := make(map[string]struct{})
	specs := make([]ClusterContentLibrarySpec, 0, len(contentLibraries))

	for _, cl := range contentLibraries {
		if _, ok := uniqueCLIDs[cl.ContentLibrary]; !ok {
			uniqueCLIDs[cl.ContentLibrary] = struct{}{}
			specs = append(specs, cl)
		}
	}

	return d.updateClusterContentLibraries(cluster, specs)
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
	clsToKeep := []ClusterContentLibrarySpec{}

	for _, clSpec := range clusterDetails.ContentLibraries {
		if slices.Contains(contentLibraryIDs, clSpec.ContentLibrary) {
			continue
		}

		// Make sure that the ContentLibrary *exists* to update with.
		if _, err := d.GetContentLibrary(clSpec.ContentLibrary); err == nil {
			clsToKeep = append(clsToKeep, clSpec)
		}
	}

	// Run an update, omitting the passed in contentLibraryIDs, but keeping other added content libraries
	// (to preserve state away from tests).
	return d.updateClusterContentLibraries(cluster, clsToKeep)
}

// updateClusterContentLibraries sets the cluster's content libraries.
func (d *wcpDcliClient) updateClusterContentLibraries(cluster string, contentLibraries []ClusterContentLibrarySpec) error {
	// The API requires an empty array rather than null for no supervisor services.
	specs := make([]ClusterContentLibrarySpec, len(contentLibraries))
	for i, cl := range contentLibraries {
		if cl.SupervisorServices == nil {
			cl.SupervisorServices = []string{}
		}

		specs[i] = cl
	}

	libraries, err := jsonArg(specs)
	if err != nil {
		return err
	}

	return d.runVoid(fmt.Sprintf("%s %s clusters update --cluster %s --content-libraries %s",
		showUnreleased, namespaceManagement, cluster, libraries))
}

func (d *wcpDcliClient) RegisterVM(namespace, vmMoID string) (string, error) {
	cmd := fmt.Sprintf("%s %s registervm --namespace %s --vm %s", showUnreleased, namespacesInstances, namespace, vmMoID)

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
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
				_, _ = fmt.Fprintf(&cmd, " --%s %s", flag, shellQuote(value))
			}
		}
	case None:
		// For None, we don't need to pass any other flags
	default:
		return fmt.Errorf("invalid proxy source: %s", proxyConfig.ProxySettingsSource)
	}

	return d.runVoid(cmd.String())
}

func (d *wcpDcliClient) GetClusterProxyConfig(clusterMoid string) (ClusterProxyConfig, error) {
	details, err := d.GetCluster(clusterMoid)
	if err != nil {
		return ClusterProxyConfig{}, err
	}

	return details.ClusterProxyConfig, nil
}

func (d *wcpDcliClient) CreateContainerImageRegistry(supervisorID string, registry ContainerImageRegistry) (ContainerImageRegistryInfo, error) {
	cmd := fmt.Sprintf("%s containerimageregistries create --name %s --supervisor %s ", namespaceManagementSupervisors, shellQuote(registry.Name), shellQuote(supervisorID))

	if registry.ImageRegistry.CertificateChain != "" {
		cmd += "--image-registry-certificate-chain " + shellQuote(registry.ImageRegistry.CertificateChain) + " "
	}

	if registry.ImageRegistry.Password != "" {
		cmd += "--image-registry-password " + shellQuote(registry.ImageRegistry.Password) + " "
	}

	if registry.ImageRegistry.Username != "" {
		cmd += "--image-registry-username " + shellQuote(registry.ImageRegistry.Username) + " "
	}

	if registry.ImageRegistry.Port != 0 {
		cmd += fmt.Sprintf("--image-registry-port %v ", registry.ImageRegistry.Port)
	}

	cmd += "--image-registry-hostname " + shellQuote(registry.ImageRegistry.Hostname) + " "
	cmd += fmt.Sprintf("--default-registry %t ", registry.DefaultRegistry)

	cmd += jsonFormatter

	result := ContainerImageRegistryInfo{}

	err := d.runJSON(cmd, &result)

	return result, err
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
	cmd := fmt.Sprintf("%s containerimageregistries list --supervisor %s %s", namespaceManagementSupervisors, shellQuote(supervisorID), jsonFormatter)

	result := []ContainerImageRegistryInfo{}

	err := d.runJSON(cmd, &result)

	return result, err
}

func (d *wcpDcliClient) DeleteContainerImageRegistry(supervisorID, registryID string) error {
	cmd := fmt.Sprintf("%s containerimageregistries delete --container-image-registry %s --supervisor %s ", namespaceManagementSupervisors, shellQuote(registryID), shellQuote(supervisorID))
	return d.runVoid(cmd)
}

func (d *wcpDcliClient) CreateKeyProvider(provider string) error {
	cmd := fmt.Sprintf("%s kms providers create --provider %s", cryptoManager, provider)
	return d.runVoid(cmd)
}

func (d *wcpDcliClient) DeleteKeyProvider(provider string) error {
	cmd := fmt.Sprintf("%s kms providers delete --provider %s", cryptoManager, provider)
	return d.runVoid(cmd)
}

// BackupKeyProvider exports (backs up) a native key provider. vCenter requires
// a backup before the provider can be used for encryption.
func (d *wcpDcliClient) BackupKeyProvider(provider string) error {
	cmd := fmt.Sprintf("%s kms providers export --provider %s", cryptoManager, provider)
	return d.runVoid(cmd)
}

// AssignLicenseEntitlement assigns the given license entitlement to the cluster using the dcli cis command.
func (d *wcpDcliClient) AssignLicenseEntitlement(signedEntitlement string) (string, error) {
	cmd := fmt.Sprintf("%s %s entitlements update-task --other-vc-usages '[]' --configuration %s", showUnreleased, dcliCISLicenseEntitlementPrefix, shellQuote(signedEntitlement))

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
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

	if err := d.runJSON(cmd, &result); err != nil {
		return "", fmt.Errorf("failed to run DCLI command or parse result: %w", err)
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

	return d.runVoid(cmd)
}

func (d *wcpDcliClient) GetWorkerDNS(clusterMoid string) ([]string, error) {
	cmd := fmt.Sprintf("%s clusters get --cluster %s %s", namespaceManagement, clusterMoid, jsonFormatter)

	var result struct {
		WorkerDNS []string `json:"worker_DNS"`
	}

	if err := d.runJSON(cmd, &result); err != nil {
		return nil, fmt.Errorf("failed to run DCLI command or parse result: %w", err)
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
			"--vsphere-network-gateway %s "+
			"--vsphere-network-subnet-mask %s %s",
		namespaceManagementNetworks,
		config.Cluster,
		config.Network,
		config.NetworkProvider,
		config.VsphereNetworkMode,
		config.VsphereNetworkIPAssignmentMode,
		config.VsphereNetworkPortgroup,
		shellQuote(config.VsphereNetworkGateway),
		shellQuote(config.VsphereNetworkSubnetMask),
		jsonFormatter,
	)

	// If the network already exists, treat it as success (idempotent operation).
	return d.runIgnoringAlreadyExists(cmd)
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

	err := d.runJSON(cmd, &result)

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

	return d.runVoid(cmd)
}

// CreateTagCategory creates a new tag category with the given name and description.
func (d *wcpDcliClient) CreateTagCategory(name, description string) (string, error) {
	cmd := fmt.Sprintf("com vmware cis tagging category create --cardinality MULTIPLE --name %s --description %s", name, shellQuote(description))

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

// CreateTag creates a new tag with the given name, description and category ID.
func (d *wcpDcliClient) CreateTag(name, description, categoryID string) (string, error) {
	cmd := fmt.Sprintf("com vmware cis tagging tag create --name %s --description %s --category-id %s", name, shellQuote(description), categoryID)

	resp, err := d.run(cmd)
	if err != nil {
		return "", err
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

	return d.runVoid(cmd.String())
}

// CreateComputePolicy creates a compute policy with the given spec and returns the created compute policy ID.
// HostTagID is optional -- some capabilities (e.g. DisableDrsVmotion) only
// take a VM tag.
func (d *wcpDcliClient) CreateComputePolicy(spec ComputePolicySpec) (string, error) {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s compute policies create --capability %s --name %s --description %s --vm-tag %s",
		dcliVCenterPrefix,
		showUnreleased,
		spec.Capability,
		spec.Name,
		shellQuote(spec.Description),
		spec.VMTagID,
	)

	if spec.HostTagID != "" {
		fmt.Fprintf(&cmd, " --host-tag %s", spec.HostTagID)
	}

	resp, err := d.run(cmd.String())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

// CreateComputePolicyWithSpec creates a compute policy for capabilities whose "compute
// policies create" verb is not supported server-side (e.g. automatic_vm_eviction), by going
// through "compute policies createtagsandpolicies" instead. Unlike create, that verb takes
// capability/name/description nested inside the JSON spec itself rather than as flags.
func (d *wcpDcliClient) CreateComputePolicyWithSpec(name, description string, capability ComputePolicyCapability, specJSON map[string]any) (string, error) {
	fullSpec := map[string]any{
		"capability":  capability,
		"name":        name,
		"description": description,
	}

	for k, v := range specJSON {
		fullSpec[k] = v
	}

	policies, err := jsonArg([]map[string]any{{"spec": fullSpec}})
	if err != nil {
		return "", fmt.Errorf("failed to marshal compute policy spec: %w", err)
	}

	cmd := fmt.Sprintf("%s %s createtagsandpolicies --policies %s %s",
		computePolicies,
		showUnreleased,
		policies,
		jsonFormatter,
	)

	var result struct {
		Policies map[string]struct {
			Policy string `json:"policy"`
		} `json:"policies"`
	}
	if err := d.runJSON(cmd, &result); err != nil {
		return "", fmt.Errorf("createtagsandpolicies failed: %w", err)
	}

	entry, ok := result.Policies[name]
	if !ok {
		return "", fmt.Errorf("createtagsandpolicies response did not contain policy %q", name)
	}

	return entry.Policy, nil
}

// DeleteComputePolicy deletes a compute policy by its ID.
func (d *wcpDcliClient) DeleteComputePolicy(policyID string) error {
	cmd := fmt.Sprintf("%s %s delete --policy %s", computePolicies, showUnreleased, policyID)

	return d.runVoid(cmd)
}

// CreateInfraPolicy creates an infrastructure policy with the given spec.
func (d *wcpDcliClient) CreateInfraPolicy(spec InfraPolicySpec) error {
	var cmd strings.Builder
	fmt.Fprintf(&cmd, "%s %s infrastructurepolicies create --policy %s --description %s --compute-policy-id %s ",
		namespaceManagement,
		showUnreleased,
		spec.Name,
		shellQuote(spec.Description),
		spec.ComputePolicyID)

	if spec.EnforcementMode != "" {
		fmt.Fprintf(&cmd, " --enforcement-mode %s", spec.EnforcementMode)
	}

	if spec.MatchGuestIDValue != "" {
		fmt.Fprintf(&cmd, " --match-workload-guest-guest-id-value %s", spec.MatchGuestIDValue)
	}

	if len(spec.MatchWorkloadLabel) > 0 {
		type labelSelector struct {
			Key      string   `json:"key"`
			Operator string   `json:"operator"`
			Values   []string `json:"values,omitempty"`
		}

		labels := make([]labelSelector, 0, len(spec.MatchWorkloadLabel))

		for key, value := range spec.MatchWorkloadLabel {
			if value == "" {
				labels = append(labels, labelSelector{Key: key, Operator: "EXISTS"})
			} else {
				labels = append(labels, labelSelector{Key: key, Operator: "IS_IN", Values: []string{value}})
			}
		}

		labelsArg, err := jsonArg(labels)
		if err != nil {
			return err
		}

		fmt.Fprintf(&cmd, " --match-workload-labels %s", labelsArg)
	}

	return d.runVoid(cmd.String())
}

// DeleteInfraPolicy deletes an infrastructure policy by name.
func (d *wcpDcliClient) DeleteInfraPolicy(name string) error {
	cmd := fmt.Sprintf("%s %s infrastructurepolicies delete --policy %s", namespaceManagement, showUnreleased, name)

	return d.runVoid(cmd)
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

	return d.runVoid(cmd.String())
}

// ListHostIDs lists all host IDs in the vCenter.
func (d *wcpDcliClient) ListHostIDs() ([]string, error) {
	cmd := fmt.Sprintf("%s %s host list %s", dcliVCenterPrefix, showUnreleased, jsonFormatter)

	var hosts []struct {
		Host string `json:"host"`
	}
	if err := d.runJSON(cmd, &hosts); err != nil {
		return nil, err
	}

	hostIDs := make([]string, len(hosts))
	for i, h := range hosts {
		hostIDs[i] = h.Host
	}

	return hostIDs, nil
}

// ListComputePolicyTagUsage lists compute policy tag usage entries filtered by category name and tag name.
func (d *wcpDcliClient) ListComputePolicyTagUsage(categoryName, tagName string) ([]TagUsageEntry, error) {
	cmd := fmt.Sprintf("%s %s", computePolicyTagUsages, jsonFormatter)

	var all []TagUsageEntry
	if err := d.runJSON(cmd, &all); err != nil {
		return nil, err
	}

	var filtered []TagUsageEntry
	for _, e := range all {
		if e.CategoryName == categoryName && e.TagName == tagName {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}

// GetComputePolicy retrieves the backend definition of a compute policy.
func (d *wcpDcliClient) GetComputePolicy(policyID string) (ComputePolicyInfo, error) {
	cmd := fmt.Sprintf("%s get --policy %s %s %s", computePolicies, policyID, showUnreleased, jsonFormatter)

	var info ComputePolicyInfo
	if err := d.runJSON(cmd, &info); err != nil {
		return ComputePolicyInfo{}, fmt.Errorf("get compute policy %q: %w", policyID, err)
	}

	return info, nil
}

// GetVMPolicyCompliance returns the compliance status of a VM against a compute policy.
func (d *wcpDcliClient) GetVMPolicyCompliance(policyID, vmMoid string) (VMPolicyComplianceStatus, error) {
	cmd := fmt.Sprintf("%s --policy %s --vm %s %s", virtualmachineComputePolicy, policyID, vmMoid, jsonFormatter)

	var status VMPolicyComplianceStatus
	if err := d.runJSON(cmd, &status); err != nil {
		return VMPolicyComplianceStatus{}, err
	}

	return status, nil
}
