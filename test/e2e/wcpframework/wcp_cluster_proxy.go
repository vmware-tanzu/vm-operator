package wcpframework

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
)

const (
	// Workload namespace annotations usually set by wcpsvc. Set here for simulated wcp
	// to mimic actual wcp environment.
	resourcePoolNsAnnotation = "vmware-system-resource-pool"
	vmFolderNsAnnotation     = "vmware-system-vm-folder"

	// Workload namespace labels usually set by wcpsvc. Set here for simulated wcp
	// to mimic actual wcp environment.
	userWorkloadNamespaceLabel = "vSphereClusterID"
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
		scName, wscName, nsName, artifactFolder string) (NamespaceContext, error)
	UpdateNamespaceWithZones(ctx context.Context, namespaceName string, zones []string, svClusterClient ctrlclient.Client) (ZoneContext, error)
	DeleteZonesFromNamespace(ctx context.Context, namespaceName string, zones []string, svClusterClient ctrlclient.Client) error
	GetZonesBoundWithSupervisor(supervisorID string) (wcp.ZoneList, error)
	CreateZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	CreateMissingZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	DeleteZoneBindingsWithSupervisor(supervisorID string, zones []string) error
	ListVSphereZones() (wcp.VSphereZoneList, error)
	CreateWCPNamespaceWithNetwork(ctx context.Context, config framework.ConfigInterface,
		vmsvcSpecs wcp.VMServiceSpecDetails,
		scName, wscName, nsName, artifactFolder string, network wcp.NameSpaceNetworkInfo) (NamespaceContext, error)
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
	scName, wscName, nsName, artifactFolder string) (NamespaceContext, error) {
	// we need to create namespace with vm class and bind it
	// the CR of vm class will get deleted if the namespace owning it gets deleted
	// we can assume that vcenter has default vm classes names present
	namespace, cancelNsWatches := wcp.CreateNamespace(ctx, wcp.NamespaceCreateInput{
		SpecName:               nsName,
		ClientSet:              w.ClusterProxyInterface.GetClientSet(),
		Client:                 w.ClusterProxyInterface.GetClient(),
		Kubeconfig:             w.ClusterProxyInterface.GetKubeconfigPath(),
		StorageClassName:       scName,
		WorkerStorageClassName: wscName,
		Config:                 config,
		WCPClient:              w.wcpAPI,
		ArtifactFolder:         artifactFolder,
		VMServiceSpec:          vmsvcSpecs,
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
		ClientSet:       w.ClusterProxyInterface.GetClientSet(),
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
		ClientSet:       w.ClusterProxyInterface.GetClientSet(),
		Kubeconfig:      w.ClusterProxyInterface.GetKubeconfigPath(),
		WCPClient:       w.wcpAPI,
	})
}

