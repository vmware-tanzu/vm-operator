// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/go-logr/logr"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
)

// DefaultWatchedPropertyPaths returns the default set of property paths to
// watch.
func DefaultWatchedPropertyPaths() []string {
	return []string{
		"config.extraConfig",
		"config.hardware.device",
		"config.keyId",
		"guest.ipStack",
		"guest.net",
		"guestHeartbeatStatus",
		"summary.config.name",
		"summary.guest",
		"summary.overallStatus",
		"summary.runtime.connectionState",
		"summary.runtime.host",
		"summary.runtime.powerState",
		"summary.storage.timestamp",
	}
}

const extraConfigNamespacedNameKey = "vmservice.namespacedName"

// defaultIgnoredExtraConfigKeys returns the default set of extra config keys to
// ignore.
var defaultIgnoredExtraConfigKeys = []string{
	backupapi.AdditionalResourcesYAMLExtraConfigKey,
	backupapi.BackupVersionExtraConfigKey,
	backupapi.ClassicDiskDataExtraConfigKey,
	backupapi.DisableAutoRegistrationExtraConfigKey,
	backupapi.EnableAutoRegistrationExtraConfigKey,
	backupapi.PVCDiskDataExtraConfigKey,
	backupapi.VMResourceYAMLExtraConfigKey,
	"govcsim",
	"guestinfo.metadata",
	"guestinfo.metadata.encoding",
	"guestinfo.userdata",
	"guestinfo.userdata.encoding",
	"guestinfo.vendordata",
	"guestinfo.vendordata.encoding",
	extraConfigNamespacedNameKey,
}

type moRef = vimtypes.ManagedObjectReference

// LookupNamespacedNameResult is returned from a call to lookup the namespaced
// name of a vSphere VM.
type LookupNamespacedNameResult struct {
	Namespace string
	Name      string

	// Verified indicates whether or not the VM's Kubernetes resource has the
	// vSphere VM's managed object ID in status.uniqueID.
	Verified bool

	// Deleted indicates whether the VM's Kubernetes resource has a
	// non-zero deletion timestamp.
	Deleted bool
}

// lookupNamespacedNameFn queries the namespace and name for a vSphere VM by
// searching for the VM's Kubernetes resource using the VM's managed object ID.
// If the namespace and name are already known, they are instead used to query
// the VM to make the lookup more efficient.
type lookupNamespacedNameFn func(
	ctx context.Context,
	vmRef moRef,
	namespace, name string) LookupNamespacedNameResult

type Result struct {
	// Namespace is the namespace to which the VirtualMachine resource belongs.
	Namespace string

	// Name is the name of the VirtualMachine resource.
	Name string

	// Ref is the ManagedObjectReference for the VM in vSphere.
	Ref moRef

	// Verified is true if the VirtualMachine resource identified by Namespace
	// and Name has already been verified to exist in this Kubernetes cluster.
	Verified bool
}

type Watcher struct {
	err        error
	errMu      sync.RWMutex
	cancel     func()
	chanDone   chan struct{}
	chanResult chan Result

	client *vim25.Client

	pc *property.Collector
	pf *property.Filter
	vm *view.Manager
	lv *view.ListView
	cv map[moRef]*view.ContainerView

	// cvr is used to keep track of what opaque IDs are using the list view of
	// each container. If the Remove function is called on a container with the
	// last ID, then the container will be removed from the list view and
	// destroyed.
	// cvr will have a container moref only if cv does, so cv should be checked
	// first.
	cvr map[moRef]map[string]struct{}

	ignoredExtraConfigKeys map[string]struct{}
	lookupNamespacedName   lookupNamespacedNameFn

	closeOnce sync.Once
}

// Done returns a channel that is closed when the watcher is shutdown.
func (w *Watcher) Done() <-chan struct{} {
	return w.chanDone
}

// Result returns a channel on which new results are received.
func (w *Watcher) Result() <-chan Result {
	return w.chanResult
}

// Err returns the error that caused the watcher to stop.
func (w *Watcher) Err() error {
	w.errMu.RLock()
	err := w.err
	w.errMu.RUnlock()
	return err
}

func (w *Watcher) setErr(err error) {
	w.errMu.Lock()
	w.err = err
	w.errMu.Unlock()
}

