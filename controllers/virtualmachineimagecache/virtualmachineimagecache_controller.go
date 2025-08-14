// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimagecache

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimagecache/internal"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	clprov "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineImageCache{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := &reconciler{
		Context:    ctx,
		Client:     mgr.GetClient(),
		Logger:     ctx.Logger.WithName("controllers").WithName(controlledTypeName),
		Recorder:   record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		VMProvider: ctx.VMProvider,

		newCLSProvdrFn: newContentLibraryProviderOrDefault(ctx),
		newSRIClientFn: newCacheStorageURIsClientOrDefault(ctx),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			SkipNameValidation: SkipNameValidation,
			LogConstructor:     pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		WatchesRawSource(source.Channel(
			cource.FromContextWithBuffer(ctx, "VirtualMachineImageCache", 100),
			&handler.EnqueueRequestForObject{})).
		Complete(r)
}

// reconciler reconciles a VirtualMachineImageCache object.
type reconciler struct {
	ctrlclient.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface

	newCLSProvdrFn newContentLibraryProviderFn
	newSRIClientFn newCacheStorageURIsClientFn
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimagecaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimagecaches/status,verbs=get;update;patch

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj vmopv1.VirtualMachineImageCache
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for %s: %w", req.NamespacedName, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, &obj); err != nil {
			if reterr == nil {
				reterr = err
			}
			pkgutil.FromContextOrDefault(ctx).Error(err, "patch failed")
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		// Noop.
		return ctrl.Result{}, nil
	}

	return pkgerr.ResultFromError(r.ReconcileNormal(ctx, &obj))
}

const conditionReasonFailed = "Failed"

func (r *reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vmopv1.VirtualMachineImageCache) (retErr error) {

	// Reset the version status so it is constructed from scratch each time.
	obj.Status = vmopv1.VirtualMachineImageCacheStatus{}

	// If the reconcile failed with an error, then make sure it is reflected in
	// the object's Ready condition.
	defer func() {
		if retErr != nil {
			pkgcond.MarkError(
				obj,
				vmopv1.ReadyConditionType,
				conditionReasonFailed,
				retErr)
		}
	}()

	// Verify the item's ID.
	if obj.Spec.ProviderID == "" {
		return pkgerr.NoRequeueError{Message: "spec.providerID is empty"}
	}

	// Is this an OVF-backed image?
	isOVF := !strings.HasPrefix(obj.Spec.ProviderID, "vm-")

	// Verify the item's version.
	if isOVF && obj.Spec.ProviderVersion == "" {
		return pkgerr.NoRequeueError{Message: "spec.providerVersion is empty"}
	}

	logger := pkgutil.FromContextOrDefault(ctx).WithValues(
		"providerID", obj.Spec.ProviderID,
		"providerVersion", obj.Spec.ProviderVersion)
	ctx = logr.NewContext(ctx, logger)

	// Get a vSphere client.
	c, err := r.VMProvider.VSphereClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get vSphere client: %w", err)
	}

	// Get the content library provider.
	var clProv clprov.Provider
	if isOVF {
		clProv = r.newCLSProvdrFn(ctx, c.RestClient())
	}

	// Reconcile the hardware.
	if err := reconcileHardware(ctx, r.Client, clProv, obj, isOVF); err != nil {
		pkgcond.MarkError(
			obj,
			vmopv1.VirtualMachineImageCacheConditionHardwareReady,
			conditionReasonFailed,
			err)
	} else {
		pkgcond.MarkTrue(
			obj,
			vmopv1.VirtualMachineImageCacheConditionHardwareReady)
	}

	if len(obj.Spec.Locations) > 0 {
		// Reconcile the underlying provider.
		if err := reconcileProvider(ctx, clProv, obj, isOVF); err != nil {
			pkgcond.MarkError(
				obj,
				vmopv1.VirtualMachineImageCacheConditionProviderReady,
				conditionReasonFailed,
				err)
		} else {
			pkgcond.MarkTrue(
				obj,
				vmopv1.VirtualMachineImageCacheConditionProviderReady)
		}

		// Reconcile the files.
		if err := r.reconcileFiles(ctx, c, clProv, obj, isOVF); err != nil {
			pkgcond.MarkError(
				obj,
				vmopv1.VirtualMachineImageCacheConditionFilesReady,
				conditionReasonFailed,
				err)
		} else {
			// Aggregate each location's Ready condition into the top-level
			// VirtualMachineImageCacheConditionFilesReady condition.
			getters := make([]pkgcond.Getter, len(obj.Status.Locations))
			for i := range obj.Status.Locations {
				getters[i] = obj.Status.Locations[i]
			}
			pkgcond.SetAggregate(
				obj,
				vmopv1.VirtualMachineImageCacheConditionFilesReady,
				getters,
				pkgcond.WithStepCounter())
		}
	}

	// Create the object's Ready condition based on its other conditions.
	pkgcond.SetSummary(obj, pkgcond.WithStepCounter())

	return nil
}

