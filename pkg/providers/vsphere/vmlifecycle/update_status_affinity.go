// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter/vm/compute/policies"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// ComputePolicyQuerier queries vCenter for the compute policies that are
// associated with VMs.
type ComputePolicyQuerier interface {
	// Query returns the compute policies associated with the VMs in spec.
	// The VM is the Kubernetes object the query is made for.
	Query(
		ctx context.Context,
		vm *vmopv1.VirtualMachine,
		spec policies.QuerySpec) (*policies.QueryResult, error)
}

// NewComputePolicyQuerier returns a ComputePolicyQuerier that uses the VM
// compute-policy query API of the vCenter that restClient is connected to.
func NewComputePolicyQuerier(restClient *rest.Client) ComputePolicyQuerier {
	return computePolicyQuerier{mgr: policies.NewManager(restClient)}
}

type computePolicyQuerier struct {
	mgr *policies.Manager
}

func (q computePolicyQuerier) Query(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	spec policies.QuerySpec) (*policies.QueryResult, error) {

	ctx = pkgctx.WithVCOpID(ctx, vm, "queryComputePolicies")
	return q.mgr.Query(ctx, spec)
}

// ReconcileAffinityStatus rebuilds vm.Status.Affinity from the compute
// policies that vCenter reports for the VM and the policy-defining Tags that
// the VM owns or carries.
//
// A reported policy ID is mapped to a Tag through the Tag status.policyID
// field index. While a related Tag has status.id but no status.policyID, and
// the VM reports a policy ID that no Tag claims, the compute-policy query is
// used to find the policy that the Tag defines, and status.policyID is
// recorded on the Tag. A related Tag whose policy is not reported for the VM
// is reported as Pending.
//
// The function never fails the status reconcile. Errors are logged, and the
// affected entries keep their previous policy ID where vCenter still reports
// it for the VM, or are otherwise Pending. An error also marks the context
// with virtualmachine.MarkAffinityStatusRetry, so the VM controller requeues
// the VM after AffinityStatusPendingRequeueDelay when the reconcile has no
// other error. When the feature is disabled, the status is cleared.
func ReconcileAffinityStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	querier ComputePolicyQuerier) {

	vm := vmCtx.VM

	if !virtualmachine.AffinityStatusEnabled(vmCtx) {
		vm.Status.Affinity = nil
		return
	}

	r := affinityStatusReconciler{
		vmCtx:     vmCtx,
		k8sClient: k8sClient,
		querier:   querier,
		reported:  map[string]string{},
		byPolicy:  map[string]*vspherepolv1.Tag{},
	}

	r.mapReportedPolicies()
	r.mapRelatedTags()

	if r.degraded {
		virtualmachine.MarkAffinityStatusRetry(vmCtx)
	}

	entries := r.entries()
	if len(entries) == 0 {
		vm.Status.Affinity = nil
		return
	}
	virtualmachine.SortAffinityPolicyStatuses(entries)
	vm.Status.Affinity = &vmopv1.VirtualMachineAffinityStatus{
		Policies: entries,
	}
}

type affinityStatusReconciler struct {
	vmCtx     pkgctx.VirtualMachineContext
	k8sClient ctrlclient.Client
	querier   ComputePolicyQuerier

	// reported maps each policy ID that vCenter reports for the VM to its
	// compliance, in the order first reported.
	reported    map[string]string
	reportedIDs []string

	// byPolicy maps a reported policy ID to the policy-defining Tag that
	// claims it.
	byPolicy map[string]*vspherepolv1.Tag

	// owned are the names of the policy-defining Tags that the VM's own
	// spec.affinity requests. It is nil when they could not be determined.
	owned sets.Set[string]

	// related are the policy-defining Tags that the VM owns or carries.
	related    []vspherepolv1.Tag
	relatedErr bool

	// degraded is true when an error prevented a mapping from being
	// resolved in this pass. It requests a retry of the affinity status.
	degraded bool
}

