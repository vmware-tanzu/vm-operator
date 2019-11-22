// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	vmopclientset "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/sequence"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
)

const (
	VsphereVmProviderName = "vsphere"

	// Annotation Key for vSphere MoRef
	VmOperatorMoRefKey          = pkg.VmOperatorKey + "/moref"
	VmOperatorVCInstanceUUIDKey = pkg.VmOperatorKey + "/vc-instance-uuid"
	VmOperatorInstanceUUIDKey   = pkg.VmOperatorKey + "/instance-uuid"
	VmOperatorBiosUUIDKey       = pkg.VmOperatorKey + "/bios-uuid"
	VmOperatorResourcePoolKey   = pkg.VmOperatorKey + "/resource-pool"

	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5
)

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

var log = logf.Log.WithName(VsphereVmProviderName)

type vSphereVmProvider struct {
	sessions SessionManager
}

func NewVSphereVmProviderFromClients(clientset *kubernetes.Clientset, ncpclient ncpclientset.Interface, vmopclient vmopclientset.Interface) VSphereVmProviderInterface {
	vmProvider := &vSphereVmProvider{
		sessions: NewSessionManager(clientset, ncpclient, vmopclient),
	}

	return vmProvider
}

func NewVsphereMachineProviderFromRestConfig(cfg *rest.Config) vmprovider.VirtualMachineProviderInterface {
	clientSet := kubernetes.NewForConfigOrDie(cfg)
	ncpclient := ncpclientset.NewForConfigOrDie(cfg)
	vmopclient := vmopclientset.NewForConfigOrDie(cfg)

	vmProvider := NewVsphereMachineProviderFromClients(clientSet, ncpclient, vmopclient)

	return vmProvider
}

func NewVsphereMachineProviderFromClients(clientset *kubernetes.Clientset, ncpclient ncpclientset.Interface, vmopclient vmopclientset.Interface) vmprovider.VirtualMachineProviderInterface {
	vSphereProvider := NewVSphereVmProviderFromClients(clientset, ncpclient, vmopclient)
	return vSphereProvider.(vmprovider.VirtualMachineProviderInterface)
}

func newVSphereVmProviderFromConfig(namespace string, config *VSphereVmProviderConfig) (VSphereVmProviderInterface, error) {
	vmProvider := &vSphereVmProvider{
		sessions: NewSessionManager(nil, nil, nil),
	}

	// Support existing behavior by setting up a Session for whatever namespace we're using.
	_, err := vmProvider.sessions.NewSession(namespace, config)
	if err != nil {
		return nil, err
	}

	return vmProvider, nil
}

// NewVSphereMachineProviderFromConfig only used in v1alpha1_suite_test.go
func NewVSphereMachineProviderFromConfig(namespace string, config *VSphereVmProviderConfig) (vmprovider.VirtualMachineProviderInterface, error) {
	vSphereProvider, err := newVSphereVmProviderFromConfig(namespace, config)

	if err != nil {
		return nil, err
	}

	return vSphereProvider.(vmprovider.VirtualMachineProviderInterface), nil
}

type VSphereVmProviderInterface interface {
	GetSession(ctx context.Context, namespace string) (*Session, error)
	UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error
	UpdateVmOpSACredSecret(ctx context.Context)
	DoClusterModulesExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	CreateClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	DeleteClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
}

func (vs *vSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *vSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *vSphereVmProvider) GetSession(ctx context.Context, namespace string) (*Session, error) {
	return vs.sessions.GetSession(ctx, namespace)
}

func (vs *vSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
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

func (vs *vSphereVmProvider) addProviderAnnotations(session *Session, objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {

	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	// Take missing annotations as a trigger to gather the information we need to populate the annotations.  We want to
	// avoid putting unnecessary pressure on VC.
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

	objectMeta.SetAnnotations(annotations)
}

// DoesVirtualMachineSetResourcePolicyExist checks if the entities of a VirtualMachineSetResourcePolicy exist on vSphere
func (vs *vSphereVmProvider) DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
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

	modulesExist, err := vs.DoClusterModulesExist(ctx, resourcePolicy)
	if err != nil {
		return false, err
	}

	return rpExists && folderExists && modulesExist, nil
}

// CreateOrUpdateVirtualMachineSetResourcePolicy creates if a VirtualMachineSetResourcePolicy doesn't exist, updates otherwise.
func (vs *vSphereVmProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
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

	moduleExists, err := vs.DoClusterModulesExist(ctx, resourcePolicy)
	if err != nil {
		return err
	}

	if !moduleExists {
		if err = vs.CreateClusterModules(ctx, resourcePolicy); err != nil {
			return err
		}
	}

	return nil
}

// DeleteVirtualMachineSetResourcePolicy deletes the VirtualMachineSetPolicy.
func (vs *vSphereVmProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
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

	if err = vs.DeleteClusterModules(ctx, resourcePolicy); err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {

	vmName := vm.NamespacedName()
	log.Info("Creating VirtualMachine", "name", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	// Determine if this is a clone or create from scratch.
	// The later is really only useful for dummy VMs at the moment.
	var resVm *res.VirtualMachine
	if vm.Spec.ImageName == "" {
		resVm, err = ses.CreateVirtualMachine(ctx, vm, vmConfigArgs)
	} else {
		resVm, err = ses.CloneVirtualMachine(ctx, vm, vmConfigArgs)
	}

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
	resVm, err := session.GetVirtualMachine(ctx, vm)
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
		deviceSpecs, err := session.GetNicChangeSpecs(ctx, vm, resVm)
		if err != nil {
			return err
		}

		configSpec, err := session.generateConfigSpec(vm.Name, &vm.Spec, &vmConfigArgs.VmClass.Spec, vmConfigArgs.VmMetadata, deviceSpecs)
		if err != nil {
			return err
		}

		err = resVm.Reconfigure(ctx, configSpec)
		if err != nil {
			return err
		}

		customizationSpec, err := session.getCustomizationSpec(vm.Namespace, vm.Name, &vm.Spec)
		if err != nil {
			return err
		}

		if customizationSpec != nil {
			log.Info("Customizing VM",
				"VirtualMachine", types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name},
				"CustomizationSpec", customizationSpec)
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

	err = vs.attachTagsToVmAndAddToClusterModules(ctx, vm, vmConfigArgs.ResourcePolicy)
	if err != nil {
		return err
	}

	err = vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return err
	}

	vs.addProviderAnnotations(session, &vm.ObjectMeta, resVm)

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
		if _, ok := te.Fault().(*vimtypes.CustomizationPending); ok {
			return true
		}
	}
	return false
}

// A helper function to check whether a given clusterModule has been created, and exists in VC.
func isClusterModulePresent(ctx context.Context, session *Session, moduleSpec v1alpha1.ClusterModuleSpec, moduleStatuses []v1alpha1.ClusterModuleStatus) (bool, error) {
	for _, module := range moduleStatuses {
		if module.GroupName == moduleSpec.GroupName {
			// If we find a match then we need to see whether the corresponding object exists in VC.
			moduleExists, err := session.DoesClusterModuleExist(ctx, module.ModuleUuid)
			if err != nil {
				return false, err
			} else {
				return moduleExists, nil
			}
		}
	}
	return false, nil
}

// DoClusterModulesExist checks whether all the ClusterModules for the given  VirtualMachineSetResourcePolicy has been
// created and exist in VC.
func (vs *vSphereVmProvider) DoClusterModulesExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return false, err
	}
	for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
		exists, err := isClusterModulePresent(ctx, ses, moduleSpec, resourcePolicy.Status.ClusterModules)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

