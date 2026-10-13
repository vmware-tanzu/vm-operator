// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	vmopv1a2sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	vspherecfg "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

const (
	// The VM-level DNS inputs use documentation address ranges (RFC 5737 and
	// RFC 3849) so they cannot collide with the Supervisor's own resolvers.
	scopedDNSVMNameserverV4 = "192.0.2.53"
	scopedDNSVMNameserverV6 = "2001:db8::53"
	scopedDNSVMSearchDomain = "vm.example.test"

	// The interface-level DNS inputs use RFC 5737 and RFC 3849 ranges too.
	scopedDNSIfaceNameserverV4 = "198.51.100.53"
	scopedDNSIfaceNameserverV6 = "2001:db8::54"
	scopedDNSIfaceSearchDomain = "iface.example.test"
)

// scopedDNSIface is the DNS configuration VM Operator is expected to render
// for one interface.
type scopedDNSIface struct {
	dhcp        bool
	nameservers []string
	search      []string
}

// scopedDNSObserved is the addressing VM Operator reports for an interface. The
// cases derive their expected DNS from it, since the default network's address
// families depend on the provider and testbed.
type scopedDNSObserved struct {
	static       bool
	ipv4, ipv6   bool // Address families of the interface's addresses.
	gate4, gate6 bool // Address families of the interface's gateways.
}

// scopedDNSSuite creates the networks and VMs for the scoped DNS cases. The
// cases assert only on what VM Operator renders: the Cloud-Init metadata and
// the VM's network status. They never inspect the guest, so no lease or
// resolver values need to be known in advance.
//
// Static interfaces use the namespace's default network, so the cases run on
// every network provider. DHCP needs a Subnet whose DHCP behavior the test
// controls, so those cases run only on VPC.
type scopedDNSSuite struct {
	ctx          context.Context
	input        VMGOSCSpecInput
	clusterProxy *common.VMServiceClusterProxy
	vimClient    *vim25.Client
	template     *vmopv1a2.VirtualMachine
	vpc          bool

	// defaults and suffixes are the Supervisor's default nameservers and
	// search domains.
	defaults []string
	suffixes []string
}