// mapReportedPolicies maps each policy ID in the VM's computePolicies to the
// policy-defining Tag that claims it, using the cached field index.
func (r *affinityStatusReconciler) mapReportedPolicies() {
	for _, p := range r.vmCtx.MoVM.ComputePolicies {
		if p.PolicyId == "" {
			continue
		}
		if _, ok := r.reported[p.PolicyId]; ok {
			continue
		}
		r.reported[p.PolicyId] = p.ComplianceStatus
		r.vmCtx.Logger.V(4).Info("Got compute policy for affinity status",
			"policyID", p.PolicyId,
			"complianceStatus", p.ComplianceStatus)
		r.reportedIDs = append(r.reportedIDs, p.PolicyId)
	}

	for _, id := range r.reportedIDs {
		var list vspherepolv1.TagList
		if err := r.k8sClient.List(
			r.vmCtx,
			&list,
			ctrlclient.InNamespace(r.vmCtx.VM.Namespace),
			ctrlclient.MatchingFields{kubeutil.TagPolicyIDIndexKey: id},
		); err != nil {
			r.vmCtx.Logger.Error(err,
				"Failed to list Tags by policy ID for affinity status",
				"policyID", id)
			r.degraded = true
			continue
		}

		// Hashed policy tags map to exactly one policy, so at most one Tag
		// is expected. Pick the first by name for a stable result.
		var match *vspherepolv1.Tag
		for i := range list.Items {
			t := &list.Items[i]
			if !virtualmachine.IsAffinityPolicyTag(t) ||
				t.Status.PolicyID != id {
				continue
			}
			if match == nil || t.Name < match.Name {
				match = t
			}
		}
		if match != nil {
			r.byPolicy[id] = match
		}
	}
}

// mapRelatedTags gets the Tags related to the VM and, under the query trigger,
// resolves the policy IDs of the related Tags that are not mapped yet.
func (r *affinityStatusReconciler) mapRelatedTags() {
	owned, err := virtualmachine.OwnedAffinityPolicyTagNames(
		r.vmCtx, r.vmCtx.VM)
	if err != nil {
		r.vmCtx.Logger.Error(err,
			"Failed to get owned affinity Tags for affinity status")
		r.relatedErr = true
		r.degraded = true
		return
	}

	related, err := virtualmachine.RelatedAffinityPolicyTags(
		r.vmCtx, r.k8sClient, r.vmCtx.VM)
	if err != nil {
		r.vmCtx.Logger.Error(err,
			"Failed to get related affinity Tags for affinity status")
		r.relatedErr = true
		r.degraded = true
		return
	}
	r.owned = owned
	r.related = related

	// needMap holds the related Tags that have a vCenter tag ID but no
	// policy ID yet, keyed by the vCenter tag ID.
	needMap := map[string]*vspherepolv1.Tag{}
	for i := range r.related {
		t := &r.related[i]
		if t.Status.ID != "" && t.Status.PolicyID == "" {
			needMap[t.Status.ID] = t
		}
	}

	var unclaimed []string
	for _, id := range r.reportedIDs {
		if _, ok := r.byPolicy[id]; !ok {
			unclaimed = append(unclaimed, id)
		}
	}

	if len(needMap) == 0 || len(unclaimed) == 0 {
		return
	}

	r.queryUnmappedTags(needMap, unclaimed)
}

