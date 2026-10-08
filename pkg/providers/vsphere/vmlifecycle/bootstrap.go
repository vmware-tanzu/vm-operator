// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/cespare/xxhash/v2"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/linuxprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/util/sysprep"
)

const (
	// OvfEnvironmentTransportGuestInfo is the OVF transport type that uses
	// GuestInfo. The other valid type is "iso".
	OvfEnvironmentTransportGuestInfo = "com.vmware.guestInfo"

	// GOSCVCFAHashID is the VCFA ID key name in the GOSC customization extra config.
	GOSCVCFAHashID = "vcfaVmHashKey"

	redacted = "***"
)

type BootstrapData struct {
	Data       map[string]string
	VAppData   map[string]string
	VAppExData map[string]map[string]string

	CloudConfig *cloudinit.CloudConfigSecretData
	Sysprep     *sysprep.SecretData
	LinuxPrep   *linuxprep.SecretData
}

type TemplateRenderFunc func(string, string) string

type BootstrapArgs struct {
	BootstrapData

	TemplateRenderFn TemplateRenderFunc
	NetBootstraps    []network.Bootstrap
	UpdatedEthCards  bool
	DomainName       string
	HostName         string
	VLANs            []vmopv1.VirtualMachineNetworkVLANSpec

	// DNSServers and SearchSuffixes are the global DNS configuration. They
	// are applied to the global IP settings of a LinuxPrep or Sysprep
	// customization spec, and reported in the VM's status.
	DNSServers     []string
	SearchSuffixes []string

	// TemplateDNSServers are the resolved nameservers available to bootstrap
	// templates: the VM-level nameservers, falling back to the Supervisor's
	// default nameservers.
	TemplateDNSServers []string
}

var (
	ErrBootstrapReconfigure = pkgerr.NoRequeueNoErr("bootstrap reconfigured vm")
	ErrBootstrapCustomize   = pkgerr.NoRequeueNoErr("bootstrap customized vm")
)

