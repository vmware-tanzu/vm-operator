// Copyright (c) 2021-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VMMultipleClusterInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

func VMMultipleClusterSpec(ctx context.Context, inputGetter func() VMMultipleClusterInput) {
	const (
		specName = "vm-multiple-cluster"
		zoneName = "zone-2"
	)

	var (
		input            VMMultipleClusterInput
		config           *e2eConfig.E2EConfig
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources

		vmName string

		linuxVMIName string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		// Only run this test in the stretched supervisor environment.
		skipper.SkipUnlessStretchSupervisorIsEnabled()

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		svClusterConfig := config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		svClusterClient = input.ClusterProxy.GetClient()
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)

		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, input.ClusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)
	})

	Context("When there are multiple clusters", func() {
		When("Create a VM with a zone assigned", func() {
			It("Should create successfully and be assigned to the correct zone", Label("smoke"), func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), input.WCPNamespaceName)
				Expect(err).NotTo(HaveOccurred())

				var bound bool

				for _, zone := range zoneList.Items {
					if zone.Name == zoneName {
						bound = true
						break
					}
				}

				if !bound {
					// Bind zone-2 to namespace.
					zones := []string{zoneName}
					_, err := input.ClusterProxy.UpdateNamespaceWithZones(ctx, input.WCPNamespaceName, zones, svClusterClient)
					Expect(err).NotTo(HaveOccurred(), "failed to update namespace with Zones")
				}

				vmToCreate := &vmopv1a2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: input.WCPNamespaceName,
						Name:      vmName,
						Labels:    map[string]string{"topology.kubernetes.io/zone": zoneName},
					},
					Spec: vmopv1a2.VirtualMachineSpec{
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a2.VirtualMachinePowerStateOn,
					},
				}
				vmoperator.CreateVirtualMachine(ctx, config, input.ClusterProxy, vmToCreate, false)

				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOn)
				vmoperator.WaitForVirtualMachineZone(ctx, config, svClusterClient, input.WCPNamespaceName, zoneName, vmName)

				// Verify that this VM is created in the specific cluster.
				vmservice.VerifyVMInZone(ctx, config, svClusterClient, input.WCPNamespaceName, zoneName, vmName)
			})
		})

		When("Create a VM without a zone assigned", func() {
			It("Should create successfully and be assigned to a zone", func() {
				vmToCreate := &vmopv1a1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: input.WCPNamespaceName,
						Name:      vmName,
					},
					Spec: vmopv1a1.VirtualMachineSpec{
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a1.VirtualMachinePoweredOn,
					},
				}
				vmoperator.CreateVirtualMachine(ctx, config, input.ClusterProxy, vmToCreate, false)

				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOn)
				vmoperator.WaitForVirtualMachineZone(ctx, config, svClusterClient, input.WCPNamespaceName, "", vmName)
			})
		})
	})
}