// CreateWCPNamespaceWithNetwork applies wcp api to create a namespace in wcp cluster with the given specs.
func (w *WCPClusterProxy) CreateWCPNamespaceWithNetwork(ctx context.Context, config framework.ConfigInterface,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	scName, wscName, nsName, artifactFolder string, network wcp.NameSpaceNetworkInfo) (NamespaceContext, error) {
	// we need to create namespace with vm class and bind it
	// the CR of vm class will get deleted if the namespace owning it gets deleted
	// we can assume that vcenter has default vm classes names present
	namespace, cancelNsWatches := wcp.CreateNamespace(ctx, wcp.NamespaceCreateInput{
		SpecName:               nsName,
		ClientSet:              w.ClusterProxyInterface.GetClientSet(),
		Client:                 w.ClusterProxyInterface.GetClient(),
		Kubeconfig:             w.ClusterProxyInterface.GetKubeconfigPath(),
		StorageClassName:       scName,
		WorkerStorageClassName: wscName,
		Config:                 config,
		WCPClient:              w.wcpAPI,
		ArtifactFolder:         artifactFolder,
		VMServiceSpec:          vmsvcSpecs,
		Network:                &network,
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
		ClientSet:              w.ClusterProxyInterface.GetClientSet(),
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

// SimulatedWCPClusterProxy implements WCP cluster proxy interface without workloadmanagement API
// Simulated wcp proxy is running in kind environment so it includes necessary steps to create
// a wcp namespace generated by canonical kubernetes behaviors.
type SimulatedWCPClusterProxy struct {
	framework.ClusterProxyInterface
}

// Instantiate a simulated wcp cluster proxy object.
func NewSimulatedWCPClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *SimulatedWCPClusterProxy {
	kubeClusterProxy := framework.NewClusterProxy(name, kubeconfigPath, scheme)

	return &SimulatedWCPClusterProxy{
		ClusterProxyInterface: kubeClusterProxy,
	}
}

// Apply wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func (s *SimulatedWCPClusterProxy) applyWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	return framework.KubectlApplyWithArgs(ctx, s.GetKubeconfigPath(), resources, args...)
}

// CreateWCPNamespace in simulatedwcpcluster type implements how to create a wcp namespace in kind cluster.
func (s *SimulatedWCPClusterProxy) CreateWCPNamespaceWithNetwork(ctx context.Context, config framework.ConfigInterface,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	scName, wscName, nsName, artifactFolder string, network wcp.NameSpaceNetworkInfo) (NamespaceContext, error) {
	return NamespaceContext{}, nil
}

// CreateWCPNamespaceWithVMReservation creates a wcp namespace in kind cluster with VM reservation.
// Note: this is needed to compile gce2e but not implemented as it's not used in kind cluster.
func (s *SimulatedWCPClusterProxy) CreateWCPNamespaceWithVMReservation(
	ctx context.Context,
	nsName, scName, zone, supervisorID string,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	vmClassNameToReservedCount map[string]int) (NamespaceContext, error) {
	return NamespaceContext{}, nil
}

// CreateWCPNamespace in simulatedwcpcluster type implements how to create a wcp namespace in kind cluster.
func (s *SimulatedWCPClusterProxy) CreateWCPNamespace(ctx context.Context, config framework.ConfigInterface,
	vmsvcSpecs wcp.VMServiceSpecDetails,
	scName, wscName, nsName, artifactFolder string) (NamespaceContext, error) {
	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   s.ClusterProxyInterface.GetClient(),
		ClientSet: s.ClusterProxyInterface.GetClientSet(),
		Name:      nsName,
		LogFolder: filepath.Join(artifactFolder, "wcp-simulated-clusters", s.ClusterProxyInterface.GetName()),
	})

	if err := s.setUserWorkloadNamespaceLabel(ctx, config, nsName); err != nil {
		return NamespaceContext{}, fmt.Errorf("failed to set user workload namespace label: %w", err)
	}

	// Only set namespace resource pool and folder when FSS_WCP_FAULTDOMAINS is not enabled.
	// Otherwise we would get resource pool and folder info from availability zones.
	haFssEnabled := utils.IsFssEnabled(ctx, s.ClusterProxyInterface.GetClient(),
		config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSHA"))
	if !haFssEnabled {
		err := s.setNamespaceRPAndFolder(ctx, config, nsName)
		if err != nil {
			return NamespaceContext{}, fmt.Errorf("failed to set namespace resource pool and folder: %w", err)
		}
	} else {
		// Add to AZs.
		err := s.setAvailabilityZones(ctx, s.ClusterProxyInterface.GetClient(), config, nsName)
		if err != nil {
			return NamespaceContext{}, fmt.Errorf("failed to add namespace to availability zones: %w", err)
		}
	}

	storageQuotaYAML, err := manifestbuilders.GetStorageQuotaYAML()
	if err != nil {
		return NamespaceContext{}, fmt.Errorf("get storage quota YAML file failed, please check fixture folder and manifestbuilder package")
	}

	err = s.applyWithArgs(ctx, storageQuotaYAML, "-n", namespace.Name)
	if err != nil {
		return NamespaceContext{}, fmt.Errorf("apply storage quota to namespace %q", namespace.Name)
	}

	// When FSS_WCP_NAMESPACED_VM_CLASS is enabled,
	// manually create namespaced VirtualMachineClass resources,
	// else, create VirtualMachineClassBinding resources in the cluster.
	// Note: in a real WCP env, when we associate VM class to a namespace, VM class binding will be created accordingly.
	// In a kind cluster, there is no wcpsvc, manually create them as a hack.
	NamespacedVMClassFssEnabled := utils.IsFssEnabled(ctx, s.ClusterProxyInterface.GetClient(),
		config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSNamespacedVMClass"))

	e2eframework.Logf("vmsvcSpecs: %+v", vmsvcSpecs)

	if NamespacedVMClassFssEnabled {
		for _, vmClass := range vmsvcSpecs.VMClasses {
			e2eframework.Logf("Create Namespaced VM class: %s", vmClass)
			vmclassYAML := manifestbuilders.GetVirtualMachineClassYaml(namespace.Name, vmClass)
			e2eframework.Logf("%v", string(vmclassYAML))

			err := s.applyWithArgs(ctx, vmclassYAML, "-n", namespace.Name)
			if err != nil {
				return NamespaceContext{}, fmt.Errorf("apply VM class to namespace %q failed", namespace.Name)
			}
		}
	} else {
		for _, vmClass := range vmsvcSpecs.VMClasses {
			e2eframework.Logf("Create VM class binding: %s", vmClass)
			vmclassBindingYAML := manifestbuilders.GetVirtualMachineClassBindingYaml(namespace.Name, vmClass)
			e2eframework.Logf("%v", string(vmclassBindingYAML))

			err := s.applyWithArgs(ctx, vmclassBindingYAML, "-n", namespace.Name)
			if err != nil {
				return NamespaceContext{}, fmt.Errorf("apply VM class binding to namespace %q failed", namespace.Name)
			}
		}
	}

	for _, contentSource := range vmsvcSpecs.ContentLibraries {
		e2eframework.Logf("Create content source binding: %s", contentSource)
		contentSourceBindingYAML := manifestbuilders.GetContentSourceBindingYaml(namespace.Name, contentSource)
		e2eframework.Logf("%v", string(contentSourceBindingYAML))

		err := s.applyWithArgs(ctx, contentSourceBindingYAML, "-n", namespace.Name)
		if err != nil {
			return NamespaceContext{}, fmt.Errorf("apply content source binding to namespace %q failed", namespace.Name)
		}
	}

	return NamespaceContext{
		namespace:       namespace,
		cancelNsWatches: cancelWatches,
	}, nil
}

func (s *SimulatedWCPClusterProxy) GetZonesBoundWithSupervisor(supervisorID string) (wcp.ZoneList, error) {
	return wcp.ZoneList{}, errors.New("getting Zones bound with Supervisor is not implemented")
}

func (s *SimulatedWCPClusterProxy) UpdateNamespaceWithZones(ctx context.Context, nsName string, zones []string, svClusterClient ctrlclient.Client) (ZoneContext, error) {
	return ZoneContext{}, errors.New("updating namespace with Zones is not implemented")
}

// DeleteZonesFromNamespace deletes specified zones from namespace.
func (w *SimulatedWCPClusterProxy) DeleteZonesFromNamespace(ctx context.Context, nsName string, zones []string, svClusterClient ctrlclient.Client) error {
	return errors.New("deleting Zone from namespace is not implemented")
}

func (w *SimulatedWCPClusterProxy) CreateZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	return errors.New("creating zone bindings with supervisor is not implemented")
}

func (w *SimulatedWCPClusterProxy) CreateMissingZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	return errors.New("creating missing zone bindings with supervisor is not implemented")
}

