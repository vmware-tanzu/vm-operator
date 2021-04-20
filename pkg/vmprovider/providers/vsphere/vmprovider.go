// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

const (
	VsphereVmProviderName = "vsphere"

	// Annotation Key for vSphere MoRef
	//VmOperatorVCInstanceUUIDKey = pkg.VmOperatorKey + "/vc-instance-uuid"
	//VmOperatorResourcePoolKey   = pkg.VmOperatorKey + "/resource-pool"

	// TODO: Rename and move to vmoperator-api
	// Annotation key to skip validation checks of GuestOS Type
	VMOperatorImageSupportedCheckKey     = pkg.VmOperatorKey + "/image-supported-check"
	VMOperatorImageSupportedCheckDisable = "disable"

	VSphereCustomizationBypassKey     = pkg.VmOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5

	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"
)

var log = logf.Log.WithName(VsphereVmProviderName)

type vSphereVmProvider struct {
	sessions SessionManager
}

func NewVSphereVmProviderFromClient(client ctrlruntime.Client, scheme *runtime.Scheme) vmprovider.VirtualMachineProviderInterface {
	vmProvider := &vSphereVmProvider{
		sessions: NewSessionManager(client, scheme),
	}

	return vmProvider
}

// VSphereVmProviderGetSessionHack is an interface that exposes helpful functions to access certain resources without polluting the vm provider interface.
// Typical usecase might be a VM image test that wants to set a custom content library.
// ONLY USED IN TESTS.
type VSphereVmProviderGetSessionHack interface {
	GetSession(ctx context.Context, namespace string) (*Session, error)
}

func (vs *vSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *vSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *vSphereVmProvider) GetSession(ctx context.Context, namespace string) (*Session, error) {
	return vs.sessions.GetSession(ctx, namespace)
}

func (vs *vSphereVmProvider) DeleteNamespaceSessionInCache(ctx context.Context, namespace string) {
	log.V(4).Info("removing namespace from session cache", "namespace", namespace)

	vs.sessions.mutex.Lock()
	defer vs.sessions.mutex.Unlock()
	delete(vs.sessions.sessions, namespace)
}

// ListVirtualMachineImagesFromContentLibrary lists VM images from a ContentLibrary
func (vs *vSphereVmProvider) ListVirtualMachineImagesFromContentLibrary(ctx context.Context, contentLibrary v1alpha1.ContentLibraryProvider) ([]*v1alpha1.VirtualMachineImage, error) {
	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary", "name", contentLibrary.Name, "UUID", contentLibrary.Spec.UUID)

	ses, err := vs.sessions.GetSession(ctx, "")
	if err != nil {
		return nil, err
	}

	return ses.ListVirtualMachineImagesFromCL(ctx, contentLibrary.Spec.UUID)
}

func (vs *vSphereVmProvider) DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error) {
	vmCtx := VMContext{
		Context: ctx,
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSession(vmCtx, vmCtx.VM.Namespace)
	if err != nil {
		return false, err
	}

	if _, err = ses.GetVirtualMachine(vmCtx); err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func (vs *vSphereVmProvider) getOpId(ctx context.Context, vm *v1alpha1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	// TODO: Is this actually useful? Avoid looking up the session multiple times.
	var clusterID string
	if ses, err := vs.sessions.GetSession(ctx, vm.Namespace); err == nil {
		clusterID = ses.cluster.Reference().Value
	}

	id := make([]byte, 8)
	for i := range id {
		id[i] = charset[rand.Intn(len(charset))]
	}

	return strings.Join([]string{"vmoperator", clusterID, vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	vmCtx := VMContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "create")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Creating VirtualMachine")

	ses, err := vs.sessions.GetSession(vmCtx, vm.Namespace)
	if err != nil {
		return err
	}

	resVM, err := ses.CloneVirtualMachine(vmCtx, vmConfigArgs)
	if err != nil {
		vmCtx.Logger.Error(err, "Clone VirtualMachine failed")
		return err
	}

	// Set a few Status fields that we easily have on hand here. The controller will immediately call
	// UpdateVirtualMachine() which will set it all.
	vm.Status.Phase = v1alpha1.Created
	vm.Status.UniqueID = resVM.MoRef().Value

	return nil
}

// UpdateVirtualMachine updates the VM status, power state, phase etc
func (vs *vSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	vmCtx := VMContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "update")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")

	ses, err := vs.sessions.GetSession(vmCtx, vm.Namespace)
	if err != nil {
		return err
	}

	err = ses.UpdateVirtualMachine(vmCtx, vmConfigArgs)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	vmCtx := VMContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "delete")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Deleting VirtualMachine")

	ses, err := vs.sessions.GetSession(ctx, vmCtx.VM.Namespace)
	if err != nil {
		return err
	}

	err = ses.DeleteVirtualMachine(vmCtx)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to delete VM")
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error) {
	vmCtx := VMContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSession(ctx, vmCtx.VM.Namespace)
	if err != nil {
		return "", err
	}

	status, err := ses.GetVirtualMachineGuestHeartbeat(vmCtx)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVmProvider) ComputeClusterCpuMinFrequency(ctx context.Context) error {

	if err := vs.sessions.ComputeClusterCpuMinFrequency(ctx); err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error {
	return vs.sessions.UpdateVcPNID(ctx, vcPNID, vcPort)
}

func (vs *vSphereVmProvider) ClearSessionsAndClient(ctx context.Context) {
	vs.sessions.clearSessionsAndClient(ctx)
}

func ResVmToVirtualMachineImage(ctx context.Context, resVM *res.VirtualMachine) (*v1alpha1.VirtualMachineImage, error) {
	ovfProperties, err := resVM.GetOvfProperties(ctx)
	if err != nil {
		return nil, err
	}

	// Prior code just used default values if the Properties called failed.
	moVM, _ := resVM.GetProperties(ctx, []string{"config.createDate", "summary"})

	var createTimestamp metav1.Time
	if moVM.Config != nil && moVM.Config.CreateDate != nil {
		createTimestamp = metav1.NewTime(*moVM.Config.CreateDate)
	}

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              resVM.Name,
			Annotations:       ovfProperties,
			CreationTimestamp: createTimestamp,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       moVM.Summary.Config.Uuid,
			InternalId: resVM.ReferenceValue(),
			PowerState: string(moVM.Summary.Runtime.PowerState),
		},
	}, nil
}

