// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/sequence"
)

const (
	VsphereVmProviderName = "vsphere"

	// Annotation Key for vSphere MoRef
	VmOperatorMoRefKey          = pkg.VmOperatorKey + "/moref"
	VmOperatorVCInstanceUUIDKey = pkg.VmOperatorKey + "/vc-instance-uuid"
	VmOperatorInstanceUUIDKey   = pkg.VmOperatorKey + "/instance-uuid"
	VmOperatorBiosUUIDKey       = pkg.VmOperatorKey + "/bios-uuid"
	VmOperatorResourcePoolKey   = pkg.VmOperatorKey + "/resource-pool"

	// Annotation denoting whether to fetch ovf properties for VM image
	// and annotate VM. Value can be unset or "true" (we should fetch ovf properties)
	// or "false" (we should not fetch ovf properties)
	VmOperatorVMImagePropsKey = pkg.VmOperatorKey + "/annotate-vm-image-props"

	// TODO: Rename and move to vmoperator-api
	// Annotation key to skip validation checks of GuestOS Type
	VMOperatorImageSupportedCheckKey     = pkg.VmOperatorKey + "/image-supported-check"
	VMOperatorImageSupportedCheckDisable = "disable"

	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5

	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"
)

//go:generate mockgen -destination=./mocks/mock_ovf_property_retriever.go -package=mocks github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere OvfPropertyRetriever

type OvfPropertyRetriever interface {
	GetOvfInfoFromLibraryItem(ctx context.Context, session *Session, item *library.Item) (*ovf.Envelope, error)
	GetOvfInfoFromVM(ctx context.Context, resVm *res.VirtualMachine) (map[string]string, error)
}

type vmOptions struct{}

var _ OvfPropertyRetriever = vmOptions{}

type ImageOptions int

const (
	AnnotateVmImage ImageOptions = iota
	DoNotAnnotateVmImage
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
	IsSessionInCache(namespace string) bool
}

func (vs *vSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *vSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *vSphereVmProvider) GetSession(ctx context.Context, namespace string) (*Session, error) {
	return vs.sessions.GetSession(ctx, namespace)
}

