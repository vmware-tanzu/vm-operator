// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	govc "github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	mopv1a2 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha2"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

// BrownfieldVMResult holds the result of a brownfield VM import operation.
type BrownfieldVMResult struct {
	// ImportedVMName is the name of the imported VirtualMachine CR.
	ImportedVMName string
	// BrownfieldVMMoID is the MoID of the vCenter VM backing the imported CR.
	BrownfieldVMMoID string
}

// BrownfieldVMCleanupInput holds the resources needed to clean up a brownfield VM after a test.
type BrownfieldVMCleanupInput struct {
	VCenterAdminClient *vim25.Client
	BrownfieldVMName   string
	BrownfieldVMMoID   string
	ImportOperation    *mopv1a2.ImportOperation
	SVClusterClient    ctrlclient.Client
}

// CleanupBrownfieldVM tears down a vCenter brownfield VM and its ImportOperation CR.
// Errors are logged but not fatal to allow best-effort cleanup in AfterEach blocks.
func CleanupBrownfieldVM(ctx context.Context, input BrownfieldVMCleanupInput) {
	if input.BrownfieldVMMoID != "" {
		By(fmt.Sprintf("Cleaning up brownfield VM %s (%s) in vCenter",
			input.BrownfieldVMName, input.BrownfieldVMMoID))

		vm := object.NewVirtualMachine(input.VCenterAdminClient, vimtypes.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: input.BrownfieldVMMoID,
		})

		powerState, err := vm.PowerState(ctx)
		if err == nil && powerState == vimtypes.VirtualMachinePowerStatePoweredOn {
			task, err := vm.PowerOff(ctx)
			if err != nil {
				e2eframework.Logf("Failed to power off VM %s: %v", input.BrownfieldVMName, err)
			} else if err = task.Wait(ctx); err != nil {
				e2eframework.Logf("Failed to wait for VM %s power off: %v", input.BrownfieldVMName, err)
			}
		}

		destroyTask, err := vm.Destroy(ctx)
		if err == nil {
			if err = destroyTask.Wait(ctx); err != nil {
				e2eframework.Logf("Failed to wait for VM %s destruction: %v", input.BrownfieldVMName, err)
			} else {
				e2eframework.Logf("Deleted brownfield VM %s", input.BrownfieldVMName)
			}
		} else {
			e2eframework.Logf("Failed to destroy VM %s: %v", input.BrownfieldVMName, err)
		}
	}

	if input.ImportOperation != nil {
		err := input.SVClusterClient.Delete(ctx, input.ImportOperation)
		if err != nil && !apierrors.IsNotFound(err) {
			e2eframework.Logf("Failed to delete ImportOperation %s: %v",
				input.ImportOperation.Name, err)
		}
	}
}

// ImportBrownfieldVMInput holds the parameters required to deploy and import a brownfield VM.
type ImportBrownfieldVMInput struct {
	// Ctx is the test context.
	Ctx context.Context
	// Config is the e2e test configuration.
	Config *e2eConfig.E2EConfig
	// VCenterAdminClient is an authenticated vCenter govmomi client.
	VCenterAdminClient *vim25.Client
	// SVClusterClient is the supervisor cluster k8s client.
	SVClusterClient ctrlclient.Client
	// Namespace is the WCP supervisor namespace.
	Namespace string
	// ClusterMoID is the MoID of the WCP cluster.
	ClusterMoID string
	// BrownfieldVMName is the name to give the vCenter VM before import.
	BrownfieldVMName string
	// StorageClassName is the storage class to use for the ImportOperation.
	StorageClassName string
	// VMClassName is the VM class to assign after import. Optional.
	VMClassName string
	// ImportOpName is the name to give the ImportOperation CR.
	ImportOpName string
	// BeforePowerOn, if non-nil, is called after the VM is deployed from the
	// content library and before it is powered on. Use it for cold
	// reconfigurations (e.g. adding shared disks with multi-writer mode) that
	// require the VM to be in a powered-off state.
	BeforePowerOn func(ctx context.Context, vm *object.VirtualMachine) error
}

