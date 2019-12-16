/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"k8s.io/client-go/rest"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/types"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/klogr"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/sequence"
)

const (
	VsphereVmProviderName = "vsphere"

	// Annotation Key for vSphere MoRef
	VmOperatorMoRefKey = pkg.VmOperatorKey + "/moref"

	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5
)

type VSphereVmProvider struct {
	sessions SessionManager
}

var _ vmprovider.VirtualMachineProviderInterface = &VSphereVmProvider{}

type OvfPropertyRetriever interface {
	FetchOvfPropertiesFromLibrary(ctx context.Context, ses *Session, item *library.Item) (map[string]string, error)
	FetchOvfPropertiesFromVM(ctx context.Context, resVm *res.VirtualMachine) (map[string]string, error)
}

type vmOptions struct{}

var _ OvfPropertyRetriever = vmOptions{}

type ImageOptions int

const (
	AnnotateVmImage ImageOptions = iota
	DoNotAnnotateVmImage
)

var log = klogr.New()

func NewVSphereVmProvider(clientset *kubernetes.Clientset, ncpclient ncpclientset.Interface) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(clientset, ncpclient),
	}

	return vmProvider, nil
}

func (vs *VSphereVmProvider) RegisterSession(namespace string, config *VSphereVmProviderConfig) error {

	// Support existing behavior by setting up a Session for whatever namespace we're using. This is
	// used in the integration tests.
	_, err := vs.sessions.NewSession(namespace, config)
	return err
}

func RegisterVsphereVmProvider(restConfig *rest.Config) (vmprovider.VirtualMachineProviderInterface, error) {
	clientSet := kubernetes.NewForConfigOrDie(restConfig)
	ncpclient := ncpclientset.NewForConfigOrDie(restConfig)

	vsphereProvider, err := NewVSphereVmProvider(clientSet, ncpclient)
	if err != nil {
		return nil, err
	}

	provider := vmprovider.RegisterVmProviderOrDie(vsphereProvider)
	return provider, nil
}

func UnregisterVsphereVmProvider(vmProvider vmprovider.VirtualMachineProviderInterface) {
	vmprovider.UnregisterVmProviderOrDie(vmProvider)
}

// NewVSphereVmProviderFromConfig is only used in the integration tests.
func NewVSphereVmProviderFromConfig(namespace string, config *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(nil, nil),
	}

	// Support existing behavior by setting up a Session for whatever namespace we're using.
	_, err := vmProvider.sessions.NewSession(namespace, config)
	if err != nil {
		return nil, err
	}

	return vmProvider, nil
}

func (vs *VSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *VSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) GetSession(ctx context.Context, namespace string) (*Session, error) {
	return vs.sessions.GetSession(ctx, namespace)
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	log.V(4).Info("Listing VirtualMachineImages", "namespace", namespace)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	if ses.contentlib != nil {
		// List images from Content Library
		imagesFromCL, err := ses.ListVirtualMachineImagesFromCL(ctx)
		if err != nil {
			return nil, err
		}

		return imagesFromCL, nil
	}

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

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
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

	resVm, err := ses.GetVirtualMachine(ctx, name)
	if err != nil {
		return nil, transformVmImageError(vmName, err)
	}

	var vmOpts OvfPropertyRetriever = vmOptions{}
	return ResVmToVirtualMachineImage(ctx, resVm, AnnotateVmImage, vmOpts)
}

func (vs *VSphereVmProvider) DoesVirtualMachineExist(ctx context.Context, namespace, name string) (bool, error) {
	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return false, err
	}

	if _, err = ses.GetVirtualMachine(ctx, name); err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	objectMeta.SetAnnotations(annotations)
}

// DoesVirtualMachineSetResourcePolicyExist checks if the entities of a VirtualMachineSetResourcePolicy exists on vSphere
func (vs *VSphereVmProvider) DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return false, err
	}

	rpExists, err := ses.DoesResourcePoolExist(ctx, resourcePolicy.Namespace, resourcePolicy.Spec.ResourcePool.Name)
	if err != nil {
		return false, err
	}

	folderExists, err := ses.DoesFolderExist(ctx, resourcePolicy.Namespace, resourcePolicy.Spec.Folder.Name)
	if err != nil {
		return false, err
	}

	return rpExists && folderExists, nil
}

