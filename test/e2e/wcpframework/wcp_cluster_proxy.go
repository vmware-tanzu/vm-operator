package wcpframework

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
)

// WCP Cluster proxy interface is designed to better fit the architecture of WCP cluster
// WCO cluster contains Kubernetes behaviors and wcp api behaviors
// Cluster proxy interface encapsulates canonical kubernetes behaviors
// However, the interface here only contains key behaviors for E2E testing
// in which case the namespace creation and deletion are the most common use cases.
type WCPClusterProxyInterface interface {
	framework.ClusterProxyInterface
	CreateWCPNamespace(ctx context.Context, config framework.ConfigInterface,
		vmsvcSpecs wcp.VMServiceSpecDetails,
		scName, nsName, artifactFolder string) (NamespaceContext, error)
	UpdateNamespaceWithZones(ctx context.Context, namespaceName string, zones []string, svClusterClient ctrlclient.Client) (ZoneContext, error)
	DeleteZonesFromNamespace(ctx context.Context, namespaceName string, zones []string, svClusterClient ctrlclient.Client) error
	GetZonesBoundWithSupervisor(supervisorID string) (wcp.ZoneList, error)
	CreateZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	CreateMissingZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	DeleteZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	ListVSphereZones() (wcp.VSphereZoneList, error)
	CreateWCPNamespaceWithNetwork(ctx context.Context, config framework.ConfigInterface,
		vmsvcSpecs wcp.VMServiceSpecDetails,
		scName, nsName, artifactFolder string, network wcp.NameSpaceNetworkInfo) (NamespaceContext, error)
	CreateWCPNamespaceWithVMReservation(ctx context.Context, nsName, scName, zone, supervisorID string, vmsvcSpecs wcp.VMServiceSpecDetails, vmClassNameToReservedCount map[string]int) (NamespaceContext, error)
	DeleteWCPNamespace(nsCtx NamespaceContext)
}

// WCP cluster proxy is designed particular to operators that need to talk with wcp api.
type WCPClusterProxy struct {
	framework.ClusterProxyInterface

	wcpAPI wcp.WorkloadManagementAPI
}

// Instantiate a wcp cluster proxy object.
func NewWCPClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *WCPClusterProxy {
	kubeClusterProxy := framework.NewClusterProxy(name, kubeconfigPath, scheme)
	wcpAPI := wcp.NewClientUsingKubeconfig(context.TODO(), kubeconfigPath)

	return &WCPClusterProxy{
		ClusterProxyInterface: kubeClusterProxy,
		wcpAPI:                wcpAPI,
	}
}

// Instantiate a wcp cluster proxy object with provided credentials.
func NewWCPClusterProxyWithCredentials(name string, kubeconfigPath string, user string, password string, scheme *runtime.Scheme) *WCPClusterProxy {
	kubeClusterProxy := framework.NewClusterProxy(name, kubeconfigPath, scheme)
	wcpAPI := wcp.NewClientUsingKubeconfigWithCredentials(context.TODO(), kubeconfigPath, user, password)

	return &WCPClusterProxy{
		ClusterProxyInterface: kubeClusterProxy,
		wcpAPI:                wcpAPI,
	}
}

// GetWorkloadManagementAPI returns the WCP API associated with the supervisor cluster.
func (w *WCPClusterProxy) GetWorkloadManagementAPI() wcp.WorkloadManagementAPI {
	return w.wcpAPI
}

// CreateWCPNamespace applies wcp api to create a namespace in wcp cluster with the given specs.
func (w *WCPClusterProxy) CreateWCPNamespace(ctx context.Context, config framework.ConfigInterface,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	scName, nsName, artifactFolder string) (NamespaceContext, error) {
	// we need to create namespace with vm class and bind it
	// the CR of vm class will get deleted if the namespace owning it gets deleted
	// we can assume that vcenter has default vm classes names present
	namespace, cancelNsWatches := wcp.CreateNamespace(ctx, wcp.NamespaceCreateInput{
		SpecName:         nsName,
		Client:           w.ClusterProxyInterface.GetClient(),
		Kubeconfig:       w.ClusterProxyInterface.GetKubeconfigPath(),
		StorageClassName: scName,
		WCPClient:        w.wcpAPI,
		ArtifactFolder:   artifactFolder,
		VMServiceSpec:    vmsvcSpecs,
	})

	return NamespaceContext{
		namespace:       namespace,
		cancelNsWatches: cancelNsWatches,
	}, nil
}