func (r *reconciler) reconcileFiles(
	ctx context.Context,
	vcClient *client.Client,
	clProv clprov.Provider,
	obj *vmopv1.VirtualMachineImageCache,
	isOVF bool) error {

	var (
		srcDatacenter = vcClient.Datacenter()
		vimClient     = vcClient.VimClient()
	)

	// Get the library item's storage paths.
	srcFiles, err := getSourceFilePaths(
		ctx,
		vcClient.VimClient(),
		clProv,
		srcDatacenter,
		obj.Spec.ProviderID,
		isOVF)
	if err != nil {
		return err
	}

	// Get the datacenters used by the item.
	dstDatacenters, err := getDatacenters(ctx, vimClient, obj)
	if err != nil {
		return err
	}

	// Get the datastores used by the item.
	dstDatastores, err := getDatastores(ctx, vimClient, obj)
	if err != nil {
		return err
	}

	// Reconcile the locations.
	r.reconcileLocations(
		ctx,
		vimClient,
		dstDatacenters,
		srcDatacenter,
		dstDatastores,
		obj,
		srcFiles)

	return nil
}

func (r *reconciler) reconcileLocations(
	ctx context.Context,
	vimClient *vim25.Client,
	dstDatacenters map[string]*object.Datacenter,
	srcDatacenter *object.Datacenter,
	dstDatastores map[string]datastore,
	obj *vmopv1.VirtualMachineImageCache,
	srcFiles []clsutil.SourceFile) {

	obj.Status.Locations = make(
		[]vmopv1.VirtualMachineImageCacheLocationStatus,
		len(obj.Spec.Locations))

	for i := range obj.Spec.Locations {

		var (
			spec       = obj.Spec.Locations[i]
			status     = &obj.Status.Locations[i]
			conditions = pkgcond.Conditions(status.Conditions)
		)

		status.DatacenterID = spec.DatacenterID
		status.DatastoreID = spec.DatastoreID
		status.ProfileID = spec.ProfileID

		// Get the preferred disk format for the datastore.
		dstDiskFormat := pkgutil.GetPreferredDiskFormat(
			dstDatastores[spec.DatastoreID].mo.Info.
				GetDatastoreInfo().SupportedVDiskFormats...)

		// Update the srcFiles elements with the profile and format info.
		for i := range srcFiles {
			srcFiles[i].DstProfileID = spec.ProfileID
			srcFiles[i].DstDiskFormat = dstDiskFormat
		}

		cachedFiles, err := r.cacheFiles(
			ctx,
			vimClient,
			dstDatacenters[spec.DatacenterID],
			srcDatacenter,
			dstDatastores[spec.DatastoreID].mo.Name,
			obj.Name,
			obj.Spec.ProviderVersion,
			srcFiles)
		if err != nil {
			conditions = conditions.MarkError(
				vmopv1.ReadyConditionType,
				conditionReasonFailed,
				err)
		} else {
			status.Files = cachedFiles
			conditions = conditions.MarkTrue(vmopv1.ReadyConditionType)
		}

		status.Conditions = conditions
	}
}

func (r *reconciler) cacheFiles(
	ctx context.Context,
	vimClient *vim25.Client,
	dstDatacenter, srcDatacenter *object.Datacenter,
	dstDatastoreName, itemName, itemVersion string,
	srcFiles []clsutil.SourceFile) ([]vmopv1.VirtualMachineImageCacheFileStatus, error) {

	// Update the srcFiles elements with the directory in which each file is
	// cached.
	sriClient := r.newSRIClientFn(vimClient)
	for i := range srcFiles {
		cacheDir := clsutil.GetCacheDirectory(
			dstDatastoreName,
			itemName,
			srcFiles[i].DstProfileID,
			itemVersion)
		srcFiles[i].DstDir = cacheDir
	}

	logger := pkgutil.FromContextOrDefault(ctx)
	logger.V(4).Info("Caching files",
		"dstDatacenter", dstDatacenter.Reference().Value,
		"srcDatacenter", srcDatacenter.Reference().Value,
		"srcFiles", srcFiles)

	cachedFiles, err := clsutil.CacheStorageURIs(
		ctx,
		sriClient,
		dstDatacenter,
		srcDatacenter,
		srcFiles...)
	if err != nil {
		return nil, fmt.Errorf("failed to cache storage items: %w", err)
	}

	cachedFileStatuses := make(
		[]vmopv1.VirtualMachineImageCacheFileStatus, len(cachedFiles))

	for i := range cachedFiles {
		if v := cachedFiles[i].Path; v != "" {
			cachedFileStatuses[i].ID = v
			if strings.EqualFold(".vmdk", path.Ext(v)) {
				cachedFileStatuses[i].Type = vmopv1.VirtualMachineImageCacheFileTypeDisk
				cachedFileStatuses[i].DiskType = vmopv1.VirtualMachineStorageDiskTypeClassic
			} else {
				cachedFileStatuses[i].Type = vmopv1.VirtualMachineImageCacheFileTypeOther
			}
		} else {
			cachedFileStatuses[i].ID = cachedFiles[i].VDiskID
			cachedFileStatuses[i].Type = vmopv1.VirtualMachineImageCacheFileTypeDisk
			cachedFileStatuses[i].DiskType = vmopv1.VirtualMachineStorageDiskTypeManaged
		}
	}

	return cachedFileStatuses, nil
}

