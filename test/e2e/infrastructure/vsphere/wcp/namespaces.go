// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"context"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
)

type ContentLibrarySpec struct {
	ContentLibrary         string `json:"content_library"`
	Writable               bool   `json:"writable"`
	AllowImport            bool   `json:"allow_import"`
	ResourceNamingStrategy string `json:"resource_naming_strategy"`
}

type VMServiceSpecDetails struct {
	VMClasses        []string `json:"vm_classes"`
	ContentLibraries []string `json:"content_libraries"`
}

type VPCNetworkInfo struct {
	VPCPath             string `json:"vpc_path"`
	VPCSharedSubnetPath string `json:"vpc_shared_subnet_path"`
	DefaultSubnetSize   int64  `json:"default_subnet_size,omitempty"`
	SupervisorID        string `json:"supervisor_id"`
}

type NameSpaceNetworkInfo struct {
	VDSNetwork string          `json:"vds_network,omitempty"`
	VPCNetwork *VPCNetworkInfo `json:"vpc_network,omitempty"`
}

func NewVMServiceSpecDetails(vmClasses []string, contentLibraries []string) VMServiceSpecDetails {
	return VMServiceSpecDetails{
		VMClasses:        vmClasses,
		ContentLibraries: contentLibraries,
	}
}

type NamespaceDetails struct {
	ClusterMoID      string               `json:"cluster"`
	Name             string               `json:"namespace"`
	ConfigStatus     string               `json:"config_status"`
	VMServiceSpec    VMServiceSpecDetails `json:"vm_service_spec"`
	VMStorageSpec    []StorageSpec        `json:"storage_specs"`
	ContentLibraries []ContentLibrarySpec `json:"content_libraries"`
}

type NamespaceGetInput struct {
	NamespaceName string
	Kubeconfig    string
	WCPClient     WorkloadManagementAPI
	Client        ctrlclient.Client
}

type NamespaceCreateInput struct {
	SpecName               string
	Kubeconfig             string
	ArtifactFolder         string
	Client                 ctrlclient.Client
	WCPClient              WorkloadManagementAPI
	StorageClassName       string
	VMServiceSpec          VMServiceSpecDetails
	Zone                   string
	SupervisorID           string
	ReservedVMClassToCount map[string]int
	Network                *NameSpaceNetworkInfo
}

type NamespaceDeleteInput struct {
	WCPClient     WorkloadManagementAPI
	Namespace     *corev1.Namespace
	CancelWatches context.CancelFunc
}

// NamespaceNetworkCreateInput contains input parameters for creating a vSphere network for WCP.
type NamespaceNetworkCreateInput struct {
	Cluster      string
	NetworkName  string
	PortGroupKey string // e.g., dvportgroup-90
	Gateway      string // can be empty
	SubnetMask   string
	SupervisorID string // i.e., cluster MoID (same as Cluster)
	WCPClient    WorkloadManagementAPI
}

func WaitForNamespaceReady(wcpClient WorkloadManagementAPI, namespaceForTest string) {
	Eventually(func() bool {
		details, err := wcpClient.GetNamespace(namespaceForTest)
		if err != nil {
			return false
		}
		// TODO Make this an enum.
		return details.ConfigStatus == "RUNNING"
	}, 300*time.Second, 5*time.Second).Should(BeTrue(), "Namespace", namespaceForTest, " did not become READY in time")
}

func GetNamespace(ctx context.Context, input NamespaceGetInput) (*corev1.Namespace, context.CancelFunc) {
	var (
		wcpnamespace NamespaceDetails
		ns           *corev1.Namespace
		err          error
	)

	wcpClient := input.WCPClient
	wcpnamespace, err = wcpClient.GetNamespace(input.NamespaceName)

	Expect(err).NotTo(HaveOccurred(), "get wcp namespace failed from wcp api %q", input.NamespaceName)

	WaitForNamespaceReady(wcpClient, wcpnamespace.Name)

	_, cancelWatches := context.WithCancel(ctx)

	ns, err = GetCorev1Namespace(ctx, input.Client, wcpnamespace.Name)

	Expect(err).NotTo(HaveOccurred(), "get wcp namespace failed from wcp cluster %q", wcpnamespace.Name)

	return ns, cancelWatches
}