// CreatePrivateRegistry creates a container image registry for the supervisor cluster.
func (w *WCPClusterProxy) CreatePrivateRegistry(ctx context.Context, registryName, hostname string, port int, username, password, certificateChain string, defaultRegistry bool) wcp.ContainerImageRegistryInfo {
	return wcp.CreatePrivateRegistry(ctx, wcp.PrivateRegistryInput{
		Kubeconfig:       w.ClusterProxyInterface.GetKubeconfigPath(),
		WCPClient:        w.wcpAPI,
		RegistryName:     registryName,
		Hostname:         hostname,
		Port:             port,
		Username:         username,
		Password:         password,
		CertificateChain: certificateChain,
		DefaultRegistry:  defaultRegistry,
	})
}

// DeletePrivateRegistry deletes a container image registry for the supervisor cluster.
func (w *WCPClusterProxy) DeletePrivateRegistry(ctx context.Context, registryName string) error {
	return wcp.DeletePrivateRegistry(ctx, wcp.PrivateRegistryInput{
		Kubeconfig:   w.ClusterProxyInterface.GetKubeconfigPath(),
		WCPClient:    w.wcpAPI,
		RegistryName: registryName,
	})
}

func (w *WCPClusterProxy) GetZonesBoundWithSupervisor(supervisorID string) (wcp.ZoneList, error) {
	zoneList, _ := wcp.GetZonesBoundWithSupervisor(wcp.ZonesGetInput{
		SupervisorID: supervisorID,
		WCPClient:    w.wcpAPI,
	})

	return zoneList, nil
}

func (w *WCPClusterProxy) CreateMissingZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	boundZones, err := w.GetZonesBoundWithSupervisor(supervisorID)
	if err != nil {
		return err
	}

	if len(boundZones.Zones) > 0 {
		var missingZones []string

		for _, z := range zones {
			missing := true

			for _, bz := range boundZones.Zones {
				if z == bz.Zone {
					missing = false
					break
				}
			}

			if missing {
				missingZones = append(missingZones, z)
			}
		}

		zones = missingZones
	}

	if len(zones) > 0 {
		return w.CreateZoneBindingsWithSupervisor(supervisorID, zones)
	}

	return nil
}

func (w *WCPClusterProxy) CreateZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	return wcp.CreateZoneBindingsWithSupervisor(wcp.ZonesBindingInput{
		SupervisorID: supervisorID,
		Zones:        zones,
		WCPClient:    w.wcpAPI,
	})
}

func (w *WCPClusterProxy) ListVSphereZones() (wcp.VSphereZoneList, error) {
	return wcp.ListVSphereZones(w.wcpAPI)
}

func (w *WCPClusterProxy) DeleteZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	return wcp.DeleteZoneBindingsWithSupervisor(wcp.ZonesBindingInput{
		SupervisorID: supervisorID,
		Zones:        zones,
		WCPClient:    w.wcpAPI,
	})
}

// UpdateNamespaceWithZones updates namespace with specified zones.
func (w *WCPClusterProxy) UpdateNamespaceWithZones(ctx context.Context, nsName string, zones []string, svClusterClient ctrlclient.Client) (ZoneContext, error) {
	zoneList, cancelZoneWatches := wcp.UpdateNamespaceWithZones(ctx, wcp.BindZonesForNamespaceInput{
		Namespace:       nsName,
		Zones:           zones,
		SvClusterClient: svClusterClient,
		Kubeconfig:      w.ClusterProxyInterface.GetKubeconfigPath(),
		WCPClient:       w.wcpAPI,
	})

	return ZoneContext{
		zoneList:          zoneList,
		cancelZoneWatches: cancelZoneWatches,
	}, nil
}

