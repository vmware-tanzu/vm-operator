// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	virtualMachineKind        = "VirtualMachine"
	virtualMachineServiceKind = "VirtualMachineService"
)

type SpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	ArtifactFolder   string
	WCPNamespaceName string
}

func VirtualMachineServiceSpec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "virtual-machine-service"

	var (
		input            SpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources

		linuxImageDisplayName string
		linuxVMIName          string

		selectorKey   string
		vmServiceName string
		vmNames       []string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		suffix := capiutil.RandomString(4)
		vmServiceName = fmt.Sprintf("%s-%s", specName, suffix)
		selectorKey = fmt.Sprintf("%s-selector", vmServiceName)
		vmNames = []string{fmt.Sprintf("%s-vm1", vmServiceName), fmt.Sprintf("%s-vm2", vmServiceName)}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmServiceName, virtualMachineServiceKind)
			for _, vmName := range vmNames {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, virtualMachineKind)
			}
		}

		Expect(ctrlclient.IgnoreNotFound(svClusterClient.Delete(ctx, &vmopv1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{Name: vmServiceName, Namespace: input.WCPNamespaceName},
		}))).To(Succeed(), "failed to delete VirtualMachineService")

		for _, vmName := range vmNames {
			Expect(ctrlclient.IgnoreNotFound(svClusterClient.Delete(ctx, &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: input.WCPNamespaceName},
			}))).To(Succeed(), "failed to delete VirtualMachine %q", vmName)
		}

		for _, vmName := range vmNames {
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should remove a VM from VirtualMachineService Endpoints once it no longer matches the selector", Label("core-functional", "experimental"), func() {
		By("Creating a VirtualMachineService selecting VMs with a unique label")
		vmService := &vmopv1.VirtualMachineService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmServiceName,
				Namespace: input.WCPNamespaceName,
			},
			Spec: vmopv1.VirtualMachineServiceSpec{
				Type: vmopv1.VirtualMachineServiceTypeClusterIP,
				Selector: map[string]string{
					selectorKey: "true",
				},
				Ports: []vmopv1.VirtualMachineServicePort{
					{
						Name:       "ssh",
						Protocol:   "TCP",
						Port:       22,
						TargetPort: 22,
					},
				},
			},
		}
		Expect(svClusterClient.Create(ctx, vmService)).To(Succeed(), "failed to create VirtualMachineService %q", vmServiceName)

		By("Creating 2 VMs labeled to match the VirtualMachineService selector")
		for _, vmName := range vmNames {
			vm := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmName,
					Namespace: input.WCPNamespaceName,
					Labels:    map[string]string{selectorKey: "true"},
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1.VirtualMachinePowerStateOn,
				},
			}
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %q", vmName)
		}

		By("Waiting for both VMs to be created, powered on, and have an IP assigned")
		for _, vmName := range vmNames {
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		}

		By("Waiting for the VirtualMachineService Endpoints to reference both VMs")
		vmoperator.WaitForVirtualMachineServiceEndpointsVMs(ctx, config, svClusterClient, input.WCPNamespaceName, vmServiceName, vmNames)

		By("Removing the selector label from the first VM")
		Eventually(func(g Gomega) {
			vm := &vmopv1.VirtualMachine{}
			err := svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmNames[0]}, vm)
			g.Expect(err).NotTo(HaveOccurred(), "failed to get VirtualMachine %q", vmNames[0])
			delete(vm.Labels, selectorKey)
			g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed(), "failed to update VirtualMachine %q", vmNames[0])
		}, config.GetIntervals("default", "wait-virtual-machine-annotation-update")...).Should(Succeed(),
			"Timed out removing selector label from VirtualMachine %q", vmNames[0])

		By("Verifying the first VM is promptly removed from the VirtualMachineService Endpoints, leaving only the second VM")
		vmoperator.WaitForVirtualMachineServiceEndpointsVMs(ctx, config, svClusterClient, input.WCPNamespaceName, vmServiceName, vmNames[1:])
	})
}