// ImportBrownfieldVM deploys a photon VM from content-library into vCenter (bypassing
// VM Operator), powers it on, and then imports it into the supervisor namespace using
// ImportOperation. It returns the imported VM name and the backing vCenter MoID so the
// caller can register cleanup with AfterEach/DeferCleanup.
func ImportBrownfieldVM(input ImportBrownfieldVMInput) BrownfieldVMResult {
	ctx := input.Ctx

	By("Finding photon template in content library")

	const photonImageDisplayName = "photon-5.0"

	photonImageName := vmoperator.WaitForVirtualMachineImageName(
		ctx, &input.Config.Config, input.SVClusterClient,
		input.Namespace, photonImageDisplayName)

	photonImage := vmopv1a5.VirtualMachineImage{}
	Expect(input.SVClusterClient.Get(ctx, ctrlclient.ObjectKey{
		Name:      photonImageName,
		Namespace: input.Namespace,
	}, &photonImage)).To(Succeed(), "Failed to get photon image CR")

	libraryItemID := photonImage.Status.ProviderItemID
	Expect(libraryItemID).ToNot(BeEmpty(), "Photon image has no ProviderItemID")
	e2eframework.Logf("Found photon image %s with library item ID: %s", photonImageName, libraryItemID)

	By("Resolving cluster compute resource and datastore")

	finder := find.NewFinder(input.VCenterAdminClient, false)
	ccr, err := finder.ClusterComputeResource(ctx, input.ClusterMoID)
	Expect(err).ToNot(HaveOccurred(), "Failed to get cluster compute resource")

	datastores, err := ccr.Datastores(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to get datastores")
	Expect(datastores).ToNot(BeEmpty(), "Expected at least one datastore in the cluster")

	var datastore *object.Datastore

	for _, ds := range datastores {
		var dsMO mo.Datastore

		if err := ds.Properties(ctx, ds.Reference(), []string{"summary"}, &dsMO); err != nil {
			continue
		}

		if dsMO.Summary.MultipleHostAccess != nil && *dsMO.Summary.MultipleHostAccess {
			datastore = ds
			e2eframework.Logf("Found shared datastore: %s (type: %s)", dsMO.Summary.Name, dsMO.Summary.Type)

			break
		}
	}

	if datastore == nil {
		datastore = datastores[0]
		e2eframework.Logf("No shared datastore found, falling back to first datastore: %s", datastore.Name())
	}

	resourcePool, err := ccr.ResourcePool(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to get cluster resource pool")

	By(fmt.Sprintf("Deploying brownfield VM %s from content library item %s",
		input.BrownfieldVMName, libraryItemID))

	restClient, err := vcenter.NewRestClient(ctx, input.VCenterAdminClient, testbed.AdminUsername, testbed.AdminPassword)
	Expect(err).ToNot(HaveOccurred(), "Failed to create vCenter REST client")

	vcenterManager := govc.NewManager(restClient)
	deploySpec := govc.Deploy{
		DeploymentSpec: govc.DeploymentSpec{
			Name:               input.BrownfieldVMName,
			DefaultDatastoreID: datastore.Reference().Value,
			AcceptAllEULA:      true,
		},
		Target: govc.Target{
			ResourcePoolID: resourcePool.Reference().Value,
		},
	}

	deployedVMRef, err := vcenterManager.DeployLibraryItem(ctx, libraryItemID, deploySpec)
	Expect(err).ToNot(HaveOccurred(), "Failed to deploy VM from content library")
	Expect(deployedVMRef).ToNot(BeNil(), "Deployed VM reference is nil")

	brownfieldVMMoID := deployedVMRef.Value
	e2eframework.Logf("Deployed brownfield VM %s with MoID: %s", input.BrownfieldVMName, brownfieldVMMoID)

	brownfieldVMObj := object.NewVirtualMachine(input.VCenterAdminClient, vimtypes.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: brownfieldVMMoID,
	})

	if input.BeforePowerOn != nil {
		By("Running pre-power-on reconfigure hook")
		Expect(input.BeforePowerOn(ctx, brownfieldVMObj)).To(Succeed(), "Pre-power-on reconfigure hook failed")
	}

	By("Powering on the brownfield VM")

	powerOnTask, err := brownfieldVMObj.PowerOn(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to power on brownfield VM")
	Expect(powerOnTask.Wait(ctx)).To(Succeed(), "Failed to wait for brownfield VM power on")
	e2eframework.Logf("Brownfield VM %s powered on", input.BrownfieldVMName)

	By("Creating ImportOperation to import the brownfield VM into the supervisor namespace")

	importOperation := &mopv1a2.ImportOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.ImportOpName,
			Namespace: input.Namespace,
		},
		Spec: mopv1a2.ImportOperationSpec{
			VirtualMachineID: brownfieldVMMoID,
			StorageClass:     input.StorageClassName,
		},
	}

	Expect(input.SVClusterClient.Create(ctx, importOperation)).
		To(Succeed(), "Failed to create ImportOperation %s", input.ImportOpName)
	e2eframework.Logf("Created ImportOperation: %s", input.ImportOpName)

	By("Waiting for ImportOperation to complete")

	var importedVMName string

	Eventually(func(g Gomega) {
		g.Expect(input.SVClusterClient.Get(ctx, ctrlclient.ObjectKey{
			Namespace: input.Namespace,
			Name:      input.ImportOpName,
		}, importOperation)).To(Succeed(), "Failed to get ImportOperation")

		for _, cond := range importOperation.Status.Conditions {
			if cond.Type == "VirtualMachineCreated" && cond.Status == metav1.ConditionTrue {
				importedVMName = importOperation.Status.VirtualMachineName
				g.Expect(importedVMName).ToNot(BeEmpty(),
					"ImportOperation completed but VirtualMachineName is empty")

				return
			}

			if cond.Type == "Failed" && cond.Status == metav1.ConditionTrue {
				Fail(fmt.Sprintf("ImportOperation failed: %s", cond.Message))
			}
		}

		g.Expect(false).To(BeTrue(), "ImportOperation not yet complete")
	}, input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).
		Should(Succeed(), "Timed out waiting for ImportOperation to complete")

	e2eframework.Logf("ImportOperation completed; imported VM name: %s", importedVMName)

	return BrownfieldVMResult{
		ImportedVMName:   importedVMName,
		BrownfieldVMMoID: brownfieldVMMoID,
	}
}