// DeleteZonesFromNamespace deletes specified zones from namespace.
func (w *WCPClusterProxy) DeleteZonesFromNamespace(ctx context.Context, nsName string, zones []string, svClusterClient ctrlclient.Client) error {
	return wcp.DeleteZonesFromNamespace(ctx, wcp.BindZonesForNamespaceInput{
		Namespace:       nsName,
		Zones:           zones,
		SvClusterClient: svClusterClient,
		Kubeconfig:      w.ClusterProxyInterface.GetKubeconfigPath(),
		WCPClient:       w.wcpAPI,
	})
}

// CreateWCPNamespaceWithNetwork applies wcp api to create a namespace in wcp cluster with the given specs.
func (w *WCPClusterProxy) CreateWCPNamespaceWithNetwork(ctx context.Context, config framework.ConfigInterface,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	scName, nsName, artifactFolder string, network wcp.NameSpaceNetworkInfo) (NamespaceContext, error) {
	// we need to create namespace with vm class and bind it
	// the CR of vm class will get deleted if the namespace owning it gets deleted
	// we can assume that vcenter has default vm classes names present
	namespace, cancelNsWatches := wcp.CreateNamespace(ctx, wcp.NamespaceCreateInput{
		SpecName:         nsName,
		Client:           w.ClusterProxyInterface.GetClient(),
		Kubeconfig:       w.ClusterProxyInterface.GetKubeconfigPath(),
		StorageClassName: scName,
		WCPClient:        w.wcpAPI,
		ArtifactFolder:   artifactFolder,
		VMServiceSpec:    vmsvcSpecs,
		Network:          &network,
	})

	return NamespaceContext{
		namespace:       namespace,
		cancelNsWatches: cancelNsWatches,
	}, nil
}

// CreateWCPNamespaceWithVMReservation creates a new namespace with VM reservations.
func (w *WCPClusterProxy) CreateWCPNamespaceWithVMReservation(
	ctx context.Context,
	nsName, scName, zone, supervisorID string,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	vmClassNameToReservedCount map[string]int) (NamespaceContext, error) {
	namespace, cancelNsWatches := wcp.CreateNamespace(ctx, wcp.NamespaceCreateInput{
		SpecName:               nsName,
		WCPClient:              w.wcpAPI,
		Kubeconfig:             w.ClusterProxyInterface.GetKubeconfigPath(),
		Client:                 w.ClusterProxyInterface.GetClient(),
		StorageClassName:       scName,
		Zone:                   zone,
		SupervisorID:           supervisorID,
		VMServiceSpec:          vmsvcSpecs,
		ReservedVMClassToCount: vmClassNameToReservedCount,
	})

	return NamespaceContext{
		namespace:       namespace,
		cancelNsWatches: cancelNsWatches,
	}, nil
}

// DeleteWCPNamespace in WCPClusterProxy type applies wcp api to delete a namespace in wcp cluster.
func (w *WCPClusterProxy) DeleteWCPNamespace(nsCtx NamespaceContext) {
	wcp.DeleteNamespace(wcp.NamespaceDeleteInput{
		Namespace:     nsCtx.namespace,
		WCPClient:     w.wcpAPI,
		CancelWatches: nsCtx.cancelNsWatches,
	})
}

type NamespaceContext struct {
	namespace       *corev1.Namespace
	cancelNsWatches context.CancelFunc
}

func (n *NamespaceContext) GetNamespace() *corev1.Namespace {
	return n.namespace
}

func (n *NamespaceContext) GetCancelNsWatches() context.CancelFunc {
	return n.cancelNsWatches
}

type ZoneContext struct {
	zoneList          *topologyv1.ZoneList
	cancelZoneWatches context.CancelFunc
}

func (z *ZoneContext) GetZoneList() *topologyv1.ZoneList {
	return z.zoneList
}
