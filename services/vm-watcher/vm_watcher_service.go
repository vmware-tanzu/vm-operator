// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmwatcher

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/property"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	zonectrl "github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
	setrpctrl "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vsphereclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
)

// AddToManager adds this package's runnable to the provided manager.
func AddToManager(
	ctx *pkgctx.ControllerManagerContext,
	mgr manager.Manager) error {

	// Index the VM's MoRef ID set in status. This is used by the vm-watcher
	// service to quickly indirect a MoRefs to a Namespace/Name.
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachine{},
		"status.uniqueID",
		func(rawObj ctrlclient.Object) []string {
			vm := rawObj.(*vmopv1.VirtualMachine)
			return []string{vm.Status.UniqueID}
		}); err != nil {
		return err
	}

	return mgr.Add(New(ctx, mgr.GetClient(), ctx.VMProvider))
}

type Service struct {
	ctrlclient.Client
	ctx      context.Context
	provider providers.VirtualMachineProviderInterface
}

func New(
	ctx context.Context,
	client ctrlclient.Client,
	provider providers.VirtualMachineProviderInterface) manager.Runnable {

	return Service{
		Client:   client,
		ctx:      ctx,
		provider: provider,
	}
}

var _ manager.LeaderElectionRunnable = Service{}

func (s Service) NeedLeaderElection() bool {
	return true
}

func (s Service) Start(ctx context.Context) error {
	ctx = cource.JoinContext(ctx, s.ctx)
	ctx = watcher.JoinContext(ctx, s.ctx)
	ctx = pkgcfg.JoinContext(ctx, s.ctx)

	logger := logr.FromContextOrDiscard(s.ctx).WithName("VMWatcherService")
	ctx = logr.NewContext(ctx, logger)

	logger.Info("Starting VM watcher service")

	for ctx.Err() == nil {
		if err := s.waitForChanges(ctx); err != nil {
			// If waitForChanges failed because of an invalid login or auth
			// error, then do not treat the error as fatal. This allows the
			// loop to run again, kicking off another watcher with what should
			// be updated credentials.
			if vsphereclient.IsInvalidLogin(err) ||
				vsphereclient.IsNotAuthenticatedError(err) {

				logger.V(4).Error(
					err,
					"Authn/authz issue trying start vm watcher service")

			} else {
				// Log unexpected error.
				logger.Error(
					err,
					"Unexpected error trying to start vm watcher service")
			}

			// TODO: We might want to sleep a bit here so we don't spin if like we
			// cannot get a VC client.
		}
	}
	return ctx.Err()
}