// A helper function which adds or updates the status of a clusterModule to a given values. If the module with the
// same group name exists, its UUID is updated. Otherwise new ClusterModuleStatus is appended.
func updateOrAddClusterModuleStatus(new v1alpha1.ClusterModuleStatus, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) {
	for idx := range resourcePolicy.Status.ClusterModules {
		if resourcePolicy.Status.ClusterModules[idx].GroupName == new.GroupName {
			resourcePolicy.Status.ClusterModules[idx].ModuleUuid = new.ModuleUuid
			return
		}
	}
	resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules, new)
}

// CreateClusterModules creates all the ClusterModules that has not created yet for a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVmProvider) CreateClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return err
	}
	for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
		exists, err := isClusterModulePresent(ctx, ses, moduleSpec, resourcePolicy.Status.ClusterModules)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		id, err := ses.CreateClusterModule(ctx)
		if err != nil {
			return err
		}
		updateOrAddClusterModuleStatus(v1alpha1.ClusterModuleStatus{GroupName: moduleSpec.GroupName, ModuleUuid: id}, resourcePolicy)
	}
	return nil
}

func IsNotFoundError(err error) bool {
	return strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}

// DeleteClusterModules deletes all the ClusterModules associated with a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVmProvider) DeleteClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	ses, err := vs.sessions.GetSession(ctx, resourcePolicy.Namespace)
	if err != nil {
		return err
	}
	i := 0
	for _, moduleStatus := range resourcePolicy.Status.ClusterModules {
		err = ses.DeleteClusterModule(ctx, moduleStatus.ModuleUuid)
		// If the clusterModule has already been deleted, we can ignore the error and proceed.
		if err != nil && !IsNotFoundError(err) {
			break
		}
		i++
	}
	resourcePolicy.Status.ClusterModules = resourcePolicy.Status.ClusterModules[i:]
	if err != nil && !IsNotFoundError(err) {
		return err
	}
	return nil
}

func (vs *vSphereVmProvider) attachTagsToVmAndAddToClusterModules(ctx context.Context, vm *v1alpha1.VirtualMachine, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}
	log.V(4).Info("Attaching tags to vm", "name: ", vm.Name)
	resVm, err := ses.GetVirtualMachine(ctx, vm)
	if err != nil {
		return err
	}
	vmRef := &vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: resVm.ReferenceValue()}
	annotations := vm.ObjectMeta.GetAnnotations()

	// We require both the clusterModule information and tag information to be able to enforce the vm-vm anti-affinity policy.
	if annotations[pkg.ClusterModuleNameKey] != "" && annotations[pkg.ProviderTagsAnnotationKey] != "" {
		// Find ClusterModule from resourcePolicy
		var moduleUuid string
		for _, clusterModule := range resourcePolicy.Status.ClusterModules {
			if clusterModule.GroupName == annotations[pkg.ClusterModuleNameKey] {
				moduleUuid = clusterModule.ModuleUuid
			}
		}
		if moduleUuid == "" {
			return errors.New("Unable to find the clusterModule to attach")
		}

		isMember, err := ses.IsVmMemberOfClusterModule(ctx, moduleUuid, vmRef)
		if err != nil {
			return err
		}
		if !isMember {
			if err = ses.AddVmToClusterModule(ctx, moduleUuid, vmRef); err != nil {
				return err
			}
		}

		// Lookup the real tag name from config and attach to the VM.
		tagName := ses.tagInfo[annotations[pkg.ProviderTagsAnnotationKey]]
		err = ses.AttachTagToVm(ctx, tagName, pkg.ProviderTagCategoryName, resVm)
		if err != nil {
			return err
		}
	}

	return nil
}