func newWatcher(
	ctx context.Context,
	client *vim25.Client,
	watchedPropertyPaths []string,
	additionalIgnoredExtraConfigKeys []string,
	lookupNamespacedName lookupNamespacedNameFn,
	containerRefsWithIDs map[moRef][]string) (*Watcher, error) {

	if watchedPropertyPaths == nil {
		watchedPropertyPaths = DefaultWatchedPropertyPaths()
	}
	ignoredExtraConfigKeys := slices.Concat(
		defaultIgnoredExtraConfigKeys,
		additionalIgnoredExtraConfigKeys)

	// Get the view manager.
	vm := view.NewManager(client)

	// For each container reference, create a container view and add it to
	// the list view's initial list of members.
	cvs, cvr, err := toContainerViewMap(ctx, vm, containerRefsWithIDs)
	if err != nil {
		return nil, err
	}

	// Create a new list view used to monitor all of the containers to which
	// VM Service VMs belong.
	lv, err := vm.CreateListView(ctx, toMoRefs(cvs))
	if err != nil {
		return nil, err
	}

	// Create a new property collector to watch for changes.
	pc, err := property.DefaultCollector(client).Create(ctx)
	if err != nil {
		return nil, err
	}

	// Create a new property filter that uses the list view created up above.
	pf, err := pc.CreateFilter(
		ctx,
		viewToVM(lv.Reference(), watchedPropertyPaths))
	if err != nil {
		return nil, err
	}

	return &Watcher{
		chanDone:               make(chan struct{}),
		chanResult:             make(chan Result),
		client:                 client,
		pc:                     pc,
		pf:                     pf,
		vm:                     vm,
		lv:                     lv,
		cv:                     cvs,
		cvr:                    cvr,
		ignoredExtraConfigKeys: toSet(ignoredExtraConfigKeys),
		lookupNamespacedName:   lookupNamespacedName,
	}, nil
}

func (w *Watcher) close() {
	w.closeOnce.Do(
		func() {
			w.cancel()
			close(w.chanDone)

			_ = w.pf.Destroy(context.Background())
			_ = w.pc.Destroy(context.Background())
			_ = w.lv.Destroy(context.Background())
			for _, cv := range w.cv {
				_ = cv.Destroy(context.Background())
			}
		})
}

// Start begins watching a vSphere server for updates to VM Service managed VMs.
// If watchedPropertyPaths is nil, DefaultWatchedPropertyPaths will be used.
// The containerRefsWithIDs parameter may be used to start the watcher with an
// initial list of entities to watch.
func Start(
	ctx context.Context,
	client *vim25.Client,
	watchedPropertyPaths []string,
	additionalIgnoredExtraConfigKeys []string,
	lookupNamespacedName lookupNamespacedNameFn,
	containerRefsWithIDs map[moRef][]string) (*Watcher, error) {

	logger := logr.FromContextOrDiscard(ctx).WithName("vSphereWatcher")

	logger.Info("Started watching VMs")

	w, err := newWatcher(
		ctx,
		client,
		watchedPropertyPaths,
		additionalIgnoredExtraConfigKeys,
		lookupNamespacedName,
		containerRefsWithIDs)
	if err != nil {
		return nil, err
	}

	// Update the context with this watcher.
	setContext(ctx, w)

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	w.cancel = cancel

	go func() {
		defer func() {
			// Remove this watcher from the context. While there is no watcher
			// in the context, calls to Add/Remove will fail.
			setContext(ctx, nil)

			w.close()
		}()

		if err := w.pc.WaitForUpdatesEx(
			ctx,
			&property.WaitOptions{},
			func(ou []vimtypes.ObjectUpdate) bool {
				return w.onUpdate(ctx, ou)
			}); err != nil {

			w.setErr(err)
		}
	}()

	return w, nil
}

const (
	virtualMachineType               = "VirtualMachine"
	configPropPath                   = "config"
	extraConfigPropPath              = configPropPath + ".extraConfig"
	extraConfigNamespaceNameKey      = "vmservice.namespacedName"
	extraConfigNamespaceNamePropPath = extraConfigPropPath + `["` + extraConfigNamespaceNameKey + `"]`
)

type objUpdate struct {
	kind    vimtypes.ObjectUpdateKind
	changes []vimtypes.PropertyChange
}

func (w *Watcher) onUpdate(
	ctx context.Context,
	ou []vimtypes.ObjectUpdate) bool {

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("OnUpdate", "objectUpdates", ou)

	updates := map[moRef]objUpdate{}

	for i := range ou {
		oui := ou[i]
		if oui.Kind != vimtypes.ObjectUpdateKindLeave {
			if v, ok := updates[oui.Obj]; !ok {
				updates[oui.Obj] = objUpdate{
					kind:    oui.Kind,
					changes: oui.ChangeSet,
				}
			} else {
				v.changes = append(v.changes, oui.ChangeSet...)
				updates[oui.Obj] = v
			}
		}
	}

	for obj, update := range updates {
		if err := w.onObject(
			ctx,
			obj,
			update); err != nil {

			w.setErr(err)
			return true
		}
	}

	return false
}