func (s Service) vmFolderMoRefWithIDs(
	ctx context.Context,
	vcClient *vsphereclient.Client) (map[vimtypes.ManagedObjectReference][]string, error) {

	// Get a list of all the folders that can contain VM Service VMs.
	var (
		zones                 topologyv1.ZoneList
		setrp                 vmopv1.VirtualMachineSetResourcePolicyList
		objSet                []vimtypes.ObjectSpec
		moids                 = map[string][]string{}
		zonesWithoutFinalizer []topologyv1.Zone
		setrpWithoutFinalizer []vmopv1.VirtualMachineSetResourcePolicy
	)

	// Get the Zone objects.
	if err := s.Client.List(ctx, &zones); err != nil {
		return nil, err
	}
	for i := range zones.Items {
		o := zones.Items[i]

		if v := o.Spec.ManagedVMs.FolderMoID; v != "" {

			// If an object is being deleted and it does not have a finalizer,
			// the object's controller has already stopped the watcher and
			// removed the finalizer. Or, an object is being deleted before a
			// watcher could start. In either case there is no need to watch the
			// folder.
			if !o.DeletionTimestamp.IsZero() &&
				!controllerutil.ContainsFinalizer(&o, zonectrl.Finalizer) {
				continue
			}

			if _, ok := moids[v]; !ok {
				moids[v] = make([]string, 0, 1)

				objSet = append(objSet, vimtypes.ObjectSpec{
					Obj: vimtypes.ManagedObjectReference{
						Type:  "Folder",
						Value: v,
					},
				})
			}
			moids[v] = append(
				moids[v],
				fmt.Sprintf("%s:%s/%s", "Zone", o.Namespace, o.Name))

			// Ensure a finalizer is added to watched objects.
			if !controllerutil.ContainsFinalizer(&o, zonectrl.Finalizer) {
				zonesWithoutFinalizer = append(zonesWithoutFinalizer, o)
			}
		}
	}

	// Get the VirtualMachineSetResourcePolicy objects.
	if err := s.Client.List(ctx, &setrp); err != nil {
		return nil, err
	}
	for i := range setrp.Items {
		o := setrp.Items[i]

		if v := o.Status.FolderID; v != "" {

			// If an object is being deleted and it does not have a finalizer,
			// the object's controller has already stopped the watcher and
			// removed the finalizer. Or, an object is being deleted before a
			// watcher could start. In either case there is no need to watch the
			// folder.
			if !o.DeletionTimestamp.IsZero() &&
				!controllerutil.ContainsFinalizer(&o, setrpctrl.Finalizer) {
				continue
			}

			if _, ok := moids[v]; !ok {
				moids[v] = make([]string, 0, 1)

				objSet = append(objSet, vimtypes.ObjectSpec{
					Obj: vimtypes.ManagedObjectReference{
						Type:  "Folder",
						Value: v,
					},
				})
			}
			moids[v] = append(
				moids[v],
				fmt.Sprintf(
					"%s:%s/%s",
					"VirtualMachineSetResourcePolicy", o.Namespace, o.Name))

			// Ensure a finalizer is added to watched objects.
			if !controllerutil.ContainsFinalizer(&o, setrpctrl.Finalizer) {
				setrpWithoutFinalizer = append(setrpWithoutFinalizer, o)
			}
		}
	}

	if len(objSet) == 0 {
		return nil, nil
	}

	// Filter the folder MoIDs for ones that exist on VC. The watcher will fail
	// to start if any of the initial MoIDs don't exist but need to watcher to
	// start.
	//
	// For any stale objects, their controllers will keep trying to add their
	// folder to the watcher.
	pc := property.DefaultCollector(vcClient.VimClient())
	res, err := pc.RetrieveProperties(ctx, vimtypes.RetrieveProperties{
		SpecSet: []vimtypes.PropertyFilterSpec{
			{
				PropSet: []vimtypes.PropertySpec{
					{
						Type: "Folder",
					},
				},
				ObjectSet:                     objSet,
				ReportMissingObjectsInResults: vimtypes.NewBool(true),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get managed VMs folder properties: %w", err)
	}

	moRefWithIDs := make(map[vimtypes.ManagedObjectReference][]string, len(res.Returnval))
	for _, o := range res.Returnval {
		moRefWithIDs[o.Obj] = moids[o.Obj.Value]
	}

	// For Zone objects with a valid FolderMoID but without the finalizer, add
	// the finalizer before starting the watcher so the zone controller will
	// remove it from the watcher.
	for _, o := range zonesWithoutFinalizer {
		moref := vimtypes.ManagedObjectReference{
			Type:  "Folder",
			Value: o.Spec.ManagedVMs.FolderMoID,
		}
		if _, ok := moRefWithIDs[moref]; ok {
			p := ctrlclient.MergeFromWithOptions(
				o.DeepCopy(),
				ctrlclient.MergeFromWithOptimisticLock{})
			o.Finalizers = append(o.Finalizers, zonectrl.Finalizer)
			if err := s.Client.Patch(ctx, &o, p); err != nil {
				return nil, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	}

	// For VirtualMachineSetResourcePolicy objects with a valid FolderMoID but
	// without the finalizer, add the finalizer before starting the watcher so
	// the zone controller will remove it from the watcher.
	for _, o := range setrpWithoutFinalizer {
		moref := vimtypes.ManagedObjectReference{
			Type:  "Folder",
			Value: o.Status.FolderID,
		}
		if _, ok := moRefWithIDs[moref]; ok {
			p := ctrlclient.MergeFromWithOptions(
				o.DeepCopy(),
				ctrlclient.MergeFromWithOptimisticLock{})
			o.Finalizers = append(o.Finalizers, setrpctrl.Finalizer)
			if err := s.Client.Patch(ctx, &o, p); err != nil {
				return nil, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	}

	return moRefWithIDs, nil
}

var emptyResult watcher.Result

func (s Service) waitForChanges(ctx context.Context) error {
	var (
		logger     = logr.FromContextOrDiscard(ctx)
		chanSource = cource.FromContextWithBuffer(ctx, "VirtualMachine", 100)
	)

	vcClient, err := s.provider.VSphereClient(ctx)
	if err != nil {
		return err
	}
	logger.Info("Got vsphere client")

	moRefWithIDs, err := s.vmFolderMoRefWithIDs(ctx, vcClient)
	if err != nil {
		return err
	}
	logger.Info("Got vm service folders", "refs", slices.Collect(maps.Keys(moRefWithIDs)))

	// Start the watcher.
	w, err := watcher.Start(
		ctx,
		vcClient.VimClient(),
		nil,
		nil,
		s.lookupNamespacedName,
		moRefWithIDs)
	if err != nil {
		return err
	}

	for {
		select {
		case result := <-w.Result():
			if result == emptyResult {
				logger.Info("Received empty result, watcher is closed")
				return w.Err()
			}

			if !result.Verified {
				// Validate the vm exists on this cluster.
				result.Verified = s.Get(
					ctx,
					ctrlclient.ObjectKey{
						Namespace: result.Namespace,
						Name:      result.Name,
					},
					&vmopv1.VirtualMachine{}) == nil
			}

			if !result.Verified {
				logger.V(4).Info(
					"Received result but unable to validate vm",
					"result", result)
				continue
			}

			// Enqueue a reconcile request for the VM.
			logger.V(4).Info("Received result", "result", result)
			chanSource <- event.GenericEvent{
				Object: &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: result.Namespace,
						Name:      result.Name,
					},
				},
			}

		case <-w.Done():
			return w.Err()
		}
	}
}

// lookupNamespacedName looks up the namespace and name for a given MoRef using
// the Kubernetes client's cache, where the "status.uniqueID" field of VMs are
// indexed for fast lookup.
func (s Service) lookupNamespacedName(
	ctx context.Context,
	moRef vimtypes.ManagedObjectReference,
	namespace, name string) watcher.LookupNamespacedNameResult {

	if namespace != "" && name != "" {
		var obj vmopv1.VirtualMachine
		if err := s.Client.Get(
			ctx,
			ctrlclient.ObjectKey{Namespace: namespace, Name: name},
			&obj); err == nil {

			return watcher.LookupNamespacedNameResult{
				Namespace: obj.Namespace,
				Name:      obj.Name,
				Verified:  obj.Status.UniqueID != "",
				Deleted:   !obj.DeletionTimestamp.IsZero(),
			}
		}
	}

	var list vmopv1.VirtualMachineList
	if err := s.Client.List(
		ctx,
		&list,
		ctrlclient.MatchingFields{"status.uniqueID": moRef.Value}); err == nil {

		if len(list.Items) == 1 {
			return watcher.LookupNamespacedNameResult{
				Namespace: list.Items[0].Namespace,
				Name:      list.Items[0].Name,
				Verified:  true,
				Deleted:   !list.Items[0].DeletionTimestamp.IsZero(),
			}
		}
	}

	return watcher.LookupNamespacedNameResult{}
}