func DoBootstrap( //nolint:gocyclo
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	bootstrapArgs BootstrapArgs) error {

	vmCtx.Logger.V(4).Info("Reconciling bootstrap state")

	bootstrap := getEffectiveBootstrapSpec(vmCtx.VM, config)
	if bootstrap == nil {
		vmCtx.Logger.V(6).Info("no bootstrap provider specified")
		return nil
	}

	if bootstrap.Disabled {
		vmCtx.Logger.V(4).Info("Skipping bootstrap since disabled")
		return nil
	}

	var (
		cloudInit  = bootstrap.CloudInit
		linuxPrep  = bootstrap.LinuxPrep
		sysPrep    = bootstrap.Sysprep
		vAppConfig = bootstrap.VAppConfig
	)

	if sysPrep != nil || vAppConfig != nil {
		bootstrapArgs.TemplateRenderFn = GetTemplateRenderFunc(vmCtx, &bootstrapArgs)
	}

	var (
		configSpec     *vimtypes.VirtualMachineConfigSpec
		customSpec     *vimtypes.CustomizationSpec
		customizeLatch *bool
		err            error
	)

	switch {
	case cloudInit != nil:
		configSpec, customSpec, err = BootStrapCloudInit(vmCtx, config, cloudInit, &bootstrapArgs)
	case linuxPrep != nil:
		configSpec, customSpec, customizeLatch, err = BootStrapLinuxPrep(
			vmCtx, config, linuxPrep, vAppConfig, &bootstrapArgs)
	case sysPrep != nil:
		configSpec, customSpec, customizeLatch, err = BootstrapSysPrep(
			vmCtx, config, sysPrep, vAppConfig, &bootstrapArgs)
	case vAppConfig != nil:
		configSpec, customSpec, err = BootstrapVAppConfig(vmCtx, config, vAppConfig, &bootstrapArgs)
	}

	if err != nil {
		return fmt.Errorf("failed to create bootstrap data: %w", err)
	}

	var retErr error

	if configSpec != nil {
		const hashKey = pkgconst.BootstrapHashConfigSpecAnnotationKey

		newHash, err := getVimTypeHash(configSpec)
		if err != nil {
			return err
		}

		var reconfigureNeeded bool
		if customizeLatch != nil {
			reconfigureNeeded = *customizeLatch
		} else {
			reconfigureNeeded = newHash != vmCtx.VM.Annotations[hashKey]
			if !reconfigureNeeded {
				vmCtx.Logger.V(4).Info(
					"Skipping bootstrap reconfigure as nothing has changed")
			}
		}

		if reconfigureNeeded {
			vmCtx.Logger.V(4).Info("Doing bootstrap reconfigure")
			if err := doReconfigure(vmCtx, vcVM, configSpec); err != nil {
				return fmt.Errorf("bootstrap reconfigure failed: %w", err)
			}

			if newHash != vmCtx.VM.Annotations[hashKey] {
				vmCtx.Logger.V(4).Info(
					"Updating bootstrap reconfigure hash", hashKey, newHash)
				if vmCtx.VM.Annotations == nil {
					vmCtx.VM.Annotations = map[string]string{}
				}
				vmCtx.VM.Annotations[hashKey] = newHash
			}

			retErr = errors.Join(retErr, ErrBootstrapReconfigure)
		}
	}

	if customSpec != nil {
		const hashKey = pkgconst.BootstrapHashCustomSpecAnnotationKey

		newHash, err := getVimTypeHash(customSpec)
		if err != nil {
			return err
		}

		var customizeNeeded bool
		if customizeLatch != nil {
			customizeNeeded = *customizeLatch
		} else {
			customizeNeeded = newHash != vmCtx.VM.Annotations[hashKey]
			if !customizeNeeded {
				vmCtx.Logger.V(4).Info(
					"Skipping bootstrap customize as nothing has changed")
			}
		}

		if customizeNeeded {
			vmCtx.Logger.V(0).Info("Doing bootstrap customize")
			if err := doCustomize(vmCtx, vcVM, config, customSpec); err != nil {
				// Mark the customization condition as failed with the error message.
				// This handles immediate failures (e.g., unsupported tools version) that
				// occur at the vSphere API level before the guest customization starts.
				conditions.MarkError(vmCtx.VM, vmopv1.GuestCustomizationCondition,
					vmopv1.GuestCustomizationFailedReason, err)
				return fmt.Errorf("bootstrap customize failed: %w", err)
			}

			if newHash != vmCtx.VM.Annotations[hashKey] {
				vmCtx.Logger.V(4).Info(
					"Updating bootstrap customize hash", hashKey, newHash)
				if vmCtx.VM.Annotations == nil {
					vmCtx.VM.Annotations = map[string]string{}
				}
				vmCtx.VM.Annotations[hashKey] = newHash
			}

			retErr = errors.Join(retErr, ErrBootstrapCustomize)
		}
	}

	if customizeLatch != nil && *customizeLatch {
		// Set latch to false so that a later customization on has to be
		// explicitly requested.
		vmCtx.Logger.V(4).Info(
			"Setting customization latch to false after successful customization")
		*customizeLatch = false
	}

	return retErr
}

// getEffectiveBootstrapSpec returns the bootstrap spec used to bootstrap the
// VM, or nil if the VM is not bootstrapped.
func getEffectiveBootstrapSpec(
	vm *vmopv1.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo) *vmopv1.VirtualMachineBootstrapSpec {

	if bootstrap := vm.Spec.Bootstrap; bootstrap != nil {
		return bootstrap
	}

	var cdRomSpecs []vmopv1.VirtualMachineCdromSpec
	if hw := vm.Spec.Hardware; hw != nil {
		cdRomSpecs = hw.Cdrom
	}

	// V1ALPHA1: We had always defaulted to LinuxPrep w/ HwClockUTC=true.
	// Now, try to just do that on Linux VMs.
	// Skip if the VM has a CD-ROM as the Linux ISO-type image may not have
	// the necessary tools to do the default LinuxPrep bootstrap.
	if len(cdRomSpecs) > 0 || config == nil ||
		vimtypes.GuestIDToFamily(config.GuestId) != vimtypes.VirtualMachineGuestOsFamilyLinuxGuest {
		return nil
	}

	return &vmopv1.VirtualMachineBootstrapSpec{
		LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
			HardwareClockIsUTC: new(true),
		},
	}
}

