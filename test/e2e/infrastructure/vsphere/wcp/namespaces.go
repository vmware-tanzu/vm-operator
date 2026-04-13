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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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
	ClientSet              *kubernetes.Clientset
	Client                 ctrlclient.Client
	WCPClient              WorkloadManagementAPI
	StorageClassName       string
	WorkerStorageClassName string
	// Config supplies loaded e2e intervals (wcp.yaml); nil uses built-in defaults for worker StorageClass wait only.
	Config                 framework.ConfigInterface
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

// EnsureWorkerKubernetesStorageClassIfMissing duplicates the primary WCP storage policy on vSphere when the
// worker-named StorageClass is missing from the supervisor, then waits for WCPSVC to sync the StorageClass.
// Wait/poll intervals use built-in defaults (NamespaceCreateInput.Config drives config when creating a namespace).
//
// When namespace and wcpClient are set, the worker storage policy is also associated with that supervisor namespace
// (via SetNamespaceStorageSpecs) if it is not already present. Use this for pre-provisioned namespaces that were
// created before the worker StorageClass existed.
func EnsureWorkerKubernetesStorageClassIfMissing(
	ctx context.Context,
	kubeconfigPath string,
	client kubernetes.Interface,
	primaryStorageClass *storagev1.StorageClass,
	workerStorageClassName string,
	cfg framework.ConfigInterface,
	namespace string,
	wcpClient WorkloadManagementAPI) *storagev1.StorageClass {
	if workerStorageClassName == "" {
		return nil
	}

	sc, err := client.StorageV1().StorageClasses().Get(ctx, workerStorageClassName, metav1.GetOptions{})
	if err == nil {
		Expect(sc.Parameters).NotTo(BeNil())
		policyID, ok := sc.Parameters["storagePolicyID"]
		Expect(ok).To(BeTrue(), "worker storage class must expose storagePolicyID for namespace association")
		ensureNamespaceAssociatesWorkerStoragePolicyByID(wcpClient, namespace, policyID)

		return sc
	}

	if !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	Expect(primaryStorageClass.Parameters).NotTo(BeNil())
	basePolicyID, ok := primaryStorageClass.Parameters["storagePolicyID"]
	Expect(ok).To(BeTrue(), "primary storage class must expose storagePolicyID for cloning worker policy")
	Expect(basePolicyID).NotTo(BeEmpty())

	vimClient := vcenter.NewVimClientFromKubeconfig(ctx, kubeconfigPath)
	defer vcenter.LogoutVimClient(vimClient)

	workerPolicyID, createErr := vcenter.GetOrCreateWorkerStoragePolicy(ctx, vimClient, workerStorageClassName, basePolicyID)
	Expect(createErr).NotTo(HaveOccurred(), "failed to create or resolve worker storage policy %q", workerStorageClassName)
	ensureNamespaceAssociatesWorkerStoragePolicyByID(wcpClient, namespace, workerPolicyID)

	sc = waitForWorkerStorageClass(ctx, client, workerStorageClassName, cfg)

	return sc
}

// ensureNamespaceAssociatesWorkerStoragePolicyByID appends the worker policy to the namespace storage
// specs when missing. The policy ID must be passed directly so this can be called before the
// Kubernetes StorageClass object exists (WCPSVC only syncs the StorageClass after the association).
func ensureNamespaceAssociatesWorkerStoragePolicyByID(
	wcpClient WorkloadManagementAPI,
	namespace string,
	workerPolicyID string) {
	if namespace == "" || wcpClient == nil || workerPolicyID == "" {
		return
	}

	details, err := wcpClient.GetNamespace(namespace)
	Expect(err).NotTo(HaveOccurred(), "get wcp namespace failed for storage policy sync %q", namespace)

	for _, s := range details.VMStorageSpec {
		if s.Policy == workerPolicyID {
			return
		}
	}

	limit := defaultNamespaceStorageQuotaMiB
	if len(details.VMStorageSpec) > 0 && details.VMStorageSpec[0].Limit > 0 {
		limit = details.VMStorageSpec[0].Limit
	}

	merged := append(append([]StorageSpec(nil), details.VMStorageSpec...),
		StorageSpec{Policy: workerPolicyID, Limit: limit})
	Expect(wcpClient.SetNamespaceStorageSpecs(namespace, merged)).NotTo(HaveOccurred(),
		"failed to associate worker storage policy with namespace %q", namespace)
	WaitForNamespaceReady(wcpClient, namespace)
}

// waitForWorkerStorageClass polls until the StorageClass appears after vSphere policy creation (WCPSVC sync).
func waitForWorkerStorageClass(ctx context.Context, client kubernetes.Interface, name string, cfg framework.ConfigInterface) *storagev1.StorageClass {
	var sc *storagev1.StorageClass

	Eventually(func() error {
		var err error

		sc, err = client.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})

		return err
	}, cfg.GetIntervals("default", "wait-storage-class-ready")...).Should(Succeed(),
		"timed out waiting for StorageClass %q after ensuring worker storage policy on vCenter", name)
	Expect(sc).NotTo(BeNil())

	return sc
}

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
	svClientSet := input.ClientSet
	storageClass, err := svClientSet.StorageV1().StorageClasses().Get(ctx, input.StorageClassName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(storageClass).NotTo(BeNil())
	Expect(storageClass.Parameters).NotTo(BeNil())
	policyID, ok := storageClass.Parameters["storagePolicyID"]
	Expect(ok).To(BeTrue(), "supervisor storage class must have a corresponding storage policy ID")

	storageSpec := []StorageSpec{{Policy: policyID, Limit: defaultNamespaceStorageQuotaMiB}}

	namespaceForTest := input.SpecName

	network := input.Network
	switch {
	case network != nil:
		err = wcpClient.CreateNamespaceWithNetwork(clusterID, namespaceForTest, storageSpec, input.VMServiceSpec, network)
	case len(input.ReservedVMClassToCount) > 0:
		err = wcpClient.CreateNamespaceWithVMReservation(namespaceForTest, input.Zone, input.SupervisorID, storageSpec, input.VMServiceSpec, input.ReservedVMClassToCount)
	default:
		err = wcpClient.CreateNamespaceWithSpecs(clusterID, namespaceForTest, storageSpec, input.VMServiceSpec)
	}

	Expect(err).NotTo(HaveOccurred())
	WaitForNamespaceReady(wcpClient, namespaceForTest)

	if input.WorkerStorageClassName != "" {
		workerStorageClass := EnsureWorkerKubernetesStorageClassIfMissing(ctx, input.Kubeconfig, svClientSet,
			storageClass, input.WorkerStorageClassName, input.Config, namespaceForTest, wcpClient)

		Expect(workerStorageClass).NotTo(BeNil())
		Expect(workerStorageClass.Parameters).NotTo(BeNil())
		_, ok = workerStorageClass.Parameters["storagePolicyID"]
		Expect(ok).To(BeTrue(), "supervisor storage class must have a corresponding storage policy ID")
	}

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