func reconcileHardware(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	clProv clprov.Provider,
	obj *vmopv1.VirtualMachineImageCache,
	isOVF bool) error {

	if isOVF {
		return reconcileOVF(ctx, k8sClient, clProv, obj)
	}

	return nil
}

const (
	ovfConfigMapValueKey          = "value"
	ovfConfigMapContentVersionKey = "contentVersion"
)

func reconcileOVF(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	clProv clprov.Provider,
	obj *vmopv1.VirtualMachineImageCache) error {

	// Ensure the OVF ConfigMap is up-to-date. Please note, this may be a no-op.
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      obj.Name,
		},
	}
	if _, err := controllerutil.CreateOrPatch(
		ctx,
		k8sClient,
		&configMap,
		func() error {

			// Make the VMI Cache object own the ConfigMap.
			if err := controllerutil.SetControllerReference(
				obj,
				&configMap,
				k8sClient.Scheme()); err != nil {

				return err
			}

			if configMap.Data[ovfConfigMapValueKey] != "" &&
				configMap.Data[ovfConfigMapContentVersionKey] == obj.Spec.ProviderVersion {
				// Do nothing if the ConfigMap has the marshaled OVF and it is
				// the latest content version.
				return nil
			}

			if configMap.Data == nil {
				configMap.Data = map[string]string{}
			}

			logger := pkgutil.FromContextOrDefault(ctx)
			logger.Info("Fetching OVF")

			// Get the OVF.
			ovfEnv, err := clProv.RetrieveOvfEnvelopeByLibraryItemID(
				ctx, obj.Spec.ProviderID)
			if err != nil {
				return fmt.Errorf("failed to retrieve ovf envelope: %w", err)
			}

			// Marshal the OVF envelope to YAML.
			data, err := yaml.Marshal(ovfEnv)
			if err != nil {
				return fmt.Errorf("failed to marshal ovf envelope to YAML: %w", err)
			}

			configMap.Data[ovfConfigMapContentVersionKey] = obj.Spec.ProviderVersion
			configMap.Data[ovfConfigMapValueKey] = string(data)

			return nil
		}); err != nil {

		return fmt.Errorf("failed to create or patch ovf configmap: %w", err)
	}

	if obj.Status.OVF == nil {
		obj.Status.OVF = &vmopv1.VirtualMachineImageCacheOVFStatus{}
	}

	obj.Status.OVF.ProviderVersion = obj.Spec.ProviderVersion
	obj.Status.OVF.ConfigMapName = configMap.Name

	return nil
}

func reconcileProvider(
	ctx context.Context,
	p clprov.Provider,
	obj *vmopv1.VirtualMachineImageCache,
	isOVF bool) error {

	if isOVF {
		return reconcileLibraryItem(ctx, p, obj)
	}

	return nil
}

func reconcileLibraryItem(
	ctx context.Context,
	p clprov.Provider,
	obj *vmopv1.VirtualMachineImageCache) error {

	logger := pkgutil.FromContextOrDefault(ctx)

	// Get the content library item to be cached.
	item, err := p.GetLibraryItemID(ctx, obj.Spec.ProviderID)
	if err != nil {
		return fmt.Errorf("failed to get library item: %w", err)
	}

	// If the item is not cached locally, then issue a sync so content library
	// fetches the item's disks.
	//
	// Please note, the m.SyncLibraryItem method is reentrant on the remote
	// side. That is to say, if the call gets interrupted and we sync the item
	// again while an existing sync is occurring, the client will block until
	// the original sync is complete.
	if !item.Cached {
		logger.Info("Syncing library item")
		if err := p.SyncLibraryItem(ctx, item, true); err != nil {
			return fmt.Errorf("failed to sync library item: %w", err)
		}
	}

	return nil
}