// GetBootstrapArgs returns the information used to bootstrap the VM via
// one of the many, possible bootstrap engines.
func GetBootstrapArgs(
	ctx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	bootstraps []network.Bootstrap,
	updatedEthCards bool,
	bootstrapData BootstrapData) (BootstrapArgs, error) {

	bsa := BootstrapArgs{
		BootstrapData:   bootstrapData,
		NetBootstraps:   bootstraps,
		UpdatedEthCards: updatedEthCards,
		HostName:        ctx.VM.Name,
	}

	if networkSpec := ctx.VM.Spec.Network; networkSpec != nil {
		if networkSpec.HostName != "" {
			bsa.HostName = networkSpec.HostName
		}
		if networkSpec.DomainName != "" {
			bsa.DomainName = networkSpec.DomainName
		}
		bsa.DNSServers = networkSpec.Nameservers
		bsa.SearchSuffixes = networkSpec.SearchDomains
		bsa.VLANs = networkSpec.VLANs
	}

	var err error
	if useScopedDNSDefaults(ctx) {
		err = applyScopedDNSDefaults(ctx, k8sClient, &bsa)
	} else {
		err = applyLegacyDNSDefaults(ctx, k8sClient, &bsa)
	}
	if err != nil {
		return BootstrapArgs{}, err
	}

	return bsa, nil
}

// existingGuestAnnotationKeys are the annotations that indicate the VM's guest
// may have already been bootstrapped or booted.
var existingGuestAnnotationKeys = []string{
	pkgconst.BootstrapHashConfigSpecAnnotationKey,
	pkgconst.BootstrapHashCustomSpecAnnotationKey,
	vmopv1.FirstBootDoneAnnotation,
	vmopv1.RestoredVMAnnotation,
	vmopv1.ImportedVMAnnotation,
	vmopv1.FailedOverVMAnnotation,
}

// useScopedDNSDefaults returns true if the Supervisor's default DNS
// configuration should be applied to the VM with the scoped behavior. The
// legacy behavior is always used when the ScopedDNSDefaults capability is not
// activated. Otherwise, the mode is selected by the VM's
// DNSDefaultsAnnotationKey annotation. When the VM does not have the
// annotation, it is set so that a VM whose guest may have already been
// bootstrapped keeps the legacy behavior.
func useScopedDNSDefaults(ctx pkgctx.VirtualMachineContext) bool {
	if !pkgcfg.FromContext(ctx).Features.ScopedDNSDefaults {
		return false
	}

	vm := ctx.VM

	mode := vm.Annotations[pkgconst.DNSDefaultsAnnotationKey]
	if mode == "" {
		mode = pkgconst.DNSDefaultsScoped
		for _, k := range existingGuestAnnotationKeys {
			if _, ok := vm.Annotations[k]; ok {
				mode = pkgconst.DNSDefaultsLegacy
				break
			}
		}

		if vm.Annotations == nil {
			vm.Annotations = map[string]string{}
		}
		vm.Annotations[pkgconst.DNSDefaultsAnnotationKey] = mode
	}

	return mode == pkgconst.DNSDefaultsScoped
}

