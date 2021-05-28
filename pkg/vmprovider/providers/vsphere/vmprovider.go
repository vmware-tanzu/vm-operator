// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

const (
	VsphereVmProviderName = "vsphere"
)

var log = logf.Log.WithName(VsphereVmProviderName)

type vSphereVmProvider struct {
	sessions      SessionManager
	eventRecorder record.Recorder
}

func NewVSphereVmProviderFromClient(client ctrlruntime.Client, scheme *runtime.Scheme,
	recorder record.Recorder) vmprovider.VirtualMachineProviderInterface {
	vmProvider := &vSphereVmProvider{
		sessions:      NewSessionManager(client, scheme),
		eventRecorder: recorder,
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
func (vs *vSphereVmProvider) ListVirtualMachineImagesFromContentLibrary(
	ctx context.Context,
	contentLibrary v1alpha1.ContentLibraryProvider,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary", "name", contentLibrary.Name, "UUID", contentLibrary.Spec.UUID)

	ses, err := vs.sessions.GetSession(ctx, "")
	if err != nil {
		return nil, err
	}

	return ses.ListVirtualMachineImagesFromCL(ctx, contentLibrary.Spec.UUID, currentCLImages)
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

// ParseVirtualHardwareVersion parses the virtual hardware version
// For eg. "vmx-15" returns 15.
func ParseVirtualHardwareVersion(vmxVersion *string) int32 {
	patternStr := `vmx-(\d+)`
	re, err := regexp.Compile(patternStr)
	if err != nil {
		return 0
	}
	// obj matches the full string and the submatch (\d+)
	// and return a []string with values
	obj := re.FindStringSubmatch(*vmxVersion)
	if len(obj) != 2 {
		return 0
	}

	version, err := strconv.ParseInt(obj[1], 10, 32)
	if err != nil {
		return 0
	}

	return int32(version)
}

// libItemVersionAnnotation returns the version annotation value for the item
func libItemVersionAnnotation(item *library.Item) string {
	return fmt.Sprintf("%s:%s:%d", item.ID, item.Version, VMImageCLVersionAnnotationVersion)
}

// LibItemToVirtualMachineImage converts a given library item and its attributes to return a
// VirtualMachineImage that represents a k8s-native view of the item.
func LibItemToVirtualMachineImage(
	item *library.Item,
	ovfEnvelope *ovf.Envelope) *v1alpha1.VirtualMachineImage {

	var ts metav1.Time
	if item.CreationTime != nil {
		ts = metav1.NewTime(*item.CreationTime)
	}

	// NOTE: Whenever a Spec/Status field, label, annotation, etc is added or removed, the or the logic
	// is changed as to what is set, the VMImageCLVersionAnnotationVersion very, very likely needs to be
	// incremented so that the VMImageCLVersionAnnotation annotations changes so the updated is actually
	// updated. This is a hack to reduce repeated ContentLibrary tasks.
	image := &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              item.Name,
			CreationTimestamp: ts,
			Annotations: map[string]string{
				VMImageCLVersionAnnotation: libItemVersionAnnotation(item),
			},
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

			// Use hardware section info from the VM image, if one exists.
			var hwVersion int32
			if virtualHwSection := ovfEnvelope.VirtualSystem.VirtualHardware; len(virtualHwSection) > 0 {
				hw := virtualHwSection[0]
				if hw.System != nil && hw.System.VirtualSystemType != nil {
					hwVersion = ParseVirtualHardwareVersion(hw.System.VirtualSystemType)
				}
			}

			ovfSystemProps := GetVmwareSystemPropertiesFromOvf(ovfEnvelope)

			for k, v := range ovfSystemProps {
				image.Annotations[k] = v
			}
			image.Spec.ProductInfo = productInfo
			image.Spec.OSInfo = osInfo
			image.Spec.OVFEnv = GetUserConfigurablePropertiesFromOvf(ovfEnvelope)
			image.Spec.HardwareVersion = hwVersion

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

		// Set Status.ImageSupported to combined compatibility of OVF compatibility.
		image.Status.ImageSupported = pointer.BoolPtr(conditions.IsTrue(image,
			v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition))
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
