// Copyright (c) 2019-2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
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
	)

	BeforeEach(func() {
		var err error

		suffix := capiutil.RandomString(6)

		input = inputGetter()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		wcpClient = input.WCPClient
		config = input.Config

		vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{}, []string{})
		nsContext, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs,
			config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			fmt.Sprintf("%s-%s", specName, suffix),
			input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace")
		DeferCleanup(func() {
			clusterProxy.DeleteWCPNamespace(nsContext)
		})

		vmServiceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)

		clInfo, err := wcpClient.GetContentLibrary(vmServiceCLID)
		Expect(err).NotTo(HaveOccurred())
		Expect(clInfo.StorageBackings).ToNot(BeEmpty())

		localCLName := fmt.Sprintf("e2e-local-cl-%s", suffix)
		localCLID, err := wcpClient.CreateLocalContentLibrary(
			localCLName,
			wcp.StorageBackingInfo{
				StorageBackings: []wcp.BackingInfo{clInfo.StorageBackings[0]},
			},
		)
		Expect(err).NotTo(HaveOccurred(), "failed to create local content library %q", localCLName)
		DeferCleanup(func() {
			if err := wcpClient.DeleteLocalContentLibrary(localCLID); err != nil {
				e2eframework.Logf("failed to delete local content library %s %q: %v", localCLName, localCLID, err)
			}
		})

		cls = []string{vmServiceCLID, localCLID}
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
		})
	})
}