func (vs *vSphereVmProvider) IsSessionInCache(namespace string) bool {
	vs.sessions.mutex.Lock()
	defer vs.sessions.mutex.Unlock()
	_, ok := vs.sessions.sessions[namespace]
	return ok
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

// AddProviderAnnotations adds VM provider annotations to the VirtualMachine object
// AKP: No comsumers of this yet. So commenting out. Uncomment when we start to add these in `vs.updateVirtualMachine`.
// func AddProviderAnnotations(session *Session, objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {

// 	annotations := objectMeta.GetAnnotations()
// 	if annotations == nil {
// 		annotations = make(map[string]string)
// 	}

// 	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
// 	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

// 	// Take missing annotations as a trigger to gather the information we need to populate the annotations.  We want to
// 	// avoid putting unnecessary pressure on content library.
// 	if _, ok := annotations[VmOperatorBiosUUIDKey]; !ok {
// 		biosUUID, err := vmRes.BiosUUID(context.Background())
// 		if err == nil {
// 			annotations[VmOperatorBiosUUIDKey] = biosUUID
// 		}
// 	}

// 	if _, ok := annotations[VmOperatorInstanceUUIDKey]; !ok {
// 		instanceUUID, err := vmRes.InstanceUUID(context.Background())
// 		if err == nil {
// 			annotations[VmOperatorInstanceUUIDKey] = instanceUUID
// 		}
// 	}

// 	if _, ok := annotations[VmOperatorVCInstanceUUIDKey]; !ok {
// 		about, err := session.ServiceContent(context.Background())
// 		if err == nil {
// 			annotations[VmOperatorVCInstanceUUIDKey] = about.InstanceUuid
// 		}
// 	}

// 	if _, ok := annotations[VmOperatorResourcePoolKey]; !ok {
// 		resourcePool, err := vmRes.ResourcePool(context.Background())
// 		if err == nil {
// 			annotations[VmOperatorResourcePoolKey] = resourcePool
// 		}
// 	}

// 	var vmOpts OvfPropertyRetriever = vmOptions{}
// 	err := AddVmImageAnnotations(annotations, context.Background(), vmOpts, vmRes)
// 	if err != nil {
// 		log.Error(err, "Error adding image annotations to VM", "vm", vmRes.Name)
// 	}

// 	objectMeta.SetAnnotations(annotations)
// }

// AddVmImageAnnotations adds annotations from the VM image to the the VirtualMachine object
func AddVmImageAnnotations(annotations map[string]string, ctx context.Context, ovfPropRetriever OvfPropertyRetriever, vmRes *res.VirtualMachine) error {
	if val, ok := annotations[VmOperatorVMImagePropsKey]; !ok || val == "true" {
		ovfProperties, err := ovfPropRetriever.GetOvfInfoFromVM(ctx, vmRes)
		if err != nil {
			return err
		}
		for ovfPropKey, ovfPropValue := range ovfProperties {
			annotations[ovfPropKey] = ovfPropValue
		}
		// Signify we don't need to fetch ovf properties again since we
		// want to avoid putting pressure on content library.
		annotations[VmOperatorVMImagePropsKey] = "false"
	}
	return nil
}

func (vs *vSphereVmProvider) getOpId(ctx context.Context, vm *v1alpha1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	id := make([]byte, 8)
	for i := range id {
		id[i] = charset[rand.Intn(len(charset))]
	}

	clusterID, _ := vs.getClusterID(ctx, vm.Namespace)
	return strings.Join([]string{"vmoperator", clusterID, vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	ctx = context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "create"))

	vmName := vm.NamespacedName()
	log.Info("Creating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.CloneVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		log.Error(err, "Clone VirtualMachine failed", "name", vmName)
		return transformVmError(vmName, err)
	}

	if err := vs.mergeVmStatus(ctx, vm, resVm); err != nil {
		return err
	}

	return nil
}

// UpdateVirtualMachine updates the VM status, power state, phase etc
func (vs *vSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	ctx = context.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "update"))

	vmName := vm.NamespacedName()
	log.V(4).Info("Updating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	err = vs.updateVirtualMachine(ctx, ses, vm, vmConfigArgs)
	if err != nil {
		return transformVmError(vmName, err)
	}

	return nil
}