// queryUnmappedTags queries the compute policies of the VM, filtered to the
// unclaimed policy IDs, and records the policy ID of every Tag in needMap
// that defines one of them.
func (r *affinityStatusReconciler) queryUnmappedTags(
	needMap map[string]*vspherepolv1.Tag,
	unclaimed []string) {

	var (
		vmCtx  = r.vmCtx
		logger = vmCtx.Logger
		vmID   = vmCtx.MoVM.Self.Value
	)

	if r.querier == nil {
		logger.V(4).Info(
			"Skipping compute policy query for affinity status: no querier")
		return
	}
	if vmID == "" {
		logger.V(4).Info(
			"Skipping compute policy query for affinity status: no VM ID")
		return
	}

	logger.V(4).Info("Querying compute policies for affinity status",
		"policyIDs", unclaimed)

	res, err := r.querier.Query(vmCtx, vmCtx.VM, policies.QuerySpec{
		VMs:    []string{vmID},
		Filter: &policies.FilterSpec{Policies: unclaimed},
	})
	if err != nil {
		logger.Error(err,
			"Failed to query compute policies for affinity status",
			"policyIDs", unclaimed)
		r.degraded = true
		return
	}
	if res == nil {
		return
	}

	if vmErr, ok := res.Errors[vmID]; ok {
		logger.Error(
			errors.New(string(vmErr)),
			"Failed to query compute policies of VM for affinity status",
			"policyIDs", unclaimed)
		r.degraded = true
		return
	}

	for _, item := range res.Items {
		if item.VM != vmID {
			continue
		}
		for _, p := range item.Policies {
			if p.Policy == "" || p.Tag == "" {
				continue
			}
			if !slices.Contains(unclaimed, p.Policy) {
				continue
			}
			if _, ok := r.byPolicy[p.Policy]; ok {
				continue
			}
			t, ok := needMap[p.Tag]
			if !ok {
				continue
			}
			if err := r.patchTagPolicyID(t, p.Policy); err != nil {
				if apierrors.IsConflict(err) {
					// Related VMs race to record the policy ID on the shared
					// Tag, so a conflict is expected. The pass is still
					// degraded, so the VM is requeued even when the
					// conflicting write did not change the Tag's IDs.
					logger.V(4).Info(
						"Conflict recording policy ID on Tag for affinity status",
						"tag", t.Name,
						"policyID", p.Policy,
						"error", err.Error())
				} else {
					logger.Error(err,
						"Failed to record policy ID on Tag for affinity status",
						"tag", t.Name,
						"policyID", p.Policy)
				}
				r.degraded = true
				continue
			}
			r.byPolicy[p.Policy] = t
			delete(needMap, p.Tag)
		}
	}
}

// patchTagPolicyID records policyID in the Tag's status. The Tag is shared by
// every VM related to the policy, so the patch uses an optimistic lock, and it
// is skipped when the value is unchanged.
func (r *affinityStatusReconciler) patchTagPolicyID(
	tag *vspherepolv1.Tag,
	policyID string) error {

	if tag.Status.PolicyID == policyID {
		return nil
	}

	base := tag.DeepCopy()
	tag.Status.PolicyID = policyID

	if err := r.k8sClient.Status().Patch(
		r.vmCtx,
		tag,
		ctrlclient.MergeFromWithOptions(
			base,
			ctrlclient.MergeFromWithOptimisticLock{}),
	); err != nil {
		tag.Status.PolicyID = base.Status.PolicyID
		return fmt.Errorf("patch Tag %s/%s status.policyID: %w",
			tag.Namespace, tag.Name, err)
	}

	r.vmCtx.Logger.Info("Recorded compute policy ID on Tag",
		"tag", tag.Name,
		"policyID", policyID)

	return nil
}