type datastore struct {
	datacenterID string
	mo           mo.Datastore
	obj          *object.Datastore
}

func includeItemFile(s string) bool {
	switch strings.ToLower(path.Ext(s)) {
	case ".vmdk", ".nvram":
		return true
	default:
		return false
	}
}

func getSourceFilePaths(
	ctx context.Context,
	c *vim25.Client,
	p clprov.Provider,
	datacenter *object.Datacenter,
	itemID string,
	isOVF bool) ([]clsutil.SourceFile, error) {

	if isOVF {
		return getSourceFilePathsForOVF(ctx, p, datacenter, itemID)
	}

	return getSourceFilePathsForVM(ctx, c, itemID)
}

func getSourceFilePathsForOVF(
	ctx context.Context,
	p clprov.Provider,
	datacenter *object.Datacenter,
	itemID string) ([]clsutil.SourceFile, error) {

	// Get the storage URIs for the library item's files.
	itemStor, err := p.ListLibraryItemStorage(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to list library item storage: %w", err)
	}

	// Resolve the item's storage URIs into datastore paths, ex.
	// [my-datastore] path/to/file.ext
	if err := p.ResolveLibraryItemStorage(
		ctx,
		datacenter,
		itemStor); err != nil {

		return nil, fmt.Errorf("failed to resolve library item storage: %w", err)
	}

	// Get the storage URIs for just vmdk and NVRAM files.
	var srcFiles []clsutil.SourceFile
	for i := range itemStor {
		is := itemStor[i]
		for j := range is.StorageURIs {
			s := is.StorageURIs[j]
			if includeItemFile(s) {
				srcFiles = append(srcFiles, clsutil.SourceFile{
					Path: s,
				})
			}
		}
	}

	return srcFiles, nil
}

func getSourceFilePathsForVM(
	ctx context.Context,
	c *vim25.Client,
	itemID string) ([]clsutil.SourceFile, error) {

	vm := object.NewVirtualMachine(c, vimtypes.ManagedObjectReference{
		Type:  string(vimtypes.ManagedObjectTypesVirtualMachine),
		Value: itemID,
	})

	var moVM mo.VirtualMachine
	if err := vm.Properties(
		ctx,
		vm.Reference(),
		[]string{"config.hardware.device", "layoutEx"},
		&moVM); err != nil {

		return nil, fmt.Errorf(
			"failed to get props for vm while getting source file paths: %w",
			err)
	}

	if moVM.Config == nil {
		return nil, fmt.Errorf("failed to get vm property %q", "config")
	}
	if moVM.LayoutEx == nil {
		return nil, fmt.Errorf("failed to get vm property %q", "layoutEx")
	}

	var srcFiles []clsutil.SourceFile

	// Get the cacheable disks.
	for _, bd := range moVM.Config.Hardware.Device {
		if disk, ok := bd.(*vimtypes.VirtualDisk); ok {
			var sf clsutil.SourceFile

			if disk.VDiskId != nil {
				sf.VDiskID = disk.VDiskId.Id
			}

			switch tb := disk.Backing.(type) {
			case *vimtypes.VirtualDiskFlatVer1BackingInfo:
				sf.Path = tb.FileName
			case *vimtypes.VirtualDiskFlatVer2BackingInfo:
				sf.Path = tb.FileName
			case *vimtypes.VirtualDiskSeSparseBackingInfo:
				sf.Path = tb.FileName
			case *vimtypes.VirtualDiskSparseVer1BackingInfo:
				sf.Path = tb.FileName
			case *vimtypes.VirtualDiskSparseVer2BackingInfo:
				sf.Path = tb.FileName
			}

			if sf.Path != "" {
				srcFiles = append(srcFiles, sf)
			}
		}
	}

	// Get the NVRAM file.
	for i := range moVM.LayoutEx.File {
		f := moVM.LayoutEx.File[i]
		if strings.EqualFold(path.Ext(f.Name), ".nvram") {
			srcFiles = append(srcFiles, clsutil.SourceFile{
				Path: f.Name,
			})
		}
	}

	return srcFiles, nil
}