func scopedDNSGuestSpecs(
	ctx context.Context,
	inputGetter func() VMGOSCSpecInput,
	templateGetter func() *vmopv1a2.VirtualMachine,
	vimGetter func() *vim25.Client) {

	Context("Guest DNS", func() {
		var s scopedDNSSuite

		BeforeEach(func() {
			input := inputGetter()

			s = scopedDNSSuite{
				vpc:          vmoperator.IsNetworkNsxtVPC(ctx, input.ClusterProxy.GetClient(), input.Config),
				ctx:          ctx,
				input:        input,
				clusterProxy: input.ClusterProxy.(*common.VMServiceClusterProxy),
				template:     templateGetter().DeepCopy(),
				vimClient:    vimGetter(),
			}

			cm := &corev1.ConfigMap{}
			Expect(s.clusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKey{
				Namespace: input.Config.GetVariable("VMOPNamespace"),
				Name:      vspherecfg.NetworkConfigMapName,
			}, cm)).To(Succeed())
			s.defaults = strings.Fields(cm.Data[vspherecfg.NameserversKey])
			s.suffixes = strings.Fields(cm.Data[vspherecfg.SearchSuffixesKey])
		})

		It("should keep legacy and automatically scoped DNS stable across reconciles", func() {
			s.requireDefaults()
			networks := s.staticNetworks(2)

			for _, mode := range []string{pkgconst.DNSDefaultsLegacy, pkgconst.DNSDefaultsScoped} {
				vm := s.newVM(mode, "cloud-init", networks)
				if mode == pkgconst.DNSDefaultsLegacy {
					vm.Annotations = map[string]string{pkgconst.DNSDefaultsAnnotationKey: mode}
				}
				s.create(vm)
				observed := s.requireLayout(vm, true, true)

				// Legacy repeats the unfiltered defaults on every static
				// interface. Scoped applies them to the first interface only,
				// filtered to the families of its gateways, along with the
				// Supervisor search suffixes (the VM is a VKS node).
				expected := []scopedDNSIface{{nameservers: s.defaults}, {nameservers: s.defaults}}
				if mode == pkgconst.DNSDefaultsScoped {
					expected = []scopedDNSIface{{
						nameservers: filterNameserversByFamily(s.defaults, observed[0].gate4, observed[0].gate6),
						search:      s.suffixes,
					}, {}}
				}
				s.checkCloudInit(vm, mode, expected)

				By(fmt.Sprintf("Triggering another reconcile of the %s VM without changing its bootstrap inputs", mode))
				original := s.get(vm)
				hash := original.Annotations[pkgconst.BootstrapHashConfigSpecAnnotationKey]
				Expect(hash).NotTo(BeEmpty())
				netplan, err := getCloudInitNetplan(s.ctx, s.vimClient, original.Status.UniqueID)
				Expect(err).NotTo(HaveOccurred())

				base := original.DeepCopy()
				metav1.SetMetaDataAnnotation(&original.ObjectMeta, "e2e.vmoperator.vmware.com/dns-reconcile", "1")
				Expect(s.clusterProxy.GetClient().Patch(s.ctx, original, ctrlclient.MergeFrom(base))).To(Succeed())

				Consistently(func(g Gomega) {
					current := s.get(vm)
					g.Expect(current.Annotations).To(HaveKeyWithValue(pkgconst.DNSDefaultsAnnotationKey, mode))
					g.Expect(current.Annotations).To(HaveKeyWithValue(pkgconst.BootstrapHashConfigSpecAnnotationKey, hash))
					latest, err := getCloudInitNetplan(s.ctx, s.vimClient, current.Status.UniqueID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(latest).To(Equal(netplan))
				}, "30s", "3s").Should(Succeed())
			}
		})

		It("should apply VM-level DNS to each static Cloud-Init interface, filtered by family", func() {
			vm := s.newVM("vmdns", "cloud-init", s.staticNetworks(2))
			vm.Spec.Network.Nameservers = []string{scopedDNSVMNameserverV4, scopedDNSVMNameserverV6}
			vm.Spec.Network.SearchDomains = []string{scopedDNSVMSearchDomain}
			s.create(vm)
			observed := s.requireLayout(vm, true, true)

			// A non-empty VM-level list also suppresses Supervisor defaults,
			// even when filtering leaves an interface with no nameservers.
			expected := make([]scopedDNSIface, len(observed))
			for i, o := range observed {
				expected[i] = scopedDNSIface{
					nameservers: filterNameserversByFamily(vm.Spec.Network.Nameservers, o.ipv4, o.ipv6),
					search:      vm.Spec.Network.SearchDomains,
				}
			}
			s.checkCloudInit(vm, pkgconst.DNSDefaultsScoped, expected)
		})

		It("should prefer interface-level DNS over VM-level DNS in Cloud-Init", func() {
			vm := s.newVM("ifacedns", "cloud-init", s.staticNetworks(2))
			vm.Spec.Network.Nameservers = []string{scopedDNSVMNameserverV4, scopedDNSVMNameserverV6}
			vm.Spec.Network.SearchDomains = []string{scopedDNSVMSearchDomain}
			vm.Spec.Network.Interfaces[0].Nameservers = []string{scopedDNSIfaceNameserverV4, scopedDNSIfaceNameserverV6}
			vm.Spec.Network.Interfaces[0].SearchDomains = []string{scopedDNSIfaceSearchDomain}
			s.create(vm)
			observed := s.requireLayout(vm, true, true)

			// Explicit interface DNS is never filtered, and nameservers and
			// search domains take precedence independently. The second
			// interface falls back to the VM-level DNS.
			s.checkCloudInit(vm, pkgconst.DNSDefaultsScoped, []scopedDNSIface{{
				nameservers: vm.Spec.Network.Interfaces[0].Nameservers,
				search:      vm.Spec.Network.Interfaces[0].SearchDomains,
			}, {
				nameservers: filterNameserversByFamily(vm.Spec.Network.Nameservers, observed[1].ipv4, observed[1].ipv6),
				search:      vm.Spec.Network.SearchDomains,
			}})
		})

		It("should not apply VM-level DNS to a DHCP Cloud-Init interface", func() {
			s.requireVPC()
			vm := s.newVM("dhcp", "cloud-init", append(s.staticNetworks(1), s.dhcpNetwork()))
			vm.Spec.Network.Nameservers = []string{scopedDNSVMNameserverV4, scopedDNSVMNameserverV6}
			vm.Spec.Network.SearchDomains = []string{scopedDNSVMSearchDomain}
			s.create(vm)
			observed := s.requireLayout(vm, true, false)

			// The DHCP interface gets nothing from VM Operator, and neither
			// interface gets Supervisor defaults.
			s.checkCloudInit(vm, pkgconst.DNSDefaultsScoped, []scopedDNSIface{{
				nameservers: filterNameserversByFamily(vm.Spec.Network.Nameservers, observed[0].ipv4, observed[0].ipv6),
				search:      vm.Spec.Network.SearchDomains,
			}, {dhcp: true}})
		})

		DescribeTable("LinuxPrep global DNS",
			func(engine string, vmLevelDNS, dhcpFirst bool) {
				var networks []*vmopv1a2common.PartialObjectRef
				if dhcpFirst {
					s.requireVPC()
					networks = append([]*vmopv1a2common.PartialObjectRef{s.dhcpNetwork()}, s.staticNetworks(1)...)
				} else {
					networks = s.staticNetworks(2)
				}
				if !vmLevelDNS {
					s.requireDefaults()
				}

				vm := s.newVM(engine, engine, networks)
				if vmLevelDNS {
					vm.Spec.Network.Nameservers = []string{scopedDNSVMNameserverV4, scopedDNSVMNameserverV6}
				}
				s.create(vm)
				observed := s.requireLayout(vm, !dhcpFirst, true)

				// GOSC applies a non-empty global list over DHCP, so VM
				// Operator supplies Supervisor defaults only when the first
				// interface is static, filtered to its gateway families.
				// VM-level DNS is applied globally, unfiltered.
				var expected []string
				switch {
				case vmLevelDNS:
					expected = vm.Spec.Network.Nameservers
				case !dhcpFirst:
					expected = filterNameserversByFamily(s.defaults, observed[0].gate4, observed[0].gate6)
				}
				Eventually(func(g Gomega) {
					current := s.get(vm)
					g.Expect(current.Annotations).To(HaveKeyWithValue(
						pkgconst.DNSDefaultsAnnotationKey, pkgconst.DNSDefaultsScoped))
					g.Expect(current.Status.Network).NotTo(BeNil())
					g.Expect(current.Status.Network.Config).NotTo(BeNil())
					var nameservers, search []string
					if dns := current.Status.Network.Config.DNS; dns != nil {
						nameservers, search = dns.Nameservers, dns.SearchDomains
					}
					g.Expect(nameservers).To(equalOrEmpty(expected))
					g.Expect(search).To(BeEmpty(), "LinuxPrep never receives Supervisor search domains")
				}, s.input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
			},
			Entry("implicit LinuxPrep with a static first interface gets the defaults", "implicit", false, false),
			Entry("explicit LinuxPrep with VM-level DNS gets only the VM-level DNS", "linux-prep", true, false),
			Entry("explicit LinuxPrep with a DHCP first interface gets no defaults", "linux-prep", false, true),
		)
		// Windows ignores the global GOSC nameserver list, so Sysprep applies DNS
		// per adapter. The VM is created here instead of during suite setup, as
		// the Sysprep specs do, and the test does not wait for the guest to boot
		// or finish customization: the DNS VM Operator applies is reported in
		// the VM's status once the vSphere VM exists. It deliberately omits
		// consts.WindowsSysprepLabel, which makes the suite precreate Windows VMs.
		It("should apply DNS per adapter with Sysprep", Label("windows"), func() {
			networks := s.staticNetworks(2)
			vm := s.sysprepVM(networks)
			vm.Spec.Network.Nameservers = []string{scopedDNSVMNameserverV4, scopedDNSVMNameserverV6}
			vm.Spec.Network.SearchDomains = []string{scopedDNSVMSearchDomain}
			vm.Spec.Network.Interfaces[1].Nameservers = []string{scopedDNSIfaceNameserverV4, scopedDNSIfaceNameserverV6}

			vmoperator.CreateVirtualMachine(s.ctx, s.input.Config, s.clusterProxy, vm, false)
			client := s.clusterProxy.GetClient()
			vmoperator.WaitForVirtualMachineToExist(s.ctx, s.input.Config, client, vm.Namespace, vm.Name)
			vmoperator.WaitForVirtualMachineImageCacheReady(s.ctx, s.input.Config, client, vm.Namespace, vm.Name, "windows-sysprep")
			vmoperator.WaitForVirtualMachineConditionCreated(s.ctx, s.input.Config, client, vm.Namespace, vm.Name)
			observed := s.requireLayout(vm, true, true)

			// The first adapter takes the VM-level nameservers filtered to its
			// address families, the second keeps its own, unfiltered. The
			// global nameserver list stays empty, and the only search domains
			// are the global VM-level ones.
			expected := [][]string{
				filterNameserversByFamily(vm.Spec.Network.Nameservers, observed[0].ipv4, observed[0].ipv6),
				vm.Spec.Network.Interfaces[1].Nameservers,
			}
			Eventually(func(g Gomega) {
				current := s.get(vm)
				g.Expect(current.Annotations).To(HaveKeyWithValue(
					pkgconst.DNSDefaultsAnnotationKey, pkgconst.DNSDefaultsScoped))
				g.Expect(current.Status.Network).NotTo(BeNil())
				nc := current.Status.Network.Config
				g.Expect(nc).NotTo(BeNil())
				g.Expect(nc.DNS).NotTo(BeNil())
				g.Expect(nc.DNS.Nameservers).To(BeEmpty())
				g.Expect(nc.DNS.SearchDomains).To(Equal(vm.Spec.Network.SearchDomains))
				g.Expect(nc.Interfaces).To(HaveLen(len(expected)))
				for i, iface := range nc.Interfaces {
					var nameservers, search []string
					if iface.DNS != nil {
						nameservers, search = iface.DNS.Nameservers, iface.DNS.SearchDomains
					}
					g.Expect(nameservers).To(equalOrEmpty(expected[i]), "nameservers for %s", iface.Name)
					g.Expect(search).To(BeEmpty(), "Sysprep has no per-adapter search domains: %s", iface.Name)
				}
			}, s.input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
		})
	})
}

// requireDefaults skips the test unless the Supervisor's network ConfigMap has
// a default IPv4 nameserver, since the expectations are computed from it.
func (s *scopedDNSSuite) requireDefaults() {
	GinkgoHelper()
	if len(filterNameserversByFamily(s.defaults, true, false)) == 0 {
		Skip("the Supervisor network ConfigMap has no default IPv4 nameservers")
	}
}

// sysprepVM returns a Windows VM with inline Sysprep and one interface per
// network reference. It skips the test when the testbed has no Windows image.
// The VM name is kept under 15 characters since Windows computer names are
// limited to that length.
func (s *scopedDNSSuite) sysprepVM(networks []*vmopv1a2common.PartialObjectRef) *vmopv1a2.VirtualMachine {
	GinkgoHelper()

	displayName := s.input.Config.InfraConfig.ManagementClusterConfig.Resources.WindowsImageDisplayName
	if displayName == "" {
		Skip("no Windows image is configured")
	}
	client := s.clusterProxy.GetClient()
	imageName := vmoperator.WaitForVirtualMachineImageName(s.ctx, &s.input.Config.Config,
		client, s.input.WCPNamespaceName, displayName)
	By("Waiting for the Windows image content to be downloaded")
	vmoperator.WaitForOVFVirtualMachineImageReady(s.ctx, &s.input.Config.Config,
		client, s.input.WCPNamespaceName, imageName)

	secretName := fmt.Sprintf("scoped-dns-sysprep-%s", capiutil.RandomString(4))
	Expect(client.Create(s.ctx, manifestbuilders.InlineSysprepSecret(manifestbuilders.Secret{
		Namespace: s.input.WCPNamespaceName,
		Name:      secretName,
	}))).To(Succeed())

	vm := s.newVM("win", "implicit", networks)
	vm.Name = fmt.Sprintf("dnswin-%s", capiutil.RandomString(4))
	vm.Spec.ImageName = imageName
	vm.Spec.PowerOffMode = vmopv1a2.VirtualMachinePowerOpModeHard
	vm.Spec.Network.HostName = vm.Name
	vm.Spec.Bootstrap = &vmopv1a2.VirtualMachineBootstrapSpec{
		Sysprep: &vmopv1a2.VirtualMachineBootstrapSysprepSpec{
			Sysprep: &vmopv1a2sysprep.Sysprep{
				GUIUnattended: &vmopv1a2sysprep.GUIUnattended{
					Password: &vmopv1a2sysprep.PasswordSecretKeySelector{
						Name: secretName,
						Key:  "vmsvc-pwd",
					},
					TimeZone: 4,
				},
				Identification: &vmopv1a2sysprep.Identification{JoinWorkgroup: "vmware"},
				UserData:       &vmopv1a2sysprep.UserData{FullName: "E2E", OrgName: "Broadcom"},
			},
		},
	}
	return vm
}

// requireVPC skips the test unless the Supervisor uses VPC networking, the only
// provider where the test controls a Subnet's DHCP configuration.
func (s *scopedDNSSuite) requireVPC() {
	GinkgoHelper()
	if !s.vpc {
		Skip("requires VPC networking to create a DHCP Subnet")
	}
}

// staticNetworks returns count network references for static interfaces. On
// VPC each is a new CIDR Subnet. Otherwise each is nil, which selects the
// namespace's default network; its interfaces get static addresses from IPAM.
func (s *scopedDNSSuite) staticNetworks(count int) []*vmopv1a2common.PartialObjectRef {
	networks := make([]*vmopv1a2common.PartialObjectRef, count)
	if s.vpc {
		for i := range networks {
			networks[i] = s.createSubnet(utils.CIDRConfig)
		}
	}
	return networks
}

// dhcpNetwork returns a new DHCP Subnet. It must only be called on VPC.
func (s *scopedDNSSuite) dhcpNetwork() *vmopv1a2common.PartialObjectRef {
	return s.createSubnet(utils.DHCPConfig)
}

func (s *scopedDNSSuite) createSubnet(networkConfigType string) *vmopv1a2common.PartialObjectRef {
	name := fmt.Sprintf("scoped-dns-%s", capiutil.RandomString(6))
	ns := s.input.WCPNamespaceName
	client := s.clusterProxy.GetClient()

	createSubnetOrSubnetSet(s.ctx, s.input.Config, s.clusterProxy,
		utils.SubnetKind, name, ns, networkConfigType, true)
	DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait,
		s.ctx, s.input.Config, client, ns, name, utils.SubnetKind)
	vmservice.VerifySubnetOrSubnetSetCreation(s.ctx, s.input.Config, client, ns, name, utils.SubnetKind)

	return &vmopv1a2common.PartialObjectRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: vpcv1alpha1.GroupVersion.String(),
			Kind:       utils.SubnetKind,
		},
		Name: name,
	}
}

