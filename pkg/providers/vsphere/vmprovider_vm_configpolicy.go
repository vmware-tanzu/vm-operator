// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicy"
)

// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies,verbs=get;list

// verifyConfigPolicy rejects powering on a VM whose *actual* vSphere
// configuration -- moVM.Config, fetched earlier in this reconcile, not the
// VM's spec -- no longer complies with the namespace's
// VirtualMachineConfigPolicy for the VM's zone. It also sets vm's
// VirtualMachineConfigPolicyVerified condition to reflect that compliance.
// The condition is deleted -- not set to a status -- when the VM's zone has
// no governing policy or the VM is not subject to one (e.g. VM Class-derived
// config bypassing the policy via VMClassMode=AsPolicy).
//
// The admission webhook (webhooks/virtualmachine/validation) only ever
// evaluates a VM's desired spec, and only at create/update/power-on request
// time. It cannot catch the case where a VM was compliant when last
// reconfigured but the policy has since changed -- e.g. an extraConfig key
// that was allowed is now denied, or the namespace's cluster no longer
// supports the VM's hardware version. This is that second check, performed
// against the VM's real, current vSphere state immediately before the
// power-on transition this reconcile is about to make.
func (vs *vSphereVMProvider) verifyConfigPolicy(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) error {

	zoneName := vm.Labels[corev1.LabelTopologyZone]
	if zoneName == "" {
		// Not yet placed in a zone; the applicable policy cannot be
		// resolved. Enforcement resumes once placement sets the zone label.
		conditions.Delete(vm, vmopv1.VirtualMachineConfigPolicyVerified)
		return nil
	}

	policy, err := vs.getVMConfigPolicy(ctx, vm.Namespace, zoneName)
	if err != nil {
		return fmt.Errorf(
			"failed to get VirtualMachineConfigPolicy for zone %s: %w", zoneName, err)
	}
	if policy == nil {
		// No policy governs this zone.
		conditions.Delete(vm, vmopv1.VirtualMachineConfigPolicyVerified)
		return nil
	}

	if !configpolicy.AppliesToVM(policy.Spec, vm.Spec.ClassName) {
		// VMClassMode defaults to AsPolicy, which preserves pre-9.1 behavior:
		// VM Class-derived config bypasses the policy entirely, so the VM is
		// not subject to it.
		conditions.Delete(vm, vmopv1.VirtualMachineConfigPolicyVerified)
		return nil
	}

	var violation error
	if moVM.Config != nil {
		in := configpolicy.InputFromConfigInfo(*moVM.Config)
		violation = configpolicy.Validate(ctx, policy.Spec, in)
	}

	if violation == nil {
		conditions.MarkTrue(vm, vmopv1.VirtualMachineConfigPolicyVerified)
		return nil
	}

	conditions.MarkFalse(vm,
		vmopv1.VirtualMachineConfigPolicyVerified,
		vmopv1.VirtualMachineConfigPolicyNotVerifiedReason,
		"%s", violation)

	if policy.Spec.PowerOnMode != vimv1.VirtualMachineConfigPolicyModeDeny {
		// The policy allows powering on regardless of compliance.
		return nil
	}

	return vs.configPolicyDeniesPowerOn(policy, violation)
}

func (vs *vSphereVMProvider) configPolicyDeniesPowerOn(
	policy *vimv1.VirtualMachineConfigPolicy, violation error) error {

	return pkgerr.NoRequeueError{
		Message: fmt.Sprintf(
			"power-on denied by VirtualMachineConfigPolicy %s/%s: %s",
			policy.Namespace, policy.Name, violation),
	}
}

// getVMConfigPolicy returns the VirtualMachineConfigPolicy that governs
// zoneName, or nil if none exists. The Zone controller always names a
// zone's policy after the zone (controllers/infra/zone), so this is a
// single, informer-cached Get -- no List, no label selector.
func (vs *vSphereVMProvider) getVMConfigPolicy(
	ctx context.Context,
	namespace, zoneName string) (*vimv1.VirtualMachineConfigPolicy, error) {

	policy := &vimv1.VirtualMachineConfigPolicy{}
	key := ctrlclient.ObjectKey{Namespace: namespace, Name: zoneName}

	if err := vs.k8sClient.Get(ctx, key, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return policy, nil
}
