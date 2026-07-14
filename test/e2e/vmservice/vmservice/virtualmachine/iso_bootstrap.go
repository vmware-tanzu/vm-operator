// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// ISOBootstrapSpecInput is the input for the auto-ISO bootstrap test spec.
type ISOBootstrapSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// ubuntuAutoinstallUserData is a minimal cloud-init autoinstall document
// sufficient to complete an unattended Ubuntu Server install. The E2E
// environment must publish an "ubuntu-24.04-live-server-amd64" ISO-type
// content library item -- see test/e2e/README.md.
const ubuntuAutoinstallUserData = `#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: ubuntu-auto-iso
    username: ubuntu
    password: "$6$exampleSaltValue$exampleHashValueDoNotUseInProduction"
  ssh:
    install-server: true
  late-commands:
    - curtin in-target --target=/target -- systemctl enable ssh
`

// ISOBootstrapSpec covers the Ubuntu (US2) end-to-end auto-ISO bootstrap
// flow: an unattended Ubuntu Server install driven entirely by
// spec.bootstrap.iso, with no VNC/console access and no pre-baked
// answer-file media.
func ISOBootstrapSpec(ctx context.Context, inputGetter func() ISOBootstrapSpecInput) {
	const specName = "vm-iso-bootstrap"

	var (
		input             ISOBootstrapSpecInput
		config            *e2eConfig.E2EConfig
		clusterProxy      *common.VMServiceClusterProxy
		svClusterClient   ctrlclient.Client
		clusterResources  *e2eConfig.Resources
		autoISOFSSEnabled bool
		vmName            string
		secretName        string
		vmYaml            []byte
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

		autoISOFSSEnabled = utils.IsFssEnabled(ctx,
			svClusterClient,
			config.GetVariable("VMOPNamespace"),
			config.GetVariable("VMOPDeploymentName"),
			config.GetVariable("VMOPManagerCommand"),
			config.GetVariable("EnvFSSAutoISO"),
		)

		vmYaml = nil
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		secretName = fmt.Sprintf("%s-bootstrap-data", vmName)
	})

	AfterEach(func() {
		if input.SkipCleanup {
			return
		}

		if len(vmYaml) > 0 {
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		}

		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: input.WCPNamespaceName}}
		Expect(ctrlclient.IgnoreNotFound(svClusterClient.Delete(ctx, secret))).To(Succeed(), "failed to delete bootstrap-assets Secret")
	})

	It("Should automatically install Ubuntu Server from an ISO with no VNC/console access", Label("experimental"), func() {
		if !autoISOFSSEnabled {
			Skip("Auto ISO FSS is not enabled")
		}

		By("Get the Ubuntu Server ISO-type image CR name")
		isoImageDisplayName := "ubuntu-24.04-live-server-amd64"
		isoImageName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, isoImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", input.WCPNamespaceName)

		By("Create the Secret containing the cloud-init autoinstall user-data and meta-data")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: input.WCPNamespaceName,
			},
			StringData: map[string]string{
				"user-data": ubuntuAutoinstallUserData,
				"meta-data": "",
			},
		}
		Expect(svClusterClient.Create(ctx, secret)).To(Succeed(), "failed to create bootstrap-assets Secret")

		By("Create a VM with spec.bootstrap.iso driving an unattended Ubuntu install")
		vmYaml = manifestbuilders.GetVirtualMachineYamlA6(manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			GuestID:          "ubuntu64Guest",
			PowerState:       "poweredOn",
			Hardware: &vmopv1a5.VirtualMachineHardwareSpec{
				Cdrom: []vmopv1a5.VirtualMachineCdromSpec{
					{
						Name: "cdrom1",
						Image: vmopv1a5.VirtualMachineImageRef{
							Name: isoImageName,
							Kind: "VirtualMachineImage",
						},
						Connected:         ptr.To(true),
						AllowGuestControl: ptr.To(true),
					},
				},
			},
			Bootstrap: manifestbuilders.Bootstrap{
				ISO: &manifestbuilders.ISO{
					// Matches spec.md's worked Ubuntu Autoinstall example:
					// bring up dracut-style static networking on the
					// GRUB2-loaded kernel, drop to the GRUB command line,
					// and boot the installer with autoinstall pointed at
					// the ephemeral bootstrap HTTP service.
					Commands: []string{
						"<esc><wait>",
						`ifname=bootnet:{{V1alpha6_FirstNicMacAddr}};ip={{(V1alpha6_FormatIP V1alpha6_FirstIP "")}}:{{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}:{{(V1alpha6_SubnetMask V1alpha6_FirstIP)}}:{{.V1alpha6.VM.Status.Network.Config.DNS.HostName}}:bootnet`,
						"<wait3s>c<wait3s>",
						`linux /casper/vmlinuz --- autoinstall ds="nocloud-net;seedfrom=http://{{V1Alpha6_BootstrapService}}/"`,
						"<enter><wait>",
						"initrd /casper/initrd",
						"<enter><wait>",
						"boot",
						"<enter>",
					},
					Assets: []manifestbuilders.ISOAsset{
						{Name: secretName, Key: "user-data"},
						{Name: secretName, Key: "meta-data"},
					},
				},
			},
		})
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM with spec.bootstrap.iso:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachineImageCacheReady(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

		By("Waiting for the unattended install to complete and the VM to report a primary IP")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Status.Network).NotTo(BeNil())
			g.Expect(vm.Status.Network.PrimaryIP4).NotTo(BeEmpty())
		}, config.GetIntervals("default", "wait-virtual-machine-iso-bootstrap")...).Should(Succeed(),
			"Timed out waiting for the Ubuntu auto-ISO install to complete")

		By("Verifying the VM's guest hostname matches the intended network config")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Status.Network).NotTo(BeNil())
			g.Expect(vm.Status.Network.HostName).NotTo(BeEmpty())
			if vm.Status.Network.Config != nil && vm.Status.Network.Config.DNS != nil {
				g.Expect(vm.Status.Network.HostName).To(Equal(vm.Status.Network.Config.DNS.HostName))
			}
		}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed())
	})
}
