// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/bitmask"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto/internal"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// Reconcile configures the VM's crypto settings.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec ptrCfgSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

// New returns a new Reconciler for a VM's crypto state.
func New() vmconfig.ReconcilerWithContext {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "crypto"
}

func (r reconciler) WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		internal.ContextKeyValue,
		func() internal.State { return internal.State{} })
}

// SprintfStateNotSynced formats and returns the message for when the encryption
// state cannot be synced.
func SprintfStateNotSynced(op string, msgs ...string) string {
	if len(msgs) == 0 {
		return ""
	}
	var msg string
	switch len(msgs) {
	case 1:
		msg = msgs[0]
	case 2:
		msg = fmt.Sprintf("%s and %s", msgs[0], msgs[1])
	default:
		msg = fmt.Sprintf(
			"%s, and %s",
			strings.Join(msgs[:len(msgs)-1], ", "),
			msgs[len(msgs)-1])
	}
	return fmt.Sprintf("Must %s when %s vm", msg, op)
}

// Reason is the type used by reasons given to the condition
// vmopv1.VirtualMachineEncryptionSynced.
type Reason uint8

const (
	ReasonInternalError Reason = 1 << iota
	ReasonEncryptionClassNotFound
	ReasonEncryptionClassInvalid
	ReasonNoDefaultKeyProvider
	ReasonInvalidState
	ReasonInvalidChanges
	ReasonReconfigureError
	_reasonMax = 1 << (iota - 1)
)

func (r Reason) MaxValue() Reason {
	return _reasonMax
}

func (r Reason) StringValue() string {
	switch r {
	case ReasonInternalError:
		return "InternalError"
	case ReasonEncryptionClassNotFound:
		return "EncryptionClassNotFound"
	case ReasonEncryptionClassInvalid:
		return "EncryptionClassInvalid"
	case ReasonNoDefaultKeyProvider:
		return "NoDefaultKeyProvider"
	case ReasonInvalidState:
		return "InvalidState"
	case ReasonInvalidChanges:
		return "InvalidChanges"
	case ReasonReconfigureError:
		return "ReconfigureError"
	}
	return ""
}

func (r Reason) String() string {
	return bitmask.String(r)
}

func markEncryptionStateNotSynced(
	vm *vmopv1.VirtualMachine,
	op string,
	reason Reason,
	msgs ...string) {

	conditions.MarkFalse(
		vm,
		vmopv1.VirtualMachineEncryptionSynced,
		reason.String(),
		"%s", SprintfStateNotSynced(op, msgs...))
}