// newVM returns a VKS worker VM with one interface per network reference. A nil
// reference selects the default network.
func (s *scopedDNSSuite) newVM(suffix, engine string, networks []*vmopv1a2common.PartialObjectRef) *vmopv1a2.VirtualMachine {
	vm := s.template.DeepCopy()
	vm.Name = fmt.Sprintf("%s-%s", vm.Name, suffix)
	vm.Annotations = nil
	vm.Labels = map[string]string{kubeutil.CAPVClusterRoleLabelKey: "worker"}

	vm.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{}
	for i, network := range networks {
		vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces,
			vmopv1a2.VirtualMachineNetworkInterfaceSpec{
				Name:    fmt.Sprintf("eth%d", i),
				Network: network,
			})
	}

	switch engine {
	case "cloud-init":
		vm.Spec.Bootstrap = &vmopv1a2.VirtualMachineBootstrapSpec{
			CloudInit: &vmopv1a2.VirtualMachineBootstrapCloudInitSpec{},
		}
	case "linux-prep":
		vm.Spec.Bootstrap = &vmopv1a2.VirtualMachineBootstrapSpec{
			LinuxPrep: &vmopv1a2.VirtualMachineBootstrapLinuxPrepSpec{},
		}
	case "implicit":
		vm.Spec.Bootstrap = nil
	}
	return vm
}