func getDatacenters(
	ctx context.Context,
	vimClient *vim25.Client,
	obj *vmopv1.VirtualMachineImageCache) (map[string]*object.Datacenter, error) {

	objMap := map[string]*object.Datacenter{}

	// Get a set of unique datacenters used by the item's storage.
	for i := range obj.Spec.Locations {
		l := obj.Spec.Locations[i]
		if _, ok := objMap[l.DatacenterID]; !ok {
			ref := vimtypes.ManagedObjectReference{
				Type:  "Datacenter",
				Value: l.DatacenterID,
			}

			var err error
			dc := object.NewDatacenter(vimClient, ref)
			// Needed for dcPath param to NewDatastoreURL
			dc.InventoryPath, err = find.InventoryPath(ctx, vimClient, ref)
			if err != nil {
				var f *vimtypes.ManagedObjectNotFound
				if _, ok := fault.As(err, &f); ok {
					return nil, fmt.Errorf("invalid datacenter ID: %s", f.Obj.Value)
				}

				return nil, fmt.Errorf("failed to get datacenter properties: %w", err)
			}

			objMap[l.DatacenterID] = dc
		}
	}

	return objMap, nil
}

func getDatastores(
	ctx context.Context,
	vimClient *vim25.Client,
	obj *vmopv1.VirtualMachineImageCache) (map[string]datastore, error) {

	var (
		refList []vimtypes.ManagedObjectReference
		objMap  = map[string]datastore{}
	)

	// Get a set of unique datastores used by the item's storage.
	for i := range obj.Spec.Locations {
		l := obj.Spec.Locations[i]
		if _, ok := objMap[l.DatastoreID]; !ok {
			ref := vimtypes.ManagedObjectReference{
				Type:  "Datastore",
				Value: l.DatastoreID,
			}
			objMap[l.DatastoreID] = datastore{
				datacenterID: l.DatacenterID,
				mo: mo.Datastore{
					ManagedEntity: mo.ManagedEntity{
						ExtensibleManagedObject: mo.ExtensibleManagedObject{
							Self: ref,
						},
					},
				},
				obj: object.NewDatastore(vimClient, ref),
			}
			refList = append(refList, ref)
		}
	}

	var (
		moList []mo.Datastore
		pc     = property.DefaultCollector(vimClient)
	)

	// Populate the properties of the unique datastores.
	if err := pc.Retrieve(
		ctx,
		refList,
		[]string{"name", "info"},
		&moList); err != nil {

		var f *vimtypes.ManagedObjectNotFound
		if _, ok := fault.As(err, &f); ok {
			return nil, fmt.Errorf("invalid datastore ID: %s", f.Obj.Value)
		}

		return nil, fmt.Errorf("failed to get datastore properties: %w", err)
	}

	for i := range moList {
		v := moList[i].Reference().Value
		o := objMap[v]
		o.mo = moList[i]
		objMap[v] = o
	}

	return objMap, nil
}

type newContentLibraryProviderFn = func(context.Context, *rest.Client) clprov.Provider
type newCacheStorageURIsClientFn = func(*vim25.Client) clsutil.CacheStorageURIsClient

func newContentLibraryProviderOrDefault(
	ctx context.Context) newContentLibraryProviderFn {

	out := clprov.NewProvider
	obj := ctx.Value(internal.NewContentLibraryProviderContextKey)
	if fn, ok := obj.(newContentLibraryProviderFn); ok {
		out = func(ctx context.Context, c *rest.Client) clprov.Provider {
			if p := fn(ctx, c); p != nil {
				return p
			}
			return clprov.NewProvider(ctx, c)
		}
	}
	return out
}

func newCacheStorageURIsClientOrDefault(
	ctx context.Context) newCacheStorageURIsClientFn {

	out := newCacheStorageURIsClient
	obj := ctx.Value(internal.NewCacheStorageURIsClientContextKey)
	if fn, ok := obj.(newCacheStorageURIsClientFn); ok {
		out = func(c *vim25.Client) clsutil.CacheStorageURIsClient {
			if p := fn(c); p != nil {
				return p
			}
			return newCacheStorageURIsClient(c)
		}
	}
	return out
}

func newCacheStorageURIsClient(c *vim25.Client) clsutil.CacheStorageURIsClient {
	return &cacheStorageURIsClient{
		FileManager:        object.NewFileManager(c),
		VirtualDiskManager: object.NewVirtualDiskManager(c),
	}
}

type cacheStorageURIsClient struct {
	*object.FileManager
	*object.VirtualDiskManager
}

func (c *cacheStorageURIsClient) DatastoreFileExists(
	ctx context.Context,
	name string,
	datacenter *object.Datacenter) error {

	vc := c.FileManager.Client()

	return pkgutil.DatastoreFileExists(ctx, vc, name, datacenter)
}

func (c *cacheStorageURIsClient) WaitForTask(
	ctx context.Context, task *object.Task) error {

	return task.Wait(ctx)
}