func (w *Watcher) onObject(
	ctx context.Context,
	obj moRef,
	update objUpdate) error {

	logger := logr.FromContextOrDiscard(ctx).
		WithName("onObject").
		WithValues("obj", obj)

	var (
		namespace string
		name      string
		verified  bool
		deleted   bool
	)

	// This update will be skipped if after removing all of the changes for
	// the ignoredExtraConfigKeys there is nothing left.
	var ignoredChanges int
	for i := range update.changes {
		c := update.changes[i]
		ignore := false
		if c.Name == extraConfigPropPath {
			if aov, ok := c.Val.(vimtypes.ArrayOfOptionValue); ok {
				ignore, namespace, name = checkExtraConfig(
					aov, w.ignoredExtraConfigKeys)
			}
		}
		if ignore {
			ignoredChanges++
		}
	}
	if ignoredChanges == len(update.changes) {
		logger.V(5).Info("skipping async signal", "reason", "ignored changes")
		return nil
	}

	if w.lookupNamespacedName != nil {
		r := w.lookupNamespacedName(ctx, obj, namespace, name)
		if r.Namespace != "" {
			namespace = r.Namespace
		}
		if r.Name != "" {
			name = r.Name
		}
		verified = r.Verified
		deleted = r.Deleted
	}

	if deleted {
		// Do not enqueue an async reconcile for a VM's Kubernetes resource that
		// is already being deleted. This avoids an infinite loop caused by the
		// fact that vSphere's Delete API invokes Reload internally on the VM.
		// The Reload API causes a MODIFY event to be sent to all property
		// collectors for the VM, resulting in this watcher triggering another
		// reconcile. Since the VM is being deleted, we would then issue another
		// Delete call to the vSphere VM, triggering another Reload call,
		// triggering another property collector signal. Rinse and repeat.
		logger.V(5).Info("skipping async signal",
			"reason", "vm is being deleted")
		return nil
	}

	if update.kind == vimtypes.ObjectUpdateKindEnter && verified {
		// The behavior of Controller-Runtime to sync all objects upon startup
		// will cause *existing* VMs to be reconciled. Therefore, do not emit a
		// result when the object is entering the scope of the watcher and the
		// corresponding Kubernetes object already exists with a matching
		// status.uniqueID field.
		logger.V(5).Info("skipping async signal",
			"reason", "verified vm entered scope")
		return nil
	}

	if namespace == "" || name == "" {
		var content []vimtypes.ObjectContent
		err := property.DefaultCollector(w.client).RetrieveOne(
			ctx,
			obj,
			[]string{extraConfigNamespaceNamePropPath},
			&content,
		)
		if err != nil {
			return err
		}
		namespace, name = namespacedNameFromObjContent(content)
	}

	if namespace != "" && name != "" {
		r := Result{
			Namespace: namespace,
			Name:      name,
			Ref:       obj,
			Verified:  verified,
		}

		logger.V(4).Info("Sending result", "result", r)

		go func(r Result) {
			w.chanResult <- r
		}(r)
	}

	return nil
}

func checkExtraConfig(
	aov vimtypes.ArrayOfOptionValue,
	ignoredKeys map[string]struct{}) (ignore bool, namespace, name string) {

	hasNonIgnoredKey := false

	for j := range aov.OptionValue {
		if ov := aov.OptionValue[j].GetOptionValue(); ov != nil {
			// Get the namespace and name of the VM from the changes
			// if they are present there.
			if ov.Key == extraConfigNamespacedNameKey {
				if namespace == "" || name == "" {
					if s, ok := ov.Value.(string); ok {
						namespace, name = namespacedNameFromString(s)
					}
				}
			}
			// Note if the key cannot be ignored.
			if _, ok := ignoredKeys[ov.Key]; !ok {
				hasNonIgnoredKey = true
			}
			if hasNonIgnoredKey && (namespace != "" && name != "") {
				break
			}
		}
	}

	return !hasNonIgnoredKey, namespace, name
}

func (w *Watcher) add(ctx context.Context, ref moRef, id string) error {
	if _, ok := w.cv[ref]; ok {
		w.cvr[ref][id] = struct{}{}
		return nil
	}

	// Do not recurse into child folders.
	// Please see BZ 3502919 for more information.
	recursive := ref.Type != string(vimtypes.ManagedObjectTypesFolder)

	cv, err := w.vm.CreateContainerView(
		ctx,
		ref,
		[]string{virtualMachineType},
		recursive)
	if err != nil {
		return err
	}

	if _, err := w.lv.Add(
		ctx,
		[]vimtypes.ManagedObjectReference{cv.Reference()}); err != nil {

		if err2 := cv.Destroy(context.Background()); err2 != nil {
			return fmt.Errorf(
				"failed to destroy container view after adding "+
					"it to list failed: addErr=%w, destroyErr=%w", err, err2)
		}

		return err
	}

	w.cv[ref] = cv
	if w.cvr[ref] == nil {
		w.cvr[ref] = map[string]struct{}{}
	}
	w.cvr[ref][id] = struct{}{}

	return nil
}