// entries returns the status entries: one for each reported policy ID that a
// Tag claims, and a Pending entry for each related Tag whose policy is not
// reported for the VM. When an error degraded this pass, an entry that would
// be Pending keeps its previous policy ID while vCenter still reports it.
func (r *affinityStatusReconciler) entries() []vmopv1.VirtualMachineAffinityPolicyStatus {
	var (
		entries  []vmopv1.VirtualMachineAffinityPolicyStatus
		byKey    = map[string]int{}
		mapped   = map[string]struct{}{}
		claimed  = map[string]struct{}{}
		addEntry = func(e vmopv1.VirtualMachineAffinityPolicyStatus) {
			k := affinityPolicyKey(e)
			if _, ok := byKey[k]; ok {
				return
			}
			byKey[k] = len(entries)
			entries = append(entries, e)
		}
	)

	for _, id := range r.reportedIDs {
		t, ok := r.byPolicy[id]
		if !ok {
			continue
		}
		e, ok := virtualmachine.AffinityPolicyStatusFromTag(
			t, virtualmachine.AffinityComplianceFromVCenter(r.reported[id]))
		if !ok {
			continue
		}
		e.PolicyID = id
		e.Origin = r.origin(t.Name, e)
		addEntry(e)
		mapped[t.Name] = struct{}{}
		claimed[id] = struct{}{}
	}

	for i := range r.related {
		t := &r.related[i]
		if _, ok := mapped[t.Name]; ok {
			continue
		}
		e, ok := virtualmachine.AffinityPolicyStatusFromTag(
			t, vmopv1.VirtualMachineAffinityCompliancePending)
		if !ok {
			continue
		}
		e.Origin = r.origin(t.Name, e)
		addEntry(e)
	}

	if r.degraded && r.vmCtx.VM.Status.Affinity != nil {
		for _, prev := range r.vmCtx.VM.Status.Affinity.Policies {
			k := affinityPolicyKey(prev)
			idx, exists := byKey[k]

			if prev.PolicyID == "" {
				// Without the related Tags, keep the previous Pending
				// entries rather than dropping them.
				if !exists && r.relatedErr {
					addEntry(withOrigin(prev))
				}
				continue
			}

			compliance, reported := r.reported[prev.PolicyID]
			if !reported {
				continue
			}
			if _, ok := claimed[prev.PolicyID]; ok {
				continue
			}

			e := withOrigin(prev)
			e.Compliance = virtualmachine.AffinityComplianceFromVCenter(
				compliance)

			switch {
			case !exists && r.relatedErr:
				addEntry(e)
			case exists && entries[idx].Compliance ==
				vmopv1.VirtualMachineAffinityCompliancePending:
				e.Origin = entries[idx].Origin
				entries[idx] = e
			default:
				continue
			}
			claimed[prev.PolicyID] = struct{}{}
		}
	}

	return entries
}

// origin returns Self when the Tag is one the VM's own spec.affinity requests,
// and Peer otherwise. When the owned Tags are not known, the previous origin
// of the same policy is kept, defaulting to Peer.
func (r *affinityStatusReconciler) origin(
	tagName string,
	e vmopv1.VirtualMachineAffinityPolicyStatus) vmopv1.VirtualMachineAffinityPolicyOrigin {

	if r.owned != nil {
		if r.owned.Has(tagName) {
			return vmopv1.VirtualMachineAffinityPolicyOriginSelf
		}
		return vmopv1.VirtualMachineAffinityPolicyOriginPeer
	}

	if prev := r.vmCtx.VM.Status.Affinity; prev != nil {
		k := affinityPolicyKey(e)
		for _, p := range prev.Policies {
			if affinityPolicyKey(p) == k && p.Origin != "" {
				return p.Origin
			}
		}
	}
	return vmopv1.VirtualMachineAffinityPolicyOriginPeer
}

// withOrigin returns a copy of a previous entry, with Peer as the origin if
// the entry has none.
func withOrigin(
	prev vmopv1.VirtualMachineAffinityPolicyStatus) vmopv1.VirtualMachineAffinityPolicyStatus {

	e := *prev.DeepCopy()
	if e.Origin == "" {
		e.Origin = vmopv1.VirtualMachineAffinityPolicyOriginPeer
	}
	return e
}

// affinityPolicyKey returns the identity of the policy an entry describes,
// which is independent of the policy ID and compliance.
func affinityPolicyKey(e vmopv1.VirtualMachineAffinityPolicyStatus) string {
	var sb strings.Builder
	sb.WriteString(string(e.Type))
	sb.WriteByte('|')
	sb.WriteString(string(e.Strictness))
	sb.WriteByte('|')
	sb.WriteString(e.TopologyKey)
	for _, t := range e.Targets {
		sb.WriteByte('|')
		sb.WriteString(t.Key)
		sb.WriteByte('=')
		sb.WriteString(t.Value)
	}
	return sb.String()
}