func (vs *vSphereVmProvider) updateVirtualMachine(ctx context.Context, session *Session, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	resVm, err := session.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		return err
	}

	err = vs.attachTagsToVmAndAddToClusterModules(ctx, vm, vmConfigArgs.ResourcePolicy)
	if err != nil {
		return err
	}

	// We were doing Status().Update() so these were never getting applied to the VM.
	// Some of these annotations like the OVF properties as massive so disable all of
	// until we can figure out what we actually needed or want.
	//AddProviderAnnotations(session, &vm.ObjectMeta, resVm)

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	vmCtx := VMContext{
		Context: ctx,
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Deleting VirtualMachine")

	ses, err := vs.sessions.GetSession(ctx, vmCtx.VM.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(vmCtx)
	if err != nil {
		return transformVmError(vmCtx.VM.NamespacedName(), err)
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vm, resVm)
	if err := deleteSequence.Execute(ctx); err != nil {
		vmCtx.Logger.Error(err, "Delete VirtualMachine sequence failed")
		return err
	}

	return nil
}

// mergeVmStatus merges the v1alpha1 VM's status with resource VM's status
func (vs *vSphereVmProvider) mergeVmStatus(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	vmStatus, err := resVm.GetStatus(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to get VirtualMachine status")
	}

	// BMV: This just ain't right.
	vmStatus.Volumes = vm.Status.Volumes
	vmStatus.Conditions = vm.Status.Conditions
	vmStatus.DeepCopyInto(&vm.Status)

	return nil
}

func (vs *vSphereVmProvider) getClusterID(ctx context.Context, namespace string) (string, error) {
	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return "", err
	}
	if ses.cluster == nil {
		return "", errors.Errorf("no cluster exists")
	}
	return ses.cluster.Reference().Value, nil
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

func ResVmToVirtualMachineImage(ctx context.Context, resVm *res.VirtualMachine, imgOptions ImageOptions, ovfPropRetriever OvfPropertyRetriever) (*v1alpha1.VirtualMachineImage, error) {
	powerState, uuid, reference := resVm.ImageFields(ctx)

	ovfProperties := make(map[string]string)
	if imgOptions == AnnotateVmImage {
		var err error
		ovfProperties, err = ovfPropRetriever.GetOvfInfoFromVM(ctx, resVm)
		if err != nil {
			return nil, err
		}
	}

	var ts v1.Time
	if creationTime, _ := resVm.GetCreationTime(ctx); creationTime != nil {
		ts = v1.NewTime(*creationTime)
	}
	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:              resVm.Name,
			Annotations:       ovfProperties,
			CreationTimestamp: ts,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       uuid,
			InternalId: reference,
			PowerState: powerState,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
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
	if ovfEnvelope.VirtualSystem != nil && ovfEnvelope.VirtualSystem.VirtualHardware != nil {
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
		return nil, fmt.Errorf("No cluster exists, can't get OS Descriptors")
	}

	log.V(4).Info("Fetching all supported guestOS types for the cluster")
	var computeResource mo.ComputeResource
	obj := cluster.Reference()

	err := cluster.Properties(ctx, obj, []string{"environmentBrowser"}, &computeResource)
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
		//Fetch all ids and families that have supportLevel other than unsupported
		if descriptor.SupportLevel != "unsupported" {
			guestOSIdsToFamily[descriptor.Id] = descriptor.Family
		}
	}

	return guestOSIdsToFamily, nil
}

// isATKGImage validates if a VirtualMachineImage OVF is a TKG Image type
func isATKGImage(systemProperties map[string]string) bool {
	tkgImageIdentifier := "vmware-system.guest.kubernetes"
	for key := range systemProperties {
		if strings.HasPrefix(key, tkgImageIdentifier) {
			return true
		}
	}
	return false
}

