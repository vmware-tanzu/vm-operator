// Copyright (c) 2019-2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VIAdminCLSpecInput struct {
	Config         *e2eConfig.E2EConfig
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	ArtifactFolder string
	WCPClient      wcp.WorkloadManagementAPI
	SkipCleanup    bool
}

func VIAdminCLSpec(ctx context.Context, inputGetter func() VIAdminCLSpecInput) {
	const (
		specName = "vmcl"
	)

	var (
		input        VIAdminCLSpecInput
		wcpClient    wcp.WorkloadManagementAPI
		clusterProxy *common.VMServiceClusterProxy
		config       *e2eConfig.E2EConfig
		nsContext    wcpframework.NamespaceContext
		cls          []string
		// localCLID holds the ID of the local content library created by BeforeEach
		// so it can be cleaned up in AfterEach. It is only set when we create one.
		localCLID string
	)

	BeforeEach(func() {
		var err error

		input = inputGetter()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		wcpClient = input.WCPClient
		config = input.Config
		localCLID = ""
		vmClassNames, contentLibraryNames := []string{}, []string{}
		vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, contentLibraryNames)

		// VIAdminCLSpec will update the WCP namespace by overwriting its content library association.
		// Therefore, we are not using the default namespace and creating a new one for each test spec.
		nsContext, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs,
			config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			config.InfraConfig.ManagementClusterConfig.Resources.WorkerStorageClassName,
			fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)),
			input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace")

		cls, err = wcpClient.ListContentLibraries()
		Expect(err).NotTo(HaveOccurred(), "failed to list content libraries")

		// Ensure there are at least two content libraries so the multi-CL
		// association tests have something to work with. On VDS testbeds that
		// do not include TKG, only the vmservice subscribed library exists.
		// We create a throwaway local library to satisfy the requirement and
		// clean it up in AfterEach.
		if len(cls) < 2 {
			datastores, err := wcpClient.ListDatastores()
			Expect(err).NotTo(HaveOccurred(), "failed to list datastores for local CL")
			var datastoreID string
			for _, dsName := range []string{"vsanDatastore", "sharedVmfs-0", "nfs0-1"} {
				for _, ds := range datastores {
					if ds.Name == dsName {
						datastoreID = ds.Datastore
						break
					}
				}
				if datastoreID != "" {
					break
				}
			}
			Expect(datastoreID).NotTo(BeEmpty(), "no suitable datastore found for local content library")

			localCLName := fmt.Sprintf("e2e-local-cl-%s", capiutil.RandomString(6))
			localCLID, err = wcpClient.CreateLocalContentLibrary(localCLName, wcp.StorageBackingInfo{
				StorageBackings: []wcp.BackingInfo{{DatastoreID: datastoreID, Type: "DATASTORE"}},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to create local content library %q", localCLName)
			cls = append(cls, localCLID)
		}

		Expect(len(cls)).Should(BeNumerically(">=", 2),
			"expected at least 2 content libraries but found %d", len(cls))
	})

	AfterEach(func() {
		clusterProxy.DeleteWCPNamespace(nsContext)
		if localCLID != "" {
			if err := wcpClient.DeleteLocalContentLibrary(localCLID); err != nil {
				GinkgoWriter.Printf("Warning: failed to delete local content library %q: %v\n", localCLID, err)
			}
			localCLID = ""
		}
	})

	Context("When testing content library association workflow with valid params", func() {
		It("Should associate single valid content library", Label("smoke"), func() {
			vmservice.VerifyCLAssociation(wcpClient, nsContext.GetNamespace().Name, cls[:1])
		})

		It("Should associate multiple valid content library", func() {
			vmservice.VerifyCLAssociation(wcpClient, nsContext.GetNamespace().Name, cls)
		})

		It("Should associate then disassociate content library", func() {
			// Associate content libraries.
			vmservice.VerifyCLAssociation(wcpClient, nsContext.GetNamespace().Name, cls)

			// Disassociate content libraries and verify removed CLs are not associated to the namespace.
			vmservice.VerifyCLAssociation(wcpClient, nsContext.GetNamespace().Name, cls[0:1])
			vmservice.CheckCLDisassociation(wcpClient, nsContext.GetNamespace().Name, cls[1:])
			/* TODO (dramdass): Figure out how/if dcli supports update to empty list or use set instead of update
			cls = []string{""}
			VerifyCLAssociation(wcpClient, nsContext.GetNamespace().Name, cls)
			*/
		})
	})
}
