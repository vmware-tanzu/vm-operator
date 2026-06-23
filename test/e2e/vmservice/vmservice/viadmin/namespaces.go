// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	capiutil "sigs.k8s.io/cluster-api/util"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	// vmServiceVMMgmtRoleID is the hardcoded vCenter role ID for the VM-Service-VM-Management role.
	vmServiceVMMgmtRoleID = int32(1039)
	vmServiceVMMgmtRole   = "VM-Service-VM-Management"
	administratorsGroup   = "Administrators"
	nsRoleSpecName        = "ns-admin-role"
)

type VIAdminNamespaceRoleSpecInput struct {
	Config         *e2eConfig.E2EConfig
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	ArtifactFolder string
	WCPClient      wcp.WorkloadManagementAPI
	SkipCleanup    bool
}

func VIAdminNamespaceRoleSpec(ctx context.Context, inputGetter func() VIAdminNamespaceRoleSpecInput) {
	var (
		input        VIAdminNamespaceRoleSpecInput
		clusterProxy *common.VMServiceClusterProxy
		config       *e2eConfig.E2EConfig
		nsContext    wcpframework.NamespaceContext
	)

	BeforeEach(func() {
		var err error

		input = inputGetter()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		config = input.Config

		vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{}, []string{})

		nsContext, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs,
			config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			config.InfraConfig.ManagementClusterConfig.Resources.WorkerStorageClassName,
			fmt.Sprintf("%s-%s", nsRoleSpecName, capiutil.RandomString(6)),
			input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace")
	})

	AfterEach(func() {
		clusterProxy.DeleteWCPNamespace(nsContext)
	})

	Context("When a Supervisor Namespace is created", func() {
		It("Should assign VM-Service-VM-Management role to the Administrators group on all zone folders and resource pools", Label("smoke"), func() {
			ns := nsContext.GetNamespace()
			Expect(ns).NotTo(BeNil())

			k8sClient := clusterProxy.GetClient()

			zoneList := &topologyv1.ZoneList{}
			Expect(k8sClient.List(ctx, zoneList, &ctrlclient.ListOptions{Namespace: ns.Name})).
				To(Succeed(), "failed to list zones for namespace %s", ns.Name)
			Expect(zoneList.Items).NotTo(BeEmpty(), "expected at least one zone in namespace %s", ns.Name)

			vimClient := vcenter.NewVimClientFromKubeconfig(ctx, config.InfraConfig.KubeconfigPath)
			defer vcenter.LogoutVimClient(vimClient)

			authzManager := object.NewAuthorizationManager(vimClient)

			for _, zone := range zoneList.Items {
				nsInfo := zone.Spec.Namespace

				By(fmt.Sprintf("Verifying Administrators role on folder %s in zone %s", nsInfo.FolderMoID, zone.Name))
				Expect(nsInfo.FolderMoID).NotTo(BeEmpty(),
					"zone %s has no namespace folderMoID", zone.Name)
				assertAdministratorsHaveRole(ctx, authzManager,
					types.ManagedObjectReference{Type: "Folder", Value: nsInfo.FolderMoID},
					zone.Name)

				Expect(nsInfo.PoolMoIDs).NotTo(BeEmpty(),
					"zone %s has no namespace poolMoIDs", zone.Name)
				for _, poolMoID := range nsInfo.PoolMoIDs {
					By(fmt.Sprintf("Verifying Administrators role on resource pool %s in zone %s", poolMoID, zone.Name))
					assertAdministratorsHaveRole(ctx, authzManager,
						types.ManagedObjectReference{Type: "ResourcePool", Value: poolMoID},
						zone.Name)
				}
			}
		})
	})
}

// assertAdministratorsHaveRole checks that the Administrators group has the
// VM-Service-VM-Management role (ID 1039) on the given vCenter entity.
func assertAdministratorsHaveRole(
	ctx context.Context,
	authzManager *object.AuthorizationManager,
	entity types.ManagedObjectReference,
	zoneName string,
) {
	perms, err := authzManager.RetrieveEntityPermissions(ctx, entity, false)
	Expect(err).NotTo(HaveOccurred(),
		"failed to retrieve permissions on %s %s (zone %s)", entity.Type, entity.Value, zoneName)

	found := false
	for _, perm := range perms {
		if !perm.Group {
			continue
		}
		if !strings.Contains(strings.ToLower(perm.Principal), strings.ToLower(administratorsGroup)) {
			continue
		}
		if perm.RoleId == vmServiceVMMgmtRoleID {
			found = true
			break
		}
	}

	Expect(found).To(BeTrue(),
		"expected Administrators group to have %s role (ID=%d) on %s %s (zone %s)",
		vmServiceVMMgmtRole, vmServiceVMMgmtRoleID, entity.Type, entity.Value, zoneName)
}