// applyScopedDNSDefaults applies the VM-level DNS configuration only to the
// VM's static interfaces, and the Supervisor's default DNS configuration only
// to the VM's primary interface, as determined by network.PrimaryInterface,
// and only when the VM's spec does not provide DNS configuration for it.
// Nameservers are only applied to an interface when they are of its IP
// families. The Supervisor's default nameservers, which are often resolvers
// on other networks, are only applied for the IP families that the primary
// interface has a gateway for.
//
// An interface's DNS configuration from the network provider, such as a VPC
// SubnetPort, is treated as the interface's own DNS configuration unless the
// interface spec specifies its own.
//
//   - CloudInit: each interface's own DNS configuration is applied to it. The
//     VM-level DNS configuration is applied to each static interface that
//     does not have its own, when the matching UseGlobal*AsDefault is unset
//     or true. The primary interface gets the Supervisor's default
//     nameservers when there are no VM-level nameservers and it does not have
//     its own. Search domains are handled the same, but only for TKG VMs.
//   - LinuxPrep: DNS configuration is only global, and GOSC treats the global
//     DNS servers as an override of the DNS servers from DHCP. The VM-level
//     DNS configuration is applied globally, followed by each interface's in
//     interface order, without duplicates. Otherwise, the Supervisor's
//     default nameservers are applied globally when the VM has a primary
//     interface. The Supervisor's default search domains are not applied.
//   - Sysprep: Windows does not use the global DNS servers, so nameservers
//     are applied per-adapter and the global DNS servers are not set. Each
//     adapter's own nameservers are applied to it. The VM-level nameservers
//     are applied to each static adapter that does not have its own, since
//     the per-adapter list overrides the DNS servers from DHCP. The primary
//     adapter gets the Supervisor's default nameservers when there are no
//     VM-level nameservers and it does not have its own. Search suffixes are
//     global: the VM-level search domains followed by each adapter's in
//     adapter order, without duplicates. The Supervisor's default search
//     domains are not applied.
//
// The nameservers made available to templates are the global nameservers
// above, or else the Supervisor's defaults. The global DNS configuration is
// what the bootstrap engine applies globally, which is none for CloudInit, or
// the resolved configuration when no bootstrap engine configures the guest's
// network.
func applyScopedDNSDefaults( //nolint:gocyclo
	ctx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	bsa *BootstrapArgs) error {

	var (
		isCloudInit, isLinuxPrep, isSysprep bool
		applyVMNS, applyVMSS                bool
	)

	// The validation webhook only allows one of CloudInit, LinuxPrep, or
	// Sysprep.
	if bs := getEffectiveBootstrapSpec(ctx.VM, ctx.MoVM.Config); bs != nil && !bs.Disabled {
		isCloudInit = bs.CloudInit != nil
		isLinuxPrep = bs.LinuxPrep != nil
		isSysprep = bs.Sysprep != nil

		switch {
		case isCloudInit:
			applyVMNS = ptr.DerefWithDefault(bs.CloudInit.UseGlobalNameserversAsDefault, true)
			applyVMSS = ptr.DerefWithDefault(bs.CloudInit.UseGlobalSearchDomainsAsDefault, true)
		case isSysprep:
			// Windows configures DNS servers per adapter, while its search
			// suffixes are global.
			applyVMNS = true
		}
	}
	configuresGuestNetwork := isCloudInit || isLinuxPrep || isSysprep

	vmNS, vmSS := bsa.DNSServers, bsa.SearchSuffixes
	bootstraps := bsa.NetBootstraps

	// The network provider's DNS configuration for an interface is treated
	// as the interface's own. InterfaceBootstrap has already cleared it for
	// an interface that specifies its own.
	for i := range bootstraps {
		b := &bootstraps[i]
		if len(b.ProviderNameservers) > 0 {
			b.Nameservers = b.ProviderNameservers
		}
		if len(b.ProviderSearchDomains) > 0 {
			b.SearchDomains = b.ProviderSearchDomains
		}
	}

	// globalNS and globalSS are the VM-level DNS configuration, followed by
	// the interfaces' DNS configuration that the bootstrap engine only
	// supports globally, in interface order and without duplicates.
	globalNS, globalSS := vmNS, vmSS
	switch {
	case isLinuxPrep:
		globalNS, globalSS = appendUnique(nil, vmNS...), appendUnique(nil, vmSS...)
		for i := range bootstraps {
			b := &bootstraps[i]
			globalNS = appendUnique(globalNS, b.Nameservers...)
			globalSS = appendUnique(globalSS, b.SearchDomains...)
			b.Nameservers, b.SearchDomains = nil, nil
		}
	case isSysprep:
		globalSS = appendUnique(nil, vmSS...)
		for i := range bootstraps {
			b := &bootstraps[i]
			globalSS = appendUnique(globalSS, b.SearchDomains...)
			b.SearchDomains = nil
		}
	}

	// Apply the VM-level DNS configuration to each static interface that
	// does not have its own. DNS configuration for a DHCP or NoIPAM
	// interface must be specified on the interface.
	for i := range bootstraps {
		b := &bootstraps[i]
		if !b.IsStatic() {
			continue
		}
		if applyVMNS && len(vmNS) > 0 && len(b.Nameservers) == 0 {
			ipv4, ipv6 := b.AddressFamilies()
			b.Nameservers = network.FilterNameserversByFamily(vmNS, ipv4, ipv6)
		}
		if applyVMSS && len(b.SearchDomains) == 0 {
			b.SearchDomains = vmSS
		}
	}

	missingSearches := slices.ContainsFunc(bootstraps, func(b network.Bootstrap) bool {
		return b.IsStatic() && len(b.SearchDomains) == 0
	})

	primary := network.PrimaryInterface(bootstraps)

	// Determine where the Supervisor's defaults are applied. As before the
	// ScopedDNSDefaults capability, the Supervisor's default search domains
	// are only applied to TKG VMs, which use CloudInit, and never by GOSC.
	var (
		// Nameservers and search domains for the primary interface.
		applyNS, applySD bool
		// The global DNS servers.
		applyGlobalNS bool
	)
	if primary != nil {
		switch {
		case isCloudInit:
			applyNS = len(vmNS) == 0 && len(primary.Nameservers) == 0
			applySD = kubeutil.HasCAPILabels(ctx.VM.Labels) &&
				len(vmSS) == 0 && len(primary.SearchDomains) == 0
		case isLinuxPrep:
			// Only the primary interface is considered, even though the
			// global DNS servers also override the DNS servers from DHCP for
			// any other interface.
			applyGlobalNS = len(globalNS) == 0
		case isSysprep:
			applyNS = len(vmNS) == 0 && len(primary.Nameservers) == 0
		}
	}

	// Only read the ConfigMap when a default is applied, or to resolve the
	// DNS configuration for templates and status. Templates always fall back
	// to the Supervisor's default nameservers, even when every interface has
	// its own, so that templates that index them keep rendering.
	var cmNS, cmSS []string
	if applyNS || applySD || applyGlobalNS ||
		(!isCloudInit && len(globalNS) == 0) ||
		(!configuresGuestNetwork && missingSearches && len(globalSS) == 0) {

		var err error
		cmNS, cmSS, err = config.GetDNSInformationFromConfigMap(ctx, k8sClient)
		if err != nil && ctrlclient.IgnoreNotFound(err) != nil {
			// This ConfigMap doesn't exist in certain test envs.
			return err
		}
	}

	resolvedNS, resolvedSS := globalNS, globalSS
	if len(resolvedNS) == 0 {
		resolvedNS = cmNS
	}
	if len(resolvedSS) == 0 {
		resolvedSS = cmSS
	}
	bsa.TemplateDNSServers = resolvedNS

	// Only apply the Supervisor's default nameservers of the IP families the
	// primary interface has a gateway for.
	var primaryCMNS []string
	if primary != nil {
		ipv4, ipv6 := primary.GatewayFamilies()
		primaryCMNS = network.FilterNameserversByFamily(cmNS, ipv4, ipv6)

		if applyNS {
			primary.Nameservers = primaryCMNS
		}
		if applySD {
			primary.SearchDomains = cmSS
		}
	}

	// Set the global DNS configuration that the bootstrap engine applies. Any
	// configuration applied to an interface is reported in the interface's
	// status.
	switch {
	case isCloudInit:
		// CloudInit does not have a global DNS configuration, so all of it is
		// reported per-interface.
		bsa.DNSServers, bsa.SearchSuffixes = nil, nil
	case isLinuxPrep:
		bsa.DNSServers, bsa.SearchSuffixes = globalNS, globalSS
		if applyGlobalNS {
			bsa.DNSServers = primaryCMNS
		}
	case isSysprep:
		bsa.DNSServers, bsa.SearchSuffixes = nil, globalSS
	default:
		bsa.DNSServers, bsa.SearchSuffixes = resolvedNS, resolvedSS
	}

	return nil
}