func (w *SimulatedWCPClusterProxy) DeleteZoneBindingsWithSupervisor(supervisorID string, zones []string) error {
	return errors.New("deleting zone bindings with supervisor is not implemented")
}

func (w *SimulatedWCPClusterProxy) ListVSphereZones() (wcp.VSphereZoneList, error) {
	return wcp.VSphereZoneList{}, errors.New("list VSphereZones is not implemented")
}

func (s *SimulatedWCPClusterProxy) setUserWorkloadNamespaceLabel(
	ctx context.Context,
	config framework.ConfigInterface,
	nsName string) error {
	ctrlClient := s.ClusterProxyInterface.GetClient()
	vcip := utils.GetVcPNID(ctx, ctrlClient, config.GetVariable("VMOPNamespace"))
	vimClient := vcenter.NewVcSimClient(ctx, vcip)
	finder := vcenter.NewVcsimFinder(ctx, vimClient)
	clusters := vcenter.GetClusterRefs(ctx, finder)

	namespace := &corev1.Namespace{}

	err := ctrlClient.Get(ctx, ctrlclient.ObjectKey{Name: nsName}, namespace)
	if err != nil {
		return err
	}

	if namespace.Labels == nil {
		namespace.Labels = map[string]string{}
	}
	// Simply set the cluster value to the first cluster in a kind setup.
	namespace.Labels[userWorkloadNamespaceLabel] = clusters[0].Reference().Value

	return ctrlClient.Update(ctx, namespace)
}

