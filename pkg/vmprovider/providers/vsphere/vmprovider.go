// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	"github.com/vmware-tanzu/vm-operator/controllers/util"
	"github.com/vmware-tanzu/vm-operator/pkg"
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

	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5
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

func NewVSphereVmProviderFromClients(ncpClient ncpclientset.Interface, client ctrlruntime.Client) vmprovider.VirtualMachineProviderInterface {
	vmProvider := &vSphereVmProvider{
		sessions: NewSessionManager(ncpClient, client),
	}

	return vmProvider
}

func NewVSphereMachineProviderFromRestConfig(cfg *rest.Config, client ctrlruntime.Client) vmprovider.VirtualMachineProviderInterface {
	ncpClient := ncpclientset.NewForConfigOrDie(cfg)
	return NewVSphereVmProviderFromClients(ncpClient, client)
}

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

func (vs *vSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	log.V(4).Info("Listing VirtualMachineImages", "namespace", namespace)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	if ses.contentlib != nil {
		// List images from Content Library
		imagesFromCL, err := ses.ListVirtualMachineImagesFromCL(ctx, ses.contentlib.ID)
		if err != nil {
			return nil, err
		}

		return imagesFromCL, nil
	}

	if ses.useInventoryForImages {
		// TODO(bryanv) Need an actual path here?
		resVms, err := ses.ListVirtualMachines(ctx, "*")
		if err != nil {
			return nil, transformVmImageError("", err)
		}

		var vmOpts OvfPropertyRetriever = vmOptions{}
		images := make([]*v1alpha1.VirtualMachineImage, 0, len(resVms))
		for _, resVm := range resVms {
			image, err := ResVmToVirtualMachineImage(ctx, resVm, AnnotateVmImage, vmOpts)
			if err != nil {
				return nil, err
			}

			images = append(images, image)
		}

		return images, nil
	}

	return nil, nil
}

func (vs *vSphereVmProvider) GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	vmName := fmt.Sprintf("%v/%v", namespace, name)
	log.V(4).Info("Getting VirtualMachineImage for ", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Find items in Library if Content Lib has been initialized
	if ses.contentlib != nil {
		image, err := ses.GetVirtualMachineImageFromCL(ctx, name)
		if err != nil {
			return nil, err
		}

		// If image is found return image or continue
		if image != nil {
			return image, nil
		}
	}

	if ses.useInventoryForImages {
		resVm, err := ses.lookupVmByName(ctx, name)
		if err != nil {
			return nil, transformVmImageError(vmName, err)
		}

		var vmOpts OvfPropertyRetriever = vmOptions{}
		return ResVmToVirtualMachineImage(ctx, resVm, AnnotateVmImage, vmOpts)
	}

	return nil, nil
}

