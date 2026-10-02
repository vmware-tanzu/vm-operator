// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vsanDefaultStoragePolicyName = "vSAN Default Storage Policy"

	// storagePolicyIDParameter is the StorageClass parameter that holds the
	// ID of the backing vSphere storage policy.
	storagePolicyIDParameter = "storagePolicyID"

	datastoreSummaryTypeVSAN = "vsan"
)

type VIAdminStoragePolicySpecInput struct {
	ClusterProxy wcpframework.WCPClusterProxyInterface
	Config       *config.E2EConfig
}

// VIAdminStoragePolicySpec verifies the datastores VM Operator reports as
// compatible with a storage policy in the StoragePolicy status.
func VIAdminStoragePolicySpec(ctx context.Context, inputGetter func() VIAdminStoragePolicySpecInput) {
	var (
		input          VIAdminStoragePolicySpecInput
		config         *config.E2EConfig
		svClient       ctrlclient.Client
		vimClient      *vim25.Client
		kubeconfigPath string
		byokFSSEnabled bool
	)

	BeforeEach(func() {
		input = inputGetter()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		config = input.Config
		clusterProxy := input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClient = clusterProxy.GetClient()
		kubeconfigPath = clusterProxy.GetKubeconfigPath()
		vimClient = vcenter.NewVimClientFromKubeconfig(ctx, kubeconfigPath)

		// The StorageClass controller, which creates the StoragePolicy
		// objects, runs when BYOK or FastDeploy is enabled. Only BYOK has an
		// FSS that can be checked here.
		byokFSSEnabled = utils.IsFssEnabled(ctx, svClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSBYOK"))
	})

	AfterEach(func() {
		if vimClient != nil {
			vcenter.LogoutVimClient(vimClient)
			vimClient = nil
		}
	})

	It("should report only vSAN datastores as compatible with the vSAN default storage policy", Label("core-functional"), func() {
		isVSANEnabled, err := vcenter.IsVSANEnabledCluster(ctx, vimClient, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())
		if !isVSANEnabled {
			Skip("Supervisor cluster does not have a vSAN datastore")
		}

		policyID, err := vcenter.GetStoragePolicyIDFromName(vimClient, vsanDefaultStoragePolicyName)
		Expect(err).ToNot(HaveOccurred())
		Expect(policyID).ToNot(BeEmpty())

		var scList storagev1.StorageClassList
		Expect(svClient.List(ctx, &scList)).To(Succeed())
		hasStorageClass := false
		for _, sc := range scList.Items {
			if strings.EqualFold(sc.Parameters[storagePolicyIDParameter], policyID) {
				hasStorageClass = true
				break
			}
		}
		if !hasStorageClass {
			Skip("No StorageClass references the vSAN default storage policy")
		}

		getStoragePolicy := func() (*infrav1.StoragePolicy, error) {
			var list infrav1.StoragePolicyList
			if err := svClient.List(ctx, &list, ctrlclient.InNamespace(config.GetVariable("VMOPNamespace"))); err != nil {
				return nil, err
			}
			for i := range list.Items {
				if strings.EqualFold(list.Items[i].Spec.ID, policyID) {
					return &list.Items[i], nil
				}
			}
			return nil, nil
		}

		if !byokFSSEnabled {
			// Without BYOK, the StoragePolicy object only exists if
			// FastDeploy is enabled, which cannot be checked from here.
			obj, err := getStoragePolicy()
			Expect(err).ToNot(HaveOccurred())
			if obj == nil {
				Skip("StoragePolicy objects are not created on this Supervisor")
			}
		}

		var storagePolicy *infrav1.StoragePolicy
		Eventually(func(g Gomega) {
			obj, err := getStoragePolicy()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(obj).ToNot(BeNil(), "StoragePolicy for policy %s not found", policyID)
			g.Expect(obj.Status.Datastores).ToNot(BeEmpty())
			storagePolicy = obj
		}, config.GetIntervals("default", "wait-storage-policy-status")...).Should(Succeed(),
			"Timed out waiting for StoragePolicy status to list datastores")

		dsRefs := make([]vimtypes.ManagedObjectReference, 0, len(storagePolicy.Status.Datastores))
		for _, ds := range storagePolicy.Status.Datastores {
			Expect(ds.Type).To(Equal(infrav1.DatastoreTypeVSAN),
				"StoragePolicy %s lists non-vSAN datastore %s", storagePolicy.Name, ds.ID.ObjectID)
			dsRefs = append(dsRefs, vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeDatastore),
				Value: ds.ID.ObjectID,
			})
		}

		var datastores []mo.Datastore
		Expect(property.DefaultCollector(vimClient).Retrieve(ctx, dsRefs, []string{"summary.type"}, &datastores)).To(Succeed())
		Expect(datastores).To(HaveLen(len(dsRefs)))
		for _, ds := range datastores {
			Expect(ds.Summary.Type).To(Equal(datastoreSummaryTypeVSAN),
				"StoragePolicy %s lists datastore %s of type %s", storagePolicy.Name, ds.Reference().Value, ds.Summary.Type)
		}

		logIfAllDatastoresAreVSAN(ctx, vimClient)
	})
}

// logIfAllDatastoresAreVSAN notes in the test output when vCenter has no
// non-vSAN datastores, since then the test cannot detect non-vSAN datastores
// being reported as compatible.
func logIfAllDatastoresAreVSAN(ctx context.Context, vimClient *vim25.Client) {
	dsType := string(vimtypes.ManagedObjectTypeDatastore)
	v, err := view.NewManager(vimClient).CreateContainerView(ctx, vimClient.ServiceContent.RootFolder, []string{dsType}, true)
	Expect(err).ToNot(HaveOccurred())
	defer func() {
		_ = v.Destroy(ctx)
	}()

	var datastores []mo.Datastore
	Expect(v.Retrieve(ctx, []string{dsType}, []string{"summary.type"}, &datastores)).To(Succeed())
	for _, ds := range datastores {
		if ds.Summary.Type != datastoreSummaryTypeVSAN {
			return
		}
	}
	By("vCenter has only vSAN datastores, so this run cannot detect non-vSAN datastores reported as compatible")
}