func GetUserConfigurablePropertiesFromOvf(ovfEnvelope *ovf.Envelope) map[string]v1alpha1.OvfProperty {
	properties := make(map[string]v1alpha1.OvfProperty)

	if ovfEnvelope.VirtualSystem != nil {
		for _, product := range ovfEnvelope.VirtualSystem.Product {
			for _, prop := range product.Property {
				// Only show user configurable properties
				if prop.UserConfigurable != nil && *prop.UserConfigurable {
					property := v1alpha1.OvfProperty{
						Key:     prop.Key,
						Type:    prop.Type,
						Default: prop.Default,
					}
					properties[prop.Key] = property
				}
			}
		}
	}
	return properties
}

func GetVmwareSystemPropertiesFromOvf(ovfEnvelope *ovf.Envelope) map[string]string {
	properties := make(map[string]string)

	if ovfEnvelope.VirtualSystem != nil {
		for _, product := range ovfEnvelope.VirtualSystem.Product {
			for _, prop := range product.Property {
				if strings.HasPrefix(prop.Key, "vmware-system") {
					properties[prop.Key] = *prop.Default
				}
			}
		}
	}
	return properties
}

// isOVFV1Alpha1Compatible checks the image if it has VMOperatorV1Alpha1ExtraConfigKey set to VMOperatorV1Alpha1ConfigReady
// in the ExtraConfig
func isOVFV1Alpha1Compatible(ovfEnvelope *ovf.Envelope) bool {
	if ovfEnvelope.VirtualSystem != nil {
		for _, virtualHardware := range ovfEnvelope.VirtualSystem.VirtualHardware {
			for _, config := range virtualHardware.ExtraConfig {
				if config.Key == VMOperatorV1Alpha1ExtraConfigKey && config.Value == VMOperatorV1Alpha1ConfigReady {
					return true
				}
			}
		}
	}
	return false
}

// GetValidGuestOSDescriptorIDs fetches valid guestOS descriptor IDs for the cluster
func GetValidGuestOSDescriptorIDs(ctx context.Context, cluster *object.ClusterComputeResource, client *vim25.Client) (map[string]string, error) {
	if cluster == nil {
		return nil, fmt.Errorf("no cluster exists, can't get OS Descriptors")
	}

	log.V(4).Info("Fetching all supported guestOS types for the cluster")
	var computeResource mo.ComputeResource
	err := cluster.Properties(ctx, cluster.Reference(), []string{"environmentBrowser"}, &computeResource)
	if err != nil {
		log.Error(err, "Failed to get cluster properties")
		return nil, err
	}

	req := vimtypes.QueryConfigOptionEx{
		This: *computeResource.EnvironmentBrowser,
		Spec: &vimtypes.EnvironmentBrowserConfigOptionQuerySpec{},
	}

	opt, err := methods.QueryConfigOptionEx(ctx, client.RoundTripper, &req)
	if err != nil {
		log.Error(err, "Failed to query config options for valid GuestOS types")
		return nil, err
	}

	guestOSIdsToFamily := make(map[string]string)
	for _, descriptor := range opt.Returnval.GuestOSDescriptor {
		// Fetch all ids and families that have supportLevel other than unsupported
		if descriptor.SupportLevel != "unsupported" {
			guestOSIdsToFamily[descriptor.Id] = descriptor.Family
		}
	}

	return guestOSIdsToFamily, nil
}