// applyLegacyDNSDefaults applies the Supervisor's default DNS configuration
// to the VM as it had always been done prior to the ScopedDNSDefaults
// capability. This must not be changed.
func applyLegacyDNSDefaults(
	ctx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	bsa *BootstrapArgs) error {

	var bootstrap vmopv1.VirtualMachineBootstrapSpec
	if bs := ctx.VM.Spec.Bootstrap; bs != nil {
		bootstrap = *bs
	}

	isCloudInit := bootstrap.CloudInit != nil
	isGOSC := bootstrap.LinuxPrep != nil || bootstrap.Sysprep != nil
	bootstraps := bsa.NetBootstraps

	if ci := bootstrap.CloudInit; ci != nil {
		applyLegacyVMDNSToInterfaces(ci, bsa)
	}

	// If the VM is missing DNS info - that is, it did not specify DNS for the
	// interfaces - populate that now from the SV global configuration. Note
	// that the VM is probably OK as long as at least one interface has DNS
	// info, but we would previously set it for every interface so keep doing
	// that here. Similarly, we didn't populate SearchDomains for non-TKG VMs so
	// we don't here either. This is all a little nuts & complicated and
	// probably not correct for every situation.
	isTKG := kubeutil.HasCAPILabels(ctx.VM.Labels)
	getDNSInformationFromConfigMap := false
	for _, b := range bootstraps {
		if b.DHCP4 || b.DHCP6 {
			continue
		}

		if len(bsa.DNSServers) == 0 && len(b.Nameservers) == 0 {
			getDNSInformationFromConfigMap = true
			break
		}

		// V1ALPHA1: Do not default the global search suffixes for LinuxPrep and
		// Sysprep to what is in the ConfigMap.
		if len(b.SearchDomains) == 0 && (isTKG || (!isGOSC && len(bsa.SearchSuffixes) == 0)) {
			getDNSInformationFromConfigMap = true
			break
		}
	}

	if getDNSInformationFromConfigMap {
		ns, ss, err := config.GetDNSInformationFromConfigMap(ctx, k8sClient)
		if err != nil && ctrlclient.IgnoreNotFound(err) != nil {
			// This ConfigMap doesn't exist in certain test envs.
			return err
		}

		if len(bsa.DNSServers) == 0 {
			// GOSC will set this for its global config.
			bsa.DNSServers = ns
		}

		if !isGOSC && len(bsa.SearchSuffixes) == 0 {
			// See the comment above: we don't apply the global suffixes to
			// GOSC.
			bsa.SearchSuffixes = ss
		}

		if isCloudInit {
			// Previously we would apply the global DNS config to every
			// interface so do that here too.
			for i := range bootstraps {
				b := &bootstraps[i]

				if b.DHCP4 || b.DHCP6 {
					continue
				}

				if len(b.Nameservers) == 0 {
					b.Nameservers = ns
				}

				// V1ALPHA1: Only apply global search domains to TKG VMs.
				if isTKG && len(b.SearchDomains) == 0 {
					b.SearchDomains = ss
				}
			}
		}
	}

	bsa.TemplateDNSServers = bsa.DNSServers

	return nil
}

