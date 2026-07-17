// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"errors"
	"fmt"
	"text/template"
	"time"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/isohttp"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/keyboard"
)

// ErrBootstrapISO indicates boot commands were sent to bootstrap a VM from
// an ISO image. Like ErrBootstrapReconfigure/ErrBootstrapCustomize, it is a
// no-requeue, non-error sentinel.
var ErrBootstrapISO = pkgerr.NoRequeueNoErr("bootstrapped vm from iso")

const (
	// maxISOBootCommandsWait bounds the sum of every <waitN> token across
	// spec.bootstrap.iso.commands. Sending boot commands blocks the calling
	// goroutine for this long in the worst case, so commands whose total
	// wait time exceeds this are rejected before anything is sent, rather
	// than risking an unbounded block of the reconcile loop.
	maxISOBootCommandsWait = 120 * time.Second

	// isoBootstrapCompletionTimeout bounds how long the ephemeral HTTP
	// server Pod and Service are kept running, waiting for the VM to report
	// a primary IP, before being torn down regardless.
	isoBootstrapCompletionTimeout = 60 * time.Minute
)

// BootstrapISO performs the ISO automated-installation bootstrap for a VM.
//
// Every reconcile, it first ensures the ephemeral HTTP server used to serve
// spec.bootstrap.iso.assets is running. Once that server has an address, it
// sends spec.bootstrap.iso.commands via the virtual USB keyboard exactly
// once per generation of the spec (tracked via an annotation hash), then
// waits for the VM to report a primary IP (or a timeout to elapse) before
// tearing the HTTP server down and marking bootstrap complete.
func BootstrapISO(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM keyboard.ScanCodeSender,
	k8sClient ctrlclient.Client,
	isoSpec *vmopv1.VirtualMachineBootstrapISOSpec,
	bootstrapArgs *BootstrapArgs) error {

	if conditions.IsTrue(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced) {
		// Already fully synced and torn down -- never recreate the HTTP
		// server after that point.
		return nil
	}

	mgr := isohttp.NewManager(k8sClient, vmCtx.VM)

	address, port, ready, err := mgr.EnsureReady(vmCtx)
	if err != nil {
		reason := vmopv1.VirtualMachineBootstrapISOHTTPServerNotReadyReason
		var assetErr *isohttp.ErrAssetNotFound
		if errors.As(err, &assetErr) {
			reason = vmopv1.VirtualMachineBootstrapISOAssetNotFoundReason
		}
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced, reason, "%v", err)
		return fmt.Errorf("failed to ensure iso bootstrap http server: %w", err)
	}
	if !ready {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced,
			vmopv1.VirtualMachineBootstrapISOHTTPServerNotReadyReason,
			"waiting for the ephemeral bootstrap http server to be assigned an address")
		return pkgerr.RequeueError{After: 5 * time.Second}
	}

	currentHash, err := getVimTypeHash(isoSpec)
	if err != nil {
		return err
	}

	if currentHash != vmCtx.VM.Annotations[pkgconst.BootstrapISOHashAnnotationKey] {
		return sendISOBootCommands(vmCtx, vcVM, bootstrapArgs, isoSpec, address, port, currentHash)
	}

	if !isoBootstrapComplete(vmCtx) {
		return pkgerr.RequeueError{After: 5 * time.Minute}
	}

	if err := mgr.Teardown(vmCtx); err != nil {
		return fmt.Errorf("failed to tear down iso bootstrap http server: %w", err)
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced)
	return nil
}

// sendISOBootCommands renders and sends spec.bootstrap.iso.commands, then
// records currentHash so this only happens once per generation of the spec.
func sendISOBootCommands(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM keyboard.ScanCodeSender,
	bootstrapArgs *BootstrapArgs,
	isoSpec *vmopv1.VirtualMachineBootstrapISOSpec,
	address string,
	port int,
	currentHash string) error {

	renderTemplate := GetTemplateRenderFunc(vmCtx, bootstrapArgs, bootstrapServiceFuncMap(address, port))
	render := func(s string) string { return renderTemplate("bootstrap-iso-command", s) }

	tokens, err := keyboard.ParseCommands(isoSpec.Commands, render)
	if err != nil {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced,
			vmopv1.VirtualMachineBootstrapISOKeyboardSendFailedReason, "%v", err)
		return fmt.Errorf("failed to parse iso boot commands: %w", err)
	}

	if total := keyboard.TotalWait(tokens); total > maxISOBootCommandsWait {
		err := fmt.Errorf(
			"total wait time %s across spec.bootstrap.iso.commands exceeds the %s maximum",
			total, maxISOBootCommandsWait)
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced,
			vmopv1.VirtualMachineBootstrapISOKeyboardSendFailedReason, "%v", err)
		return err
	}

	if err := keyboard.SendCommands(vmCtx, vcVM, tokens); err != nil {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced,
			vmopv1.VirtualMachineBootstrapISOKeyboardSendFailedReason, "%v", err)
		return fmt.Errorf("failed to send iso boot commands: %w", err)
	}

	if vmCtx.VM.Annotations == nil {
		vmCtx.VM.Annotations = map[string]string{}
	}
	vmCtx.VM.Annotations[pkgconst.BootstrapISOHashAnnotationKey] = currentHash
	vmCtx.VM.Annotations[pkgconst.BootstrapISOStartedAtAnnotationKey] = time.Now().UTC().Format(time.RFC3339)

	conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineBootstrapISOSynced,
		"CommandsSent", "boot commands sent; waiting for installation to complete")

	return ErrBootstrapISO
}

// isoBootstrapComplete reports whether the VM has finished installing from
// the ISO, heuristically: either the VM has reported a primary IP, or the
// isoBootstrapCompletionTimeout has elapsed since boot commands were sent.
func isoBootstrapComplete(vmCtx pkgctx.VirtualMachineContext) bool {
	if net := vmCtx.VM.Status.Network; net != nil && (net.PrimaryIP4 != "" || net.PrimaryIP6 != "") {
		return true
	}

	startedAt, err := time.Parse(time.RFC3339, vmCtx.VM.Annotations[pkgconst.BootstrapISOStartedAtAnnotationKey])
	if err != nil {
		return false
	}

	return time.Since(startedAt) > isoBootstrapCompletionTimeout
}

// bootstrapServiceFuncMap returns the single-entry template.FuncMap adding
// V1Alpha6_BootstrapService, scoped to this render call only -- it is never
// merged into the shared function map GetTemplateRenderFunc's other callers
// (e.g. vAppConfig) use, since outside the ISO bootstrap path there is no
// ephemeral HTTP server address for it to resolve.
func bootstrapServiceFuncMap(address string, port int) template.FuncMap {
	return template.FuncMap{
		constants.V1alpha6BootstrapService: func() string {
			return fmt.Sprintf("%s:%d", address, port)
		},
	}
}
