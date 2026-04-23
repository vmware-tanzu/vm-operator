package wcp

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"k8s.io/client-go/kubernetes"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type BindZonesForNamespaceInput struct {
	Namespace       string
	Zones           []string
	Kubeconfig      string
	ArtifactFolder  string
	ClientSet       *kubernetes.Clientset
	SvClusterClient ctrlclient.Client
	WCPClient       WorkloadManagementAPI
}

type ZonesGetInput struct {
	SupervisorID string
	WCPClient    WorkloadManagementAPI
}

type ZonesBindingInput struct {
	SupervisorID string
	Zones        []string
	WCPClient    WorkloadManagementAPI
}

type ZoneList struct {
	Zones []ZoneDetails `json:"zones"`
}

type ZoneDetails struct {
	Zone       string        `json:"zone"`
	Type       string        `json:"type"`
	Namespaces []string      `json:"namespaces"`
	Status     string        `json:"status"`
	Messages   []ZoneMessage `json:"messages"`
}

type ZoneMessage struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type VSphereZoneList struct {
	VSphereZones []VSphereZoneItem `json:"items"`
}

type VSphereZoneItem struct {
	Zone string          `json:"zone"`
	Info VSphereZoneInfo `json:"info"`
}

type VSphereZoneInfo struct {
	Description string `json:"description"`
}

func WaitUpdateNamespaceWithZonesReady(svClusterClient ctrlclient.Client, testNamespace string, zones []string) {
	Eventually(func() bool {
		retZoneList, err := ListZonesByNamespace(context.Background(), svClusterClient, testNamespace)
		if err != nil {
			return false
		}
		// By default, namespace is already bound with zone-1
		return len(retZoneList.Items) == len(zones)+1
	}, 180*time.Second, 10*time.Second).Should(BeTrue(), "Zones ", zones, "update namespace with Zones not READY in time")
}

func WaitForZoneRemovalFromNamespace(svClusterClient ctrlclient.Client, testNamespace string, zone string) {
	Eventually(func() bool {
		retZoneList, err := ListZonesByNamespace(context.Background(), svClusterClient, testNamespace)
		if err != nil {
			return false
		}

		for _, item := range retZoneList.Items {
			if item.Name == zone {
				// If the zone is found, it hasn't been removed yet
				return false
			}
		}

		// If the zone is not found, it has been successfully removed
		return true
	}, 1800*time.Second, 20*time.Second).Should(BeTrue(), "Zone ", zone, "delete zone from namespace failed within the timeout")
}

func WaitUpdateZoneBindingsSupervisorReady(wcpClient WorkloadManagementAPI, supervisorID string, expectedZones int) {
	Eventually(func() bool {
		details, err := wcpClient.GetZonesBoundWithSupervisor(supervisorID)
		if err != nil {
			e2eframework.Logf("Failed to get zones bound with supervisor %s due to %v", supervisorID, err)
			return false
		}

		// Check if the number of zones matches the expectedZones
		if len(details.Zones) != expectedZones {
			e2eframework.Logf("Expected %d Zones bound with Supervisor %s, actual %v Zones", expectedZones, supervisorID, len(details.Zones))
			return false
		}

		supervisorSummaryList, err := wcpClient.ListSupervisorSummary()
		if err != nil {
			e2eframework.Logf("Failed to list supervisor Summary %v", err)
			return false
		}

		for _, supervisorSummary := range supervisorSummaryList.Supervisors {
			if supervisorSummary.Supervisor == supervisorID {
				return supervisorSummary.Info.ConfigStatus == "RUNNING"
			}
		}

		return false
	}, 600*time.Second, 15*time.Second).Should(BeTrue(), "Expected Zones", expectedZones, "update Zone bindings with supervisor is not READY in time")
}

