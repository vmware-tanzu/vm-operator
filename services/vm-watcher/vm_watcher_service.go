// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmwatcher

import (
	"context"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
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
			if !vsphereclient.IsInvalidLogin(err) &&
				!vsphereclient.IsNotAuthenticatedError(err) {

				return err
			}
		}
	}
	return ctx.Err()
}

var emptyResult watcher.Result

func (s Service) vmFolderRefs(ctx context.Context) ([]vimtypes.ManagedObjectReference, error) {
	// Get a list of all the folders that can contain VM Service VMs.
	var (
		zones topologyv1.ZoneList
		frefs []vimtypes.ManagedObjectReference
		moids = map[string]struct{}{}
	)
	if err := s.Client.List(ctx, &zones); err != nil {
		return nil, err
	}
	for i := range zones.Items {
		z := zones.Items[i]
		if v := z.Spec.ManagedVMs.FolderMoID; v != "" {
			if _, ok := moids[v]; !ok {
				moids[v] = struct{}{}
				frefs = append(frefs, vimtypes.ManagedObjectReference{
					Type:  "Folder",
					Value: v,
				})
			}
		}
	}

	return frefs, nil
}

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

	refs, err := s.vmFolderRefs(ctx)
	if err != nil {
		return err
	}
	logger.Info("Got vm service folders", "refs", refs)

	// Start the watcher.
	w, err := watcher.Start(
		ctx,
		vcClient.VimClient(),
		nil,
		nil,
		s.lookupNamespacedName,
		refs...)
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
	moRef vimtypes.ManagedObjectReference) (string, string, bool) {

	var list vmopv1.VirtualMachineList
	if err := s.Client.List(
		ctx,
		&list,
		ctrlclient.MatchingFields{"status.uniqueID": moRef.Value}); err == nil {

		if len(list.Items) == 1 {
			return list.Items[0].Namespace, list.Items[0].Name, true
		}
	}

	return "", "", false
}