// applyLegacyVMDNSToInterfaces applies the VM-level DNS configuration to
// every interface that does not specify its own, unless disabled by the
// CloudInit spec, as it had always been done prior to the ScopedDNSDefaults
// capability.
func applyLegacyVMDNSToInterfaces(
	ci *vmopv1.VirtualMachineBootstrapCloudInitSpec,
	bsa *BootstrapArgs) {

	applyVMNS := ptr.DerefWithDefault(ci.UseGlobalNameserversAsDefault, true)
	applyVMSS := ptr.DerefWithDefault(ci.UseGlobalSearchDomainsAsDefault, true)
	for i := range bsa.NetBootstraps {
		b := &bsa.NetBootstraps[i]
		if applyVMNS && len(b.Nameservers) == 0 {
			b.Nameservers = bsa.DNSServers
		}
		if applyVMSS && len(b.SearchDomains) == 0 {
			b.SearchDomains = bsa.SearchSuffixes
		}
	}
}

func doReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	defaultConfigSpec := &vimtypes.VirtualMachineConfigSpec{}
	if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		logConfigSpec(vmCtx, *configSpec)

		if _, err := resources.NewVMFromObject(vcVM).Reconfigure(vmCtx, configSpec); err != nil {
			vmCtx.Logger.Error(err, "customization reconfigure failed")
			return err
		}
	}

	return nil
}

func doCustomize(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	customSpec *vimtypes.CustomizationSpec) error {

	var skipReason string

	if vmCtx.VM.Annotations[constants.VSphereCustomizationBypassKey] == constants.VSphereCustomizationBypassDisable {
		skipReason = "bypass annotation"
	} else if IsCustomizationPendingExtraConfig(config.ExtraConfig) {
		// TODO: We should really determine if the pending customization is
		//       stale, clear it if so, and then re-customize. Otherwise, the
		//       Customize call could perpetually fail preventing power on.
		skipReason = "already pending"
	}

	if skipReason != "" {
		vmCtx.Logger.Info("Skipping customization", "reason", skipReason)
		return nil
	}

	logCustomizationSpec(vmCtx, *customSpec)

	if err := resources.NewVMFromObject(vcVM).Customize(vmCtx, *customSpec); err != nil {
		// isCustomizationPendingExtraConfig() above is supposed to prevent this error, but
		// handle it explicitly here just in case so VM reconciliation can proceed.
		if !fault.Is(err, &vimtypes.CustomizationPending{}) {
			return err
		}
	}

	return nil
}