// UpdateNamespaceWithZones binds provided Zones for a WCP kubernetes namespace object.
func UpdateNamespaceWithZones(ctx context.Context, input BindZonesForNamespaceInput) (*topologyv1.ZoneList, context.CancelFunc) {
	testNamespace := input.Namespace
	wcpClient := input.WCPClient
	svKubeConfig := input.Kubeconfig
	// Get supervisor ID
	supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, svKubeConfig)
	Expect(supervisorID).NotTo(BeEmpty(), "Unable to get supervisor ID")

	zoneSpecs := make([]ZoneSpec, 0, len(input.Zones))
	for _, z := range input.Zones {
		zSpec := ZoneSpec{Name: z}
		zoneSpecs = append(zoneSpecs, zSpec)
	}

	err := wcpClient.UpdateNamespaceWithZones(testNamespace, zoneSpecs)
	Expect(err).NotTo(HaveOccurred())
	WaitUpdateNamespaceWithZonesReady(input.SvClusterClient, testNamespace, input.Zones)

	_, cancelWatches := context.WithCancel(ctx)
	retZoneList, err := ListZonesByNamespace(ctx, input.SvClusterClient, input.Namespace)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(retZoneList.Items)).To(BeNumerically(">=", 2))

	return retZoneList, cancelWatches
}

// DeleteZonesFromNamespace deletes specified Zones for a WCP kubernetes namespace object.
func DeleteZonesFromNamespace(ctx context.Context, input BindZonesForNamespaceInput) error {
	testNamespace := input.Namespace
	wcpClient := input.WCPClient

	for _, zone := range input.Zones {
		err := wcpClient.DeleteZoneFromNamespace(testNamespace, zone)
		Expect(err).NotTo(HaveOccurred())
		WaitForZoneRemovalFromNamespace(input.SvClusterClient, testNamespace, zone)
	}

	return nil
}

// GetZonesBoundWithSupervisor gets zones bound with supervisor.
func GetZonesBoundWithSupervisor(input ZonesGetInput) (ZoneList, error) {
	wcpClient := input.WCPClient
	supervisorID := input.SupervisorID

	zoneList, err := wcpClient.GetZonesBoundWithSupervisor(supervisorID)
	if err != nil {
		return ZoneList{}, err
	}

	return zoneList, nil
}

// CreateZoneBindingsWithSupervisor creates Zone bindings with supervisor.
func CreateZoneBindingsWithSupervisor(input ZonesBindingInput) error {
	wcpClient := input.WCPClient
	supervisorID := input.SupervisorID
	zones := input.Zones

	currentZones, err := wcpClient.GetZonesBoundWithSupervisor(supervisorID)
	if err != nil {
		return err
	}

	specs := make([]ZoneBindingSpecs, 0, len(zones))
	for _, zone := range zones {
		spec := ZoneBindingSpecs{Zone: zone, Type: "WORKLOAD"}
		specs = append(specs, spec)
	}

	if err = wcpClient.CreateZoneBindingsWithSupervisor(supervisorID, specs); err != nil {
		return err
	}

	WaitUpdateZoneBindingsSupervisorReady(wcpClient, supervisorID, len(currentZones.Zones)+len(zones))

	return nil
}

// DeleteZoneBindingsWithSupervisor delete Zone bindings with supervisor.
func DeleteZoneBindingsWithSupervisor(input ZonesBindingInput) error {
	wcpClient := input.WCPClient
	supervisorID := input.SupervisorID
	zones := input.Zones

	currentZones, err := wcpClient.GetZonesBoundWithSupervisor(supervisorID)
	if err != nil {
		return err
	}
	// Supervisor don’t allow > 1 zone to be removed at the same time. If one is being removed, need to wait until the
	// first is removed and remove another
	for i := 0; i < (len(zones)); i++ {
		err = wcpClient.DeleteZoneBindingsWithSupervisor(supervisorID, zones[i])
		if err != nil {
			return fmt.Errorf("failed to delete zone binding for %s: %w", zones[i], err)
		}

		WaitUpdateZoneBindingsSupervisorReady(wcpClient, supervisorID, len(currentZones.Zones)-i-1)
		// Sleep for 30 min to wait for the Zone binding is fully deleted even after it is removed from zone binding list.
		// TODO: This is a temporary workaround for wcp issue https://bugzilla-vcf.lvn.broadcom.net/show_bug.cgi?id=3493084.
		// Remove it when issue is fixed.
		time.Sleep(1800 * time.Second)
	}

	return nil
}

func ListVSphereZones(wcpClient WorkloadManagementAPI) (VSphereZoneList, error) {
	vSphereZoneList, err := wcpClient.ListVSphereZones()
	if err != nil {
		return VSphereZoneList{}, err
	}

	return vSphereZoneList, nil
}