// LibItemToVirtualMachineImage converts a given library item and its attributes to return a VirtualMachineImage that represents a k8s-native
// view of the item.
func LibItemToVirtualMachineImage(ctx context.Context, session *Session, item *library.Item, imgOptions ImageOptions, ovfPropRetriever OvfPropertyRetriever,
	gOSIdsToFamily map[string]string) (*v1alpha1.VirtualMachineImage, error) {

	var (
		ovfSystemProps  = make(map[string]string)
		ovfProperties   = make(map[string]v1alpha1.OvfProperty)
		productInfo     = &v1alpha1.VirtualMachineImageProductInfo{}
		osInfo          = &v1alpha1.VirtualMachineImageOSInfo{}
		isOVFCompatible = false
	)

	if item.Type == library.ItemTypeOVF {
		ovfEnvelope, err := ovfPropRetriever.GetOvfInfoFromLibraryItem(ctx, session, item)
		if err != nil {
			return nil, err
		}
		if ovfEnvelope.VirtualSystem != nil {
			// Fetch the system properties when there is a VirtualSystem block in the OVF
			systemProps := GetVmwareSystemPropertiesFromOvf(ovfEnvelope)
			if imgOptions == AnnotateVmImage {
				ovfSystemProps = systemProps

			}
			os := ovfEnvelope.VirtualSystem.OperatingSystem
			product := ovfEnvelope.VirtualSystem.Product

			// Use info from the first product section in the VM image, if one exists.
			if len(product) > 0 {
				p := product[0]
				productInfo.Vendor = p.Vendor
				productInfo.Product = p.Product
				productInfo.FullVersion = p.FullVersion
				productInfo.Version = p.Version
			}

			// Use operating system info from the first os section in the VM image, if one exists.
			if len(os) > 0 {
				o := os[0]

				if o.Version != nil {
					osInfo.Version = *o.Version
				}

				if o.OSType != nil {
					osInfo.Type = *o.OSType
				}
			}

			// Allow OVF compatibility if
			// - The OVF contains the VMOperatorV1Alpha1ConfigKey key that denotes cloud-init being disabled at first-boot
			// - If it is a TKG image
			isOVFCompatible = isOVFV1Alpha1Compatible(ovfEnvelope) || isATKGImage(systemProps)

			// Populate ovf properties
			ovfProperties = GetUserConfigurablePropertiesFromOvf(ovfEnvelope)
		}
	}

	var ts v1.Time
	if item.CreationTime != nil {
		ts = v1.NewTime(*item.CreationTime)
	}

	image := &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:              item.Name,
			Annotations:       ovfSystemProps,
			CreationTimestamp: ts,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            item.Type,
			ImageSourceType: "Content Library",
			ProductInfo:     *productInfo,
			OSInfo:          *osInfo,
			OVFEnv:          ovfProperties,
		},
	}
	if item.Type == library.ItemTypeOVF {
		if isOVFCompatible {
			conditions.MarkTrue(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
		} else {
			msg := "VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1"
			conditions.MarkFalse(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
				v1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason, v1alpha1.ConditionSeverityError, msg)
		}

		isSupportedGuestOS := false
		// Set the isSupportedGuestOS to true if the GuestOS Descriptors IDs map was populated
		if len(gOSIdsToFamily) > 0 {
			// gOSFamily will be present for supported OSTypes and
			// support only VirtualMachineGuestOsFamilyLinuxGuest for now
			gOSFamily := gOSIdsToFamily[osInfo.Type]
			if gOSFamily != "" && gOSFamily == string(vimtypes.VirtualMachineGuestOsFamilyLinuxGuest) {
				isSupportedGuestOS = true
				conditions.MarkTrue(image, v1alpha1.VirtualMachineImageOSTypeSupportedCondition)
			} else {
				isSupportedGuestOS = false
				msg := fmt.Sprintf("VirtualMachineImage image type %s is not supported by VM Svc", osInfo.Type)
				conditions.MarkFalse(image, v1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					v1alpha1.VirtualMachineImageOSTypeNotSupportedReason, v1alpha1.ConditionSeverityError, msg)
			}
		} else {
			// bypass isSupportedGuestOS valdation as GuestOS Descriptors IDs map was not populated
			isSupportedGuestOS = true
		}
		// Update VirtualMachineImageStatus.ImageSupported to combined compatibility of OVF compatibility and supported
		// guest OS
		imageSupportedState := isOVFCompatible && isSupportedGuestOS
		image.Status.ImageSupported = &imageSupportedState
	}

	return image, nil
}

func (vm vmOptions) GetOvfInfoFromLibraryItem(ctx context.Context, session *Session, item *library.Item) (*ovf.Envelope, error) {
	contentLibSession := NewContentLibraryProvider(session)

	clDownloadHandler := createClDownloadHandler()

	ovfEnvelope, err := contentLibSession.RetrieveOvfEnvelopeFromLibraryItem(ctx, item, clDownloadHandler)
	if err != nil {
		return nil, err
	}

	return ovfEnvelope, nil
}

// TODO: Convert this to return all OVF info rather than a pre-parsed map to be as consistent as possible with
// GetOvfInfoFromLibraryItem.
func (vm vmOptions) GetOvfInfoFromVM(ctx context.Context, resVm *res.VirtualMachine) (map[string]string, error) {
	return resVm.GetOvfProperties(ctx)
}

func createClDownloadHandler() ContentDownloadHandler {
	// Integration test environment would require a much lesser wait time
	envClApiWaitSecs := os.Getenv(EnvContentLibApiWaitSecs)

	value, err := strconv.Atoi(envClApiWaitSecs)
	if err != nil {
		value = DefaultContentLibApiWaitSecs
	}

	return ContentDownloadProvider{ApiWaitTimeSecs: value}
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