func (s *SimulatedWCPClusterProxy) setNamespaceRPAndFolder(
	ctx context.Context,
	config framework.ConfigInterface,
	nsName string) error {
	ctrlClient := s.ClusterProxyInterface.GetClient()
	vcip := utils.GetVcPNID(ctx, ctrlClient, config.GetVariable("VMOPNamespace"))
	vimClient := vcenter.NewVcSimClient(ctx, vcip)
	finder := vcenter.NewVcsimFinder(ctx, vimClient)
	clusters := vcenter.GetClusterRefs(ctx, finder)
	resourcePool, folder := vcenter.GetResourcePoolAndFolder(ctx, finder, clusters[0])

	namespace := &corev1.Namespace{}

	err := ctrlClient.Get(ctx, ctrlclient.ObjectKey{Name: nsName}, namespace)
	if err != nil {
		return err
	}

	if namespace.Annotations == nil {
		namespace.Annotations = map[string]string{}
	}

	namespace.Annotations[resourcePoolNsAnnotation] = resourcePool
	namespace.Annotations[vmFolderNsAnnotation] = folder

	return ctrlClient.Update(ctx, namespace)
}

// DeleteWCPNamespace in simulatedwcpcluster type implements how to delete a wcp namespace in kind cluster.
func (s *SimulatedWCPClusterProxy) DeleteWCPNamespace(nsCtx NamespaceContext) {
	ctx := context.TODO()
	c := s.ClusterProxyInterface.GetClient()
	// Remove from availability zones.
	azs, err := utils.ListAvailabilityZones(ctx, c)
	Expect(err).ShouldNot(HaveOccurred())

	for _, az := range azs.Items {
		item := az
		delete(item.Spec.Namespaces, nsCtx.namespace.Name)
		Expect(c.Update(ctx, &item)).ShouldNot(HaveOccurred())
	}

	framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
		Deleter: c,
		Name:    nsCtx.namespace.Name,
	})
	nsCtx.cancelNsWatches()
}

func (s *SimulatedWCPClusterProxy) setAvailabilityZones(ctx context.Context,
	c ctrlclient.Client,
	config framework.ConfigInterface,
	ns string) error {
	azs, err := utils.ListAvailabilityZones(ctx, c)
	if err != nil {
		return err
	}

	ctrlClient := s.ClusterProxyInterface.GetClient()
	vcip := utils.GetVcPNID(ctx, ctrlClient, config.GetVariable("VMOPNamespace"))
	vimClient := vcenter.NewVcSimClient(ctx, vcip)
	finder := vcenter.NewVcsimFinder(ctx, vimClient)

	clusterRefs := vcenter.GetClusterRefs(ctx, finder)
	refMapping := vcenter.GetClusterIDToRefMapping(clusterRefs)

	for _, az := range azs.Items {
		item := az

		Expect(item.Spec.ClusterComputeResourceMoIDs).ToNot(BeEmpty())
		clusterID := item.Spec.ClusterComputeResourceMoIDs[0]

		rp, folder := vcenter.GetResourcePoolAndFolder(ctx, finder, refMapping[clusterID])

		if item.Spec.Namespaces == nil {
			item.Spec.Namespaces = map[string]topologyv1.NamespaceInfo{}
		}

		item.Spec.Namespaces[ns] = topologyv1.NamespaceInfo{
			PoolMoId:   rp,
			PoolMoIDs:  []string{rp},
			FolderMoId: folder,
		}
		Expect(c.Update(ctx, &item)).ShouldNot(HaveOccurred())
	}

	return nil
}