func (s *scopedDNSSuite) create(vm *vmopv1a2.VirtualMachine) {
	vmoperator.CreateVirtualMachine(s.ctx, s.input.Config, s.clusterProxy, vm, false)
	vmoperator.WaitForVirtualMachineCreation(s.ctx, s.input.Config,
		s.clusterProxy.GetClient(), vm.Namespace, vm.Name)
}

func (s *scopedDNSSuite) get(vm *vmopv1a2.VirtualMachine) *vmopv1.VirtualMachine {
	GinkgoHelper()
	current, err := utils.GetVirtualMachine(s.ctx, s.clusterProxy.GetClient(), vm.Namespace, vm.Name)
	Expect(err).NotTo(HaveOccurred())
	return current
}

// requireLayout waits for the VM's network status and returns what it reports
// for each interface. The test is skipped, instead of failing, when the
// network does not provide the layout the case needs: first and others say
// whether the first interface, and every other interface, must be static
// (otherwise DHCP). A static first interface must also have a gateway.
func (s *scopedDNSSuite) requireLayout(vm *vmopv1a2.VirtualMachine, first, others bool) []scopedDNSObserved {
	GinkgoHelper()

	var observed []scopedDNSObserved
	Eventually(func(g Gomega) {
		current := s.get(vm)
		g.Expect(current.Status.Network).NotTo(BeNil())
		g.Expect(current.Status.Network.Config).NotTo(BeNil())
		interfaces := current.Status.Network.Config.Interfaces
		g.Expect(interfaces).To(HaveLen(len(vm.Spec.Network.Interfaces)))

		observed = nil
		for _, iface := range interfaces {
			g.Expect(iface.IP).NotTo(BeNil())
			o := scopedDNSObserved{
				static: iface.IP.DHCP == nil && len(iface.IP.Addresses) > 0,
				gate4:  iface.IP.Gateway4 != "",
				gate6:  iface.IP.Gateway6 != "",
			}
			for _, address := range iface.IP.Addresses {
				ip, _, err := net.ParseCIDR(address)
				g.Expect(err).NotTo(HaveOccurred())
				if ip.To4() != nil {
					o.ipv4 = true
				} else {
					o.ipv6 = true
				}
			}
			observed = append(observed, o)
		}
	}, s.input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

	for i, o := range observed {
		want := others
		if i == 0 {
			want = first
		}
		if o.static != want {
			Skip(fmt.Sprintf("interface %d is not %s on this network", i, map[bool]string{true: "static", false: "DHCP"}[want]))
		}
	}
	if first && !observed[0].gate4 && !observed[0].gate6 {
		Skip("the first interface has no gateway on this network")
	}
	return observed
}

// checkCloudInit waits for the VM to select mode, and for the rendered
// Cloud-Init metadata to match expected. A scoped VM's interface status must
// report the same nameservers and search domains.
func (s *scopedDNSSuite) checkCloudInit(vm *vmopv1a2.VirtualMachine, mode string, expected []scopedDNSIface) {
	GinkgoHelper()

	By(fmt.Sprintf("Verifying the %s VM's Cloud-Init metadata", mode))
	Eventually(func(g Gomega) {
		current := s.get(vm)
		g.Expect(current.Annotations).To(HaveKeyWithValue(pkgconst.DNSDefaultsAnnotationKey, mode))
		g.Expect(current.Status.UniqueID).NotTo(BeEmpty())

		netplan, err := getCloudInitNetplan(s.ctx, s.vimClient, current.Status.UniqueID)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(netplan.Network.Ethernets).To(HaveLen(len(expected)))

		for i, want := range expected {
			name := fmt.Sprintf("eth%d", i)
			eth, ok := netplan.Network.Ethernets[name]
			g.Expect(ok).To(BeTrue(), "no netplan ethernet for %s", name)
			g.Expect(eth.DHCP4).To(Equal(want.dhcp), "dhcp4 for %s", name)
			g.Expect(eth.Nameservers.Addresses).To(equalOrEmpty(want.nameservers), "nameservers for %s", name)
			// Search domains are only asserted for scoped VMs; legacy
			// output is covered by unit tests.
			if mode == pkgconst.DNSDefaultsScoped {
				g.Expect(eth.Nameservers.Search).To(equalOrEmpty(want.search), "search domains for %s", name)
			}
		}

		if mode != pkgconst.DNSDefaultsScoped {
			return
		}
		g.Expect(current.Status.Network).NotTo(BeNil())
		g.Expect(current.Status.Network.Config).NotTo(BeNil())
		// Cloud-Init has no global DNS.
		if dns := current.Status.Network.Config.DNS; dns != nil {
			g.Expect(dns.Nameservers).To(BeEmpty())
			g.Expect(dns.SearchDomains).To(BeEmpty())
		}
		g.Expect(current.Status.Network.Config.Interfaces).To(HaveLen(len(expected)))
		for i, iface := range current.Status.Network.Config.Interfaces {
			var nameservers, search []string
			if iface.DNS != nil {
				nameservers, search = iface.DNS.Nameservers, iface.DNS.SearchDomains
			}
			g.Expect(nameservers).To(equalOrEmpty(expected[i].nameservers), "status nameservers for %s", iface.Name)
			g.Expect(search).To(equalOrEmpty(expected[i].search), "status search domains for %s", iface.Name)
		}
	}, s.input.Config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
}