// CreateOrUpdateVirtualMachineSetResourcePolicy creates if a VirtualMachineSetResourcePolicy doesn't exist, updates otherwise.
func (vs *VSphereVmProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return err
	}

	rpExists, err := ses.DoesResourcePoolExist(ctx, resourcePolicy.Namespace, resourcePolicy.Spec.ResourcePool.Name)
	if err != nil {
		return err
	}

	if !rpExists {
		if _, err = ses.CreateResourcePool(ctx, &resourcePolicy.Spec.ResourcePool); err != nil {
			return err
		}
	} else {
		if err = ses.UpdateResourcePool(ctx, &resourcePolicy.Spec.ResourcePool); err != nil {
			return err
		}
	}

	folderExists, err := ses.DoesFolderExist(ctx, resourcePolicy.Namespace, resourcePolicy.Spec.Folder.Name)
	if err != nil {
		return err
	}

	if !folderExists {
		if _, err = ses.CreateFolder(ctx, &resourcePolicy.Spec.Folder); err != nil {
			return err
		}
	}

	return nil
}

// DeleteVirtualMachineSetResourcePolicy deletes the VirtualMachineSetPolicy.
func (vs *VSphereVmProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return err
	}

	if err = ses.DeleteResourcePool(ctx, resourcePolicy.Spec.ResourcePool.Name); err != nil {
		return err
	}

	if err = ses.DeleteFolder(ctx, resourcePolicy.Spec.Folder.Name); err != nil {
		return err
	}

	return nil
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	vmClass v1alpha1.VirtualMachineClass, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy,
	vmMetadata vmprovider.VirtualMachineMetadata, storageProfileID string) error {

	vmName := vm.NamespacedName()
	log.Info("Creating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	// Determine if this is a clone or create from scratch.
	// The later is really only useful for dummy VMs at the moment.
	if vm.Spec.ImageName == "" {
		_, err = ses.CreateVirtualMachine(ctx, vm, vmClass, resourcePolicy, vmMetadata)
	} else {
		_, err = ses.CloneVirtualMachine(ctx, vm, vmClass, resourcePolicy, vmMetadata, storageProfileID)
	}

	if err != nil {
		log.Error(err, "Create/Clone VirtualMachine failed", "name", vmName)
		return transformVmError(vmName, err)
	}

	return nil
}

func (vs *VSphereVmProvider) updatePowerState(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
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
func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata) error {
	vmName := vm.NamespacedName()
	log.Info("Updating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	err = vs.updateVirtualMachine(ctx, ses, vm, vmClass, vmMetadata)
	if err != nil {
		return transformVmError(vmName, err)
	}

	return nil
}

func (vs *VSphereVmProvider) updateVirtualMachine(ctx context.Context, ses *Session, vm *v1alpha1.VirtualMachine, vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata) error {
	resVm, err := ses.GetVirtualMachine(ctx, vm.Name)
	if err != nil {
		return err
	}

	isOff, err := resVm.IsVMPoweredOff(ctx)
	if err != nil {
		return err
	}

	// This is just a horrible, temporary hack so that we reconfigure "once" and not disrupt a running VM.
	if isOff {
		// Add device change specs to configSpec
		deviceSpecs, err := ses.GetNicChangeSpecs(ctx, vm, resVm)
		if err != nil {
			return err
		}

		configSpec, err := ses.generateConfigSpec(vm.Name, &vm.Spec, &vmClass.Spec, vmMetadata, deviceSpecs)
		if err != nil {
			return err
		}

		err = resVm.Reconfigure(ctx, configSpec)
		if err != nil {
			return err
		}

		customizationSpec, err := ses.getCustomizationSpec(vm.Namespace, vm.Name, &vm.Spec)
		if err != nil {
			return err
		}

		if customizationSpec != nil {
			if err := resVm.Customize(ctx, *customizationSpec); err != nil {
				// Ignore customization pending fault as this means we have already tried to customize the VM and it is
				// pending. This can happen if the VM has failed to power-on since the last time we customized the VM. If
				// we don't ignore this error, we will never be able to power-on the VM and the we will always fail here.
				if !IsCustomizationPendingError(err) {
					return err
				}
				log.Info("Ignoring customization error due to pending guest customization", "name", vm.NamespacedName())
			}
		}
	}

	err = vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return err
	}

	vs.addProviderAnnotations(&vm.ObjectMeta, resVm)

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return err
	}

	return nil
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.NamespacedName()
	log.Info("Deleting VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vmToDelete.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vmToDelete.Name)
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
func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	vmStatus, err := resVm.GetStatus(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to get VirtualMachine status")
	}

	// BMV: This just ain't right.
	vmStatus.Volumes = vm.Status.Volumes
	vmStatus.DeepCopyInto(&vm.Status)

	return nil
}

func (vs *VSphereVmProvider) GetClusterID(ctx context.Context, namespace string) (string, error) {
	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return "", err
	}
	if ses.cluster == nil {
		return "", errors.Errorf("no cluster exists")
	}
	return ses.cluster.Reference().Value, nil
}

