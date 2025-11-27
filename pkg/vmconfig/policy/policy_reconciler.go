// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

// ExtraConfigPolicyTagsKey is the ExtraConfig key that contains the tag
// UUIDs that we have applied from the VM's PolicyEvaluation, so can later
// only remove tags that we have applied.
const ExtraConfigPolicyTagsKey = "vmservice.policy.tags"

// ErrPolicyNotReady is returned by the reconciler when the evaluated policy is
// not yet ready.
var ErrPolicyNotReady = pkgerr.NoRequeueNoErr("policy not ready")

// Reconcile configures the VM's Policies and Tags.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

func New() vmconfig.Reconciler {
	return reconciler{}
}

func (r reconciler) Name() string {
	return "policy"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}
	restClient := pkgctx.GetRestClient(ctx)
	if restClient == nil {
		panic("restClient is nil")
	}

	results, err := getPolicyEvaluationResults(
		ctx,
		k8sClient,
		vm,
		moVM,
		*configSpec)
	if err != nil {
		return fmt.Errorf("failed to evaluate policy: %w", err)
	}

	var (
		are      []string // tags that are attached to the VM
		haveBeen []string // tags that have been attached to the VM
		shouldBe []string // tags that should be attached to the VM
		toAdd    []string // tags to attach to the VM
		toRem    []string // tags to detach from the VM

		logger = pkglog.FromContextOrDefault(ctx)
	)

	shouldBe = getTagsFromPolicyEvaluationResults(results...)

	if moVM.Config != nil {
		are = pkgctx.GetVMTags(ctx)
		ec := object.OptionValueList(moVM.Config.ExtraConfig)
		if v, _ := ec.GetString(ExtraConfigPolicyTagsKey); v != "" {
			haveBeen = strings.Split(v, ",")
		}
	}

	for _, t := range shouldBe {
		if !slices.Contains(are, t) {
			toAdd = append(toAdd, t)
		}
	}

	for _, t := range are {
		if slices.Contains(haveBeen, t) && !slices.Contains(shouldBe, t) {
			toRem = append(toRem, t)
		}
	}

	logLevel := 4
	if len(toAdd) > 0 || len(toRem) > 0 {
		logLevel = 2
	}
	logger.V(logLevel).Info(
		"Calculated tags",
		"are", are,
		"haveBeen", haveBeen,
		"shouldBe", shouldBe,
		"toAdd", toAdd,
		"toRem", toRem)

	if moVM.Config == nil {
		for _, tag := range toAdd {
			configSpec.TagSpecs = append(configSpec.TagSpecs, vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
					Operation: vimtypes.ArrayUpdateOperationAdd,
				},
				Id: vimtypes.TagId{
					Uuid: tag,
				},
			})
		}

		configSpec.ExtraConfig = append(configSpec.ExtraConfig,
			&vimtypes.OptionValue{
				Key:   ExtraConfigPolicyTagsKey,
				Value: strings.Join(toAdd, ","),
			},
		)
	} else {
		mgr := tags.NewManager(restClient)

		if len(toRem) > 0 {
			if err := mgr.DetachMultipleTagsFromObject(
				ctx,
				toRem,
				moVM.Reference()); err != nil {

				return fmt.Errorf("failed to detach tags from vm: %w", err)
			}
		}

		if len(toAdd) > 0 {
			if err := mgr.AttachMultipleTagsToObject(
				ctx,
				toAdd,
				moVM.Reference()); err != nil {

				return fmt.Errorf("failed to attach tags to vm: %w", err)
			}
		}

		if len(toAdd) > 0 || len(toRem) > 0 {
			var (
				ec    = object.OptionValueList(moVM.Config.ExtraConfig)
				ev    = strings.Join(shouldBe, ",")
				av, _ = ec.GetString(ExtraConfigPolicyTagsKey)
			)

			if av != ev {
				configSpec.ExtraConfig = append(configSpec.ExtraConfig,
					&vimtypes.OptionValue{
						Key:   ExtraConfigPolicyTagsKey,
						Value: ev,
					},
				)
			}
		} else {
			// Only update the status if there are no changes.
			vm.Status.Policies = make([]vmopv1.PolicyStatus, len(results))
			for i := range results {
				vm.Status.Policies[i].APIVersion = results[i].APIVersion
				vm.Status.Policies[i].Kind = results[i].Kind
				vm.Status.Policies[i].Generation = results[i].Generation
				vm.Status.Policies[i].Name = results[i].Name
			}
		}
	}

	return nil
}
