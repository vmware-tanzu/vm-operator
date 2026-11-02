// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmwatcher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/property"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	zonectrl "github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vsphereclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
)

func init() {
	sync.OnceFunc(func() {
		// Every 30m print the cache statistics.
		ticker := time.NewTicker(30 * time.Minute)
		logger := ctrl.Log.WithName("vm-watcher-cache-stats")
		go func() {
			for range ticker.C {
				kvp := watcher.CacheGetStats()
				logger.Info("VM watcher cache stats", kvp...)
			}
		}()
	})
}

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

	logger := pkglog.FromContextOrDefault(s.ctx).WithName("VMWatcherService")
	ctx = logr.NewContext(ctx, logger)

	logger.Info("Starting VM watcher service")

	for ctx.Err() == nil {
		if err := s.waitForChanges(ctx); err != nil {

			// If waitForChanges failed because of an invalid login or auth
			// error, then do not treat the error as fatal. This allows the
			// loop to run again, kicking off another watcher with what should
			// be updated credentials.
			switch {
			case errors.Is(err, errVSphereClientReplaced):
				logger.V(4).Info("Restarting VM watcher on the replaced vSphere client")

			case vsphereclient.IsInvalidLogin(err) ||
				vsphereclient.IsNotAuthenticatedError(err):

				logger.V(4).Error(
					err,
					"Authn/authz issue trying start vm watcher service")

			default:
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
		objSet                []vimtypes.ObjectSpec
		moids                 = map[string][]string{}
		zonesWithoutFinalizer []topologyv1.Zone
	)

	if err := s.Client.List(ctx, &zones); err != nil {
		return nil, err
	}

	for i := range zones.Items {
		z := zones.Items[i]

		if v := z.Spec.ManagedVMs.FolderMoID; v != "" {

			// If a zone is being deleted and it does not have any
			// finalizer, the zone controller has already stopped the
			// watcher and removed the finalizer. Or, a zone is being
			// deleted even before a watcher could start. In either
			// case, no need to watch this folder.
			if !z.DeletionTimestamp.IsZero() &&
				!controllerutil.ContainsFinalizer(&z, zonectrl.Finalizer) {
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
			moids[v] = append(moids[v], fmt.Sprintf("%s/%s", z.Namespace, z.Name))

			// Ensure we add a finalizer to zones that the watcher is watching.
			if !controllerutil.ContainsFinalizer(&z, zonectrl.Finalizer) {
				zonesWithoutFinalizer = append(zonesWithoutFinalizer, z)
			}
		}
	}

	if len(objSet) == 0 {
		return nil, nil
	}

	// Filter the folder MoIDs for ones that exist on VC. The watcher will fail to start if
	// any of the initial MoIDs don't exist but need to watcher to start. For any stale Zones
	// the infra Zone controller will keep retrying to add their folder to the watcher.
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
		return nil, fmt.Errorf("failed to get managed VMs folder properties: %w", err)
	}

	moRefWithIDs := make(map[vimtypes.ManagedObjectReference][]string, len(res.Returnval))
	for _, o := range res.Returnval {
		moRefWithIDs[o.Obj] = moids[o.Obj.Value]
	}

	// For Zones with a valid FolderMoID but without the finalizer, add it before starting the
	// watcher so the infra Zone controller will remove it from the watcher.
	for _, z := range zonesWithoutFinalizer {
		moref := vimtypes.ManagedObjectReference{
			Type:  "Folder",
			Value: z.Spec.ManagedVMs.FolderMoID,
		}

		if _, ok := moRefWithIDs[moref]; ok {
			zPatch := ctrlclient.MergeFromWithOptions(z.DeepCopy(), ctrlclient.MergeFromWithOptimisticLock{})
			z.Finalizers = append(z.Finalizers, zonectrl.Finalizer)
			if err := s.Client.Patch(ctx, &z, zPatch); err != nil {
				return nil, fmt.Errorf("failed to add finalizer: %w", err)
			}
		}
	}

	return moRefWithIDs, nil
}

var emptyResult watcher.Result

// errVSphereClientReplaced is returned by waitForChanges after the provider discarded the vSphere
// client that the watcher was started with. The watcher is restarted on the replacement client
// rather than waiting for the old session's long poll to fail.
var errVSphereClientReplaced = errors.New("vSphere client replaced")

func (s Service) waitForChanges(ctx context.Context) error {

	var (
		logger     = pkglog.FromContextOrDefault(ctx)
		chanSource = cource.FromContextWithBuffer(ctx, "VirtualMachine", 100)
	)

	// Fetch the replacement signal before the client so that a replacement racing with the
	// fetch still restarts the watcher.
	clientReplaced := s.provider.VSphereClientReplaced()

	vcClient, err := s.provider.VSphereClient(ctx)
	if err != nil {
		return err
	}
	logger.Info("Got vsphere client")

	moRefWithIDs, err := s.vmFolderMoRefWithIDs(ctx, vcClient)
	if err != nil {
		return err
	}
	logger.Info("Got vm service folders",
		"refs", slices.Collect(maps.Keys(moRefWithIDs)))

	// Canceling watchCtx stops the watcher's pending WaitForUpdates so a
	// replaced client does not keep the old watcher alive.
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	// Start the watcher.
	w, err := watcher.Start(
		watchCtx,
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

			// Validate the vm exists on this cluster.
			var verifiedObj ctrlclient.Object

			if result.Verified {
				if obj, ok := result.VerifiedObj.(ctrlclient.Object); ok {
					verifiedObj = obj
				} else {
					logger.V(4).Info(
						"Received result but verified object is invalid",
						"result", result)
					result.Verified = false
				}
			}

			if !result.Verified {
				var vm vmopv1.VirtualMachine
				if s.Get(
					ctx,
					ctrlclient.ObjectKey{
						Namespace: result.Namespace,
						Name:      result.Name,
					},
					&vm) == nil {

					result.Verified = true
					verifiedObj = &vm
				}
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
				Object: verifiedObj,
			}

		case <-clientReplaced:
			logger.Info("vSphere client was replaced, restarting VM watcher")
			cancelWatch()
			// Wait for the old watcher to finish so its cleanup cannot clear the
			// context-registered watcher that the next run installs.
			<-w.Done()
			return errVSphereClientReplaced

		case <-w.Done():
			logger.Info("Received done signal, watcher is closed")
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
				Namespace:   obj.Namespace,
				Name:        obj.Name,
				Verified:    obj.Status.UniqueID != "",
				VerifiedObj: &obj,
				Deleted:     !obj.DeletionTimestamp.IsZero(),
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
				Namespace:   list.Items[0].Namespace,
				Name:        list.Items[0].Name,
				Verified:    true,
				VerifiedObj: &list.Items[0],
				Deleted:     !list.Items[0].DeletionTimestamp.IsZero(),
			}
		}
	}

	return watcher.LookupNamespacedNameResult{}
}