func (vs *VSphereVmProvider) ComputeClusterCpuMinFrequency(ctx context.Context) error {
	if err := vs.sessions.ComputeClusterCpuMinFrequency(ctx); err != nil {
		return err
	}

	return nil
}

func ResVmToVirtualMachineImage(ctx context.Context, resVm *res.VirtualMachine, imgOptions ImageOptions, vmProvider OvfPropertyRetriever) (*v1alpha1.VirtualMachineImage, error) {
	powerState, uuid, reference := resVm.ImageFields(ctx)

	var ovfProperties map[string]string

	if imgOptions == AnnotateVmImage {
		var err error
		ovfProperties, err = vmProvider.FetchOvfPropertiesFromVM(ctx, resVm)
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

func LibItemToVirtualMachineImage(ctx context.Context, sess *Session, item *library.Item, imgOptions ImageOptions, vmProvider OvfPropertyRetriever) (*v1alpha1.VirtualMachineImage, error) {

	var ovfProperties map[string]string

	if imgOptions == AnnotateVmImage {
		var err error
		ovfProperties, err = vmProvider.FetchOvfPropertiesFromLibrary(ctx, sess, item)
		if err != nil {
			return nil, err
		}
	}

	var ts v1.Time
	if item.CreationTime != nil {
		ts = v1.NewTime(*item.CreationTime)
	}
	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:              item.Name,
			Annotations:       ovfProperties,
			CreationTimestamp: ts,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            item.Type,
			ImageSourceType: "Content Library",
		},
	}, nil

}

func (vm vmOptions) FetchOvfPropertiesFromLibrary(ctx context.Context, ses *Session, item *library.Item) (map[string]string, error) {
	contentLibSession := NewContentLibraryProvider(ses)

	clDownloadHandler := createClDownloadHandler()

	// Fetch & parse ovf from CL and populate the properties as annotations
	ovfProperties, err := contentLibSession.ParseAndRetrievePropsFromLibraryItem(ctx, item, clDownloadHandler)
	if err != nil {
		return nil, err
	}

	return ovfProperties, nil
}

func (vm vmOptions) FetchOvfPropertiesFromVM(ctx context.Context, resVm *res.VirtualMachine) (map[string]string, error) {
	return resVm.GetOvfProperties(ctx)
}

func createClDownloadHandler() ContentDownloadHandler {
	// Integration test environment would require a much lesser wait time
	// BMV: This envvar is never set.
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
		return k8serror.NewNotFound(vmoperator.Resource(resourceType), resource)
	case *find.MultipleFoundError, *find.DefaultMultipleFoundError:
		// Transform?
		return err
	default:
		return err
	}
}

func transformVmError(resource string, err error) error {
	return transformError(vmoperator.InternalVirtualMachine.GetKind(), resource, err)
}

func transformVmImageError(resource string, err error) error {
	return transformError(vmoperator.InternalVirtualMachineImage.GetKind(), resource, err)
}

func IsCustomizationPendingError(err error) bool {
	if te, ok := err.(task.Error); ok {
		if _, ok := te.Fault().(*types.CustomizationPending); ok {
			return true
		}
	}
	return false
}