func (vs *vSphereVmProvider) DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error) {
	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return false, err
	}

	if _, err = ses.GetVirtualMachine(ctx, vm); err != nil {
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
func AddProviderAnnotations(session *Session, objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {

	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	// Take missing annotations as a trigger to gather the information we need to populate the annotations.  We want to
	// avoid putting unnecessary pressure on content library.
	if _, ok := annotations[VmOperatorBiosUUIDKey]; !ok {
		biosUUID, err := vmRes.BiosUUID(context.Background())
		if err == nil {
			annotations[VmOperatorBiosUUIDKey] = biosUUID
		}
	}

	if _, ok := annotations[VmOperatorInstanceUUIDKey]; !ok {
		instanceUUID, err := vmRes.InstanceUUID(context.Background())
		if err == nil {
			annotations[VmOperatorInstanceUUIDKey] = instanceUUID
		}
	}

	if _, ok := annotations[VmOperatorVCInstanceUUIDKey]; !ok {
		about, err := session.ServiceContent(context.Background())
		if err == nil {
			annotations[VmOperatorVCInstanceUUIDKey] = about.InstanceUuid
		}
	}

	if _, ok := annotations[VmOperatorResourcePoolKey]; !ok {
		resourcePool, err := vmRes.ResourcePool(context.Background())
		if err == nil {
			annotations[VmOperatorResourcePoolKey] = resourcePool
		}
	}

	var vmOpts OvfPropertyRetriever = vmOptions{}
	err := AddVmImageAnnotations(annotations, context.Background(), vmOpts, vmRes)
	if err != nil {
		log.Error(err, "Error adding image annotations to VM", "vm", vmRes.Name)
	}

	objectMeta.SetAnnotations(annotations)
}

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

// DoesContentLiraryExists checks if a ContentLibrary exists on the vSphere infrastructure
// AKP: move to content provider
func (vs *vSphereVmProvider) DoesContentLibraryExist(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) (bool, error) {
	// GetSession with empty namespace grabs a cluster scoped session
	ses, err := vs.sessions.GetSession(ctx, "")
	if err != nil {
		return false, err
	}

	return ses.DoesContentLibraryExist(ctx, contentLibrary.Spec.UUID)
}

func (vs *vSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	createOpID := ctx.Value(vimtypes.ID{}).(string)
	ctx = context.WithValue(ctx, vimtypes.ID{}, createOpID+"-create-"+util.RandomString(pkg.OPIDLength))

	vmName := vm.NamespacedName()
	log.Info("Creating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.CloneVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		log.Error(err, "Create/Clone VirtualMachine failed", "name", vmName)
		return transformVmError(vmName, err)
	}

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) updatePowerState(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	// Default to on.
	powerState := v1alpha1.VirtualMachinePoweredOn
	if vm.Spec.PowerState != "" {
		powerState = vm.Spec.PowerState
	}

	if err := resVm.SetPowerState(ctx, powerState); err != nil {
		return errors.Wrapf(err, "failed to set power state to %v", powerState)
	}

	return nil
}

// UpdateVirtualMachine updates the VM status, power state, phase etc
func (vs *vSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	updateOpID := ctx.Value(vimtypes.ID{}).(string)
	ctx = context.WithValue(ctx, vimtypes.ID{}, updateOpID+"-update-"+util.RandomString(pkg.OPIDLength))

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
	resVm, err := session.updateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		return err
	}

	err = vs.attachTagsToVmAndAddToClusterModules(ctx, vm, vmConfigArgs.ResourcePolicy)
	if err != nil {
		return err
	}

	err = vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return err
	}

	AddProviderAnnotations(session, &vm.ObjectMeta, resVm)

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.NamespacedName()
	log.Info("Deleting VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vmToDelete.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vmToDelete)
	if err != nil {
		return transformVmError(vmName, err)
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vmToDelete, resVm)
	err = deleteSequence.Execute(ctx)
	if err != nil {
		log.Error(err, "Delete VirtualMachine sequence failed", "name", vmName)
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
	vmStatus.DeepCopyInto(&vm.Status)

	return nil
}

func (vs *vSphereVmProvider) GetClusterID(ctx context.Context, namespace string) (string, error) {
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

func (vs *vSphereVmProvider) UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error {
	return vs.sessions.UpdateVcPNID(ctx, clusterConfigMap)
}

func (vs *vSphereVmProvider) UpdateVmOpSACredSecret(ctx context.Context) {
	vs.sessions.clearSessionsAndClient(ctx)
}

func (vs *vSphereVmProvider) UpdateVmOpConfigMap(ctx context.Context) {
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

// For a given library item, convert its attributes to return a VirtualMachineImage that represents a k8s-native
// view of the item.
func LibItemToVirtualMachineImage(ctx context.Context, session *Session, item *library.Item, imgOptions ImageOptions, ovfPropRetriever OvfPropertyRetriever) (*v1alpha1.VirtualMachineImage, error) {

	var (
		ovfSystemProps = make(map[string]string)
		productInfo    = &v1alpha1.VirtualMachineImageProductInfo{}
		osInfo         = &v1alpha1.VirtualMachineImageOSInfo{}
	)

	if item.Type == library.ItemTypeOVF {
		ovfEnvelope, err := ovfPropRetriever.GetOvfInfoFromLibraryItem(ctx, session, item)
		if err != nil {
			return nil, err
		}

		if imgOptions == AnnotateVmImage {
			systemProps := GetVmwareSystemPropertiesFromOvf(ovfEnvelope)
			ovfSystemProps = systemProps
		}

		if ovfEnvelope.VirtualSystem != nil {
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
		}
	}

	var ts v1.Time
	if item.CreationTime != nil {
		ts = v1.NewTime(*item.CreationTime)
	}

	return &v1alpha1.VirtualMachineImage{
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
		},
	}, nil
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

func transformVmImageError(resource string, err error) error {
	return transformError("VirtualMachineImage", resource, err)
}

func IsCustomizationPendingError(err error) bool {
	if te, ok := err.(task.Error); ok {
		if _, ok := te.Fault().(*vimtypes.CustomizationPending); ok {
			return true
		}
	}
	return false
}