func IsCustomizationPendingExtraConfig(extraConfig []vimtypes.BaseOptionValue) bool {
	for _, opt := range extraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			if optValue.Key == constants.GOSCPendingExtraConfigKey {
				return optValue.Value.(string) != ""
			}
		}
	}
	return false
}

func logConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec) {

	if !pkgcfg.FromContext(vmCtx).LogSensitiveData {
		configSpec = SanitizeConfigSpec(configSpec)
	}

	vmCtx.Logger.Info("Customization Reconfigure", "configSpec", pkgutil.SafeConfigSpecToString(&configSpec))
}

func SanitizeConfigSpec(cs vimtypes.VirtualMachineConfigSpec) vimtypes.VirtualMachineConfigSpec {

	cs.ExtraConfig = slices.Clone(cs.ExtraConfig)
	for i, ec := range cs.ExtraConfig {
		optVal := ec.GetOptionValue()
		if optVal == nil {
			continue
		}

		// This is what is likely to contain any sensitive. We can expand this to vendor
		// and metadata later if needed.
		if optVal.Key == constants.CloudInitGuestInfoUserdata {
			optValCopy := *optVal
			optValCopy.Value = redacted
			cs.ExtraConfig[i] = &optValCopy
			break
		}
	}

	if vAppConfig := cs.VAppConfig; vAppConfig != nil && vAppConfig.GetVmConfigSpec() != nil {
		vmConfigSpec := *vAppConfig.GetVmConfigSpec()

		vmConfigSpec.Property = slices.Clone(vmConfigSpec.Property)
		for i, vmProp := range vmConfigSpec.Property {
			if vmProp.Info == nil || vmProp.Info.UserConfigurable == nil || !*vmProp.Info.UserConfigurable {
				continue
			}

			info := *vmProp.Info
			info.Value = redacted
			vmConfigSpec.Property[i].Info = &info
		}

		cs.VAppConfig = &vmConfigSpec
	}

	return cs
}

func logCustomizationSpec(
	vmCtx pkgctx.VirtualMachineContext,
	customizationSpec vimtypes.CustomizationSpec) {

	if !pkgcfg.FromContext(vmCtx).LogSensitiveData {
		customizationSpec = SanitizeCustomizationSpec(customizationSpec)
	}

	vmCtx.Logger.Info("Customizing VM", "customizationSpec", customizationSpec)
}

func SanitizeCustomizationSpec(cs vimtypes.CustomizationSpec) vimtypes.CustomizationSpec {
	switch identity := cs.Identity.(type) {
	case *internal.CustomizationCloudinitPrep:
		cloudInitPrep := *identity
		cloudInitPrep.Userdata = redacted
		cs.Identity = &cloudInitPrep
	case *vimtypes.CustomizationLinuxPrep:
		linuxPrep := *identity
		if linuxPrep.Password != nil {
			password := *linuxPrep.Password
			password.Value = redacted
			linuxPrep.Password = &password
		}
		if linuxPrep.ScriptText != "" {
			linuxPrep.ScriptText = redacted
		}
		cs.Identity = &linuxPrep
	case *vimtypes.CustomizationSysprepText:
		sysPrepText := *identity
		sysPrepText.Value = redacted
		cs.Identity = &sysPrepText
	case *vimtypes.CustomizationSysprep:
		sysPrep := *identity
		if sysPrep.GuiUnattended.Password != nil {
			password := *sysPrep.GuiUnattended.Password
			if password.Value != "" {
				password.Value = redacted
			}
			sysPrep.GuiUnattended.Password = &password
		}
		if sysPrep.Identification.DomainAdminPassword != nil {
			password := *sysPrep.Identification.DomainAdminPassword
			if password.Value != "" {
				password.Value = redacted
			}
			sysPrep.Identification.DomainAdminPassword = &password
		}
		if sysPrep.ScriptText != "" {
			sysPrep.ScriptText = redacted
		}
		cs.Identity = &sysPrep
	}

	return cs
}

func getVimTypeHash(obj vimtypes.AnyType) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal vim type to json: %w", err)
	}
	h := xxhash.New()
	if _, err := h.Write(data); err != nil {
		return "", fmt.Errorf("failed to write vim type to hash: %w", err)
	}
	out := h.Sum(nil)
	return fmt.Sprintf("%x", out), nil
}

// appendUnique appends each of values to dst that is not already in dst.
func appendUnique(dst []string, values ...string) []string {
	for _, v := range values {
		if !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}