func (w *Watcher) remove(_ context.Context, ref moRef, id string) error {
	cv, ok := w.cv[ref]
	if !ok {
		return nil
	}

	// Only remove the container from the list view if this ref is the
	// last user, and make sure that this ID is actually in use.
	if _, ok := w.cvr[ref][id]; !ok || len(w.cvr[ref]) > 1 {
		delete(w.cvr[ref], id)
		return nil
	}

	_, err := w.lv.Remove(context.Background(), []moRef{cv.Reference()})
	if err != nil {
		return err
	}

	if err := cv.Destroy(context.Background()); err != nil {
		return err
	}

	delete(w.cv, ref)
	delete(w.cvr, ref)

	return nil
}

func toContainerViewMap(
	ctx context.Context,
	vm *view.Manager,
	containerRefsWithIDs map[moRef][]string) (map[moRef]*view.ContainerView, map[moRef]map[string]struct{}, error) {

	var (
		cvMap     = map[moRef]*view.ContainerView{}
		cvRefsMap = map[moRef]map[string]struct{}{}
	)

	if len(containerRefsWithIDs) == 0 {
		return cvMap, cvRefsMap, nil
	}

	var resultErr error
	for moref, ids := range containerRefsWithIDs {
		cv, err := vm.CreateContainerView(
			ctx,
			moref,
			[]string{virtualMachineType},
			true)
		if err != nil {
			resultErr = err
			break
		}
		cvMap[moref] = cv

		idSet := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			idSet[id] = struct{}{}
		}
		cvRefsMap[moref] = idSet
	}

	if resultErr != nil {
		// There was an error creating container views, so make sure to clean up
		// any views that *were* created before returning.
		for _, cv := range cvMap {
			if err := cv.Destroy(context.Background()); err != nil {
				resultErr = fmt.Errorf("%w,%w", resultErr, err)
			}
		}
		return nil, nil, resultErr
	}

	return cvMap, cvRefsMap, nil
}

func namespacedNameFromString(s string) (string, string) {
	if p := strings.Split(s, "/"); len(p) == 2 {
		return p[0], p[1]
	}
	return "", ""
}

func namespacedNameFromObjContent(
	oc []vimtypes.ObjectContent) (string, string) {

	for i := range oc {
		for j := range oc[i].PropSet {
			dp := oc[i].PropSet[j]
			if dp.Name == extraConfigNamespaceNamePropPath {
				if ov, ok := dp.Val.(vimtypes.OptionValue); ok {
					if v, ok := ov.Value.(string); ok {
						return namespacedNameFromString(v)
					}
				}
			}
		}
	}
	return "", ""
}

func viewToVM(ref moRef, watchedPropertyPaths []string) vimtypes.CreateFilter {
	return vimtypes.CreateFilter{
		Spec: vimtypes.PropertyFilterSpec{
			ObjectSet: []vimtypes.ObjectSpec{
				{
					Obj:  ref,
					Skip: &[]bool{true}[0],
					SelectSet: []vimtypes.BaseSelectionSpec{
						// ListView --> ContainerView
						&vimtypes.TraversalSpec{
							Type: "ListView",
							Path: "view",
							SelectSet: []vimtypes.BaseSelectionSpec{
								&vimtypes.SelectionSpec{
									Name: "visitViews",
								},
							},
						},
						// ContainerView --> VM
						&vimtypes.TraversalSpec{
							SelectionSpec: vimtypes.SelectionSpec{
								Name: "visitViews",
							},
							Type: "ContainerView",
							Path: "view",
						},
					},
				},
			},
			PropSet: []vimtypes.PropertySpec{
				{
					Type:    virtualMachineType,
					PathSet: watchedPropertyPaths,
				},
			},
		},
	}
}

type hasRef interface {
	Reference() moRef
}

func toMoRefs[M ~map[K]V, K comparable, V hasRef](m M) []moRef {
	if len(m) == 0 {
		return nil
	}
	r := make([]moRef, 0, len(m))
	for _, v := range m {
		r = append(r, v.Reference())
	}
	return r
}

func toSet[K comparable](s []K) map[K]struct{} {
	if len(s) == 0 {
		return nil
	}
	r := make(map[K]struct{}, len(s))
	for i := range s {
		r[s[i]] = struct{}{}
	}
	return r
}