// GetClusterMoIDForNamespace returns the cluster MoID from the first AvailabilityZone
// bound to the given namespace. It fails the test if no zone or cluster is found.
func GetClusterMoIDForNamespace(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	namespace string,
) string {
	zones := &topologyv1.ZoneList{}
	Expect(svClusterClient.List(ctx, zones, &ctrlclient.ListOptions{Namespace: namespace})).
		To(Succeed(), "Failed to list zones for namespace %s", namespace)
	Expect(zones.Items).ToNot(BeEmpty(),
		"Expected at least one zone bound to namespace %s", namespace)

	azName := zones.Items[0].Spec.Zone.Name
	az := &topologyv1.AvailabilityZone{}
	Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: azName}, az)).
		To(Succeed(), "Failed to get AvailabilityZone %s", azName)

	clusterMoID := az.Spec.ClusterComputeResourceMoId
	if clusterMoID == "" && len(az.Spec.ClusterComputeResourceMoIDs) > 0 {
		clusterMoID = az.Spec.ClusterComputeResourceMoIDs[0]
	}

	Expect(clusterMoID).ToNot(BeEmpty(),
		"Expected at least one cluster MoID in AvailabilityZone %s", azName)
	e2eframework.Logf("Resolved WCP cluster MoID: %s", clusterMoID)

	return clusterMoID
}