// defaultNamespaceStorageQuotaMiB is the default per-policy quota (MiB) used when associating storage with a WCP namespace.
const defaultNamespaceStorageQuotaMiB = int64(1024 * 500)

// CreateNamespace creates a WCP kubernetes namespace object using DCLI client.
func CreateNamespace(ctx context.Context, input NamespaceCreateInput) (*corev1.Namespace, context.CancelFunc) {
	wcpClient := input.WCPClient
	svKubeConfig := input.Kubeconfig
	// Create a new cluster, either because we were asked to, or because no running cluster was found.
	vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, svKubeConfig)
	Expect(vCenterHostname).NotTo(BeEmpty(), "Unable to determine VC PNID")

	clusterID := vcenter.GetClusterMoIDFromKubeconfig(ctx, svKubeConfig)
	Expect(clusterID).NotTo(BeEmpty(), "Unable to determine cluster MoID")

	// Check the supervisor storage class, get the storage policy from it.
	// Avoid name based lookups as the VC storage policy name can be transformed when it's being synced to the supervisor cluster.
	storageClass := storagev1.StorageClass{}
	Expect(input.Client.Get(ctx, ctrlclient.ObjectKey{Name: input.StorageClassName}, &storageClass)).To(Succeed())
	Expect(storageClass.Parameters).NotTo(BeNil())
	policyID, ok := storageClass.Parameters["storagePolicyID"]
	Expect(ok).To(BeTrue(), "supervisor storage class must have a corresponding storage policy ID")

	storageSpec := []StorageSpec{{Policy: policyID, Limit: defaultNamespaceStorageQuotaMiB}}

	namespaceForTest := input.SpecName

	var err error
	switch {
	case input.Network != nil:
		err = wcpClient.CreateNamespaceWithNetwork(clusterID, namespaceForTest, storageSpec, input.VMServiceSpec, input.Network)
	case len(input.ReservedVMClassToCount) > 0:
		err = wcpClient.CreateNamespaceWithVMReservation(namespaceForTest, input.Zone, input.SupervisorID, storageSpec, input.VMServiceSpec, input.ReservedVMClassToCount)
	default:
		err = wcpClient.CreateNamespaceWithSpecs(clusterID, namespaceForTest, storageSpec, input.VMServiceSpec)
	}
	Expect(err).NotTo(HaveOccurred())
	WaitForNamespaceReady(wcpClient, namespaceForTest)

	_, cancelWatches := context.WithCancel(ctx)
	ns, err := GetCorev1Namespace(ctx, input.Client, namespaceForTest)
	Expect(err).NotTo(HaveOccurred())

	return ns, cancelWatches
}

func DeleteNamespace(input NamespaceDeleteInput) {
	if input.CancelWatches != nil {
		input.CancelWatches()
	}

	if input.Namespace == nil || input.Namespace.Name == "" {
		return
	}

	Expect(input.WCPClient).NotTo(BeNil())
	Expect(input.WCPClient.DeleteNamespace(input.Namespace.Name)).NotTo(HaveOccurred())
}

// TODO: Timeout time can be reduced after this issue is fixed.
// https://bugzilla.eng.vmware.com/show_bug.cgi?id=3432165
func WaitForNamespaceDeleted(wcpClient WorkloadManagementAPI, namespaceForTest string) {
	Eventually(func() bool {
		_, err := wcpClient.GetNamespace(namespaceForTest)

		var dcliErr DcliError
		if err != nil && errors.As(err, &dcliErr) {
			return strings.Contains(dcliErr.Response(), "com.vmware.vapi.std.errors.NotFound")
		}

		return false
	}, 6*time.Minute, 30*time.Second).Should(BeTrue(), "Namespace %s did not get DELETED in time", namespaceForTest)
}

func GetCorev1Namespace(ctx context.Context, client ctrlclient.Client, name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}

	key := types.NamespacedName{
		Name: name,
	}

	err := client.Get(ctx, key, ns)
	if err != nil {
		return nil, err
	}

	return ns, nil
}