// isATKGImage validates if a VirtualMachineImage OVF is a TKG Image type
func isATKGImage(systemProperties map[string]string) bool {
	const tkgImageIdentifier = "vmware-system.guest.kubernetes"
	for key := range systemProperties {
		if strings.HasPrefix(key, tkgImageIdentifier) {
			return true
		}
	}
	return false
}

// LibItemToVirtualMachineImage converts a given library item and its attributes to return a
// VirtualMachineImage that represents a k8s-native view of the item.
func LibItemToVirtualMachineImage(
	item *library.Item,
	ovfEnvelope *ovf.Envelope,
	gOSIdsToFamily map[string]string) *v1alpha1.VirtualMachineImage {

	var ts metav1.Time
	if item.CreationTime != nil {
		ts = metav1.NewTime(*item.CreationTime)
	}

	image := &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              item.Name,
			CreationTimestamp: ts,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            item.Type,
			ImageSourceType: "Content Library",
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
		},
	}

	if item.Type == library.ItemTypeOVF {
		if ovfEnvelope.VirtualSystem != nil {
			productInfo := v1alpha1.VirtualMachineImageProductInfo{}
			osInfo := v1alpha1.VirtualMachineImageOSInfo{}

			// Use info from the first product section in the VM image, if one exists.
			if product := ovfEnvelope.VirtualSystem.Product; len(product) > 0 {
				p := product[0]
				productInfo.Vendor = p.Vendor
				productInfo.Product = p.Product
				productInfo.Version = p.Version
				productInfo.FullVersion = p.FullVersion
			}

			// Use operating system info from the first os section in the VM image, if one exists.
			if os := ovfEnvelope.VirtualSystem.OperatingSystem; len(os) > 0 {
				o := os[0]
				if o.Version != nil {
					osInfo.Version = *o.Version
				}
				if o.OSType != nil {
					osInfo.Type = *o.OSType
				}
			}

			ovfSystemProps := GetVmwareSystemPropertiesFromOvf(ovfEnvelope)

			image.Annotations = ovfSystemProps
			image.Spec.ProductInfo = productInfo
			image.Spec.OSInfo = osInfo
			image.Spec.OVFEnv = GetUserConfigurablePropertiesFromOvf(ovfEnvelope)

			// Allow OVF compatibility if
			// - The OVF contains the VMOperatorV1Alpha1ConfigKey key that denotes cloud-init being disabled at first-boot
			// - If it is a TKG image
			if isOVFV1Alpha1Compatible(ovfEnvelope) || isATKGImage(ovfSystemProps) {
				conditions.MarkTrue(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
			} else {
				msg := "VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1"
				conditions.MarkFalse(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					v1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason, v1alpha1.ConditionSeverityError, msg)
			}
		}

		isSupportedGuestOS := false
		// Set the isSupportedGuestOS to true if the GuestOS Descriptors IDs map was populated
		if len(gOSIdsToFamily) > 0 {
			osType := image.Spec.OSInfo.Type

			// osFamily will be present for supported OSTypes and support only VirtualMachineGuestOsFamilyLinuxGuest for now
			if osFamily := gOSIdsToFamily[osType]; osFamily == string(vimtypes.VirtualMachineGuestOsFamilyLinuxGuest) {
				isSupportedGuestOS = true
				conditions.MarkTrue(image, v1alpha1.VirtualMachineImageOSTypeSupportedCondition)
			} else {
				// BMV: This message is weird if OSType wasn't populated above.
				msg := fmt.Sprintf("VirtualMachineImage image type %s is not supported by VMService", osType)
				conditions.MarkFalse(image, v1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					v1alpha1.VirtualMachineImageOSTypeNotSupportedReason, v1alpha1.ConditionSeverityError, msg)
			}
		} else {
			// Bypass isSupportedGuestOS validation as GuestOS Descriptors IDs map was not populated
			isSupportedGuestOS = true
		}

		// Set Status.ImageSupported to combined compatibility of OVF compatibility and supported guest OS.
		image.Status.ImageSupported = pointer.BoolPtr(isSupportedGuestOS &&
			conditions.IsTrue(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition))
	}

	return image
}

// Transform Govmomi error to Kubernetes error
// TODO: Fill out with VIM fault types
func transformError(resourceType string, resource string, err error) error {
	switch err.(type) {
	case *find.NotFoundError, *find.DefaultNotFoundError:
		return k8serrors.NewNotFound(schema.GroupResource{Group: "vmoperator.vmware.com", Resource: strings.ToLower(resourceType)}, resource)
	case *find.MultipleFoundError, *find.DefaultMultipleFoundError:
		// Transform?
		return err
	default:
		return err
	}
}

func transformVmError(resource string, err error) error {
	return transformError("VirtualMachine", resource, err)
}
