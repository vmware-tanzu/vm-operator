// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
)

// reloginAction describes what the SOAP round tripper does after an
// authentication fault on the first attempt.
type reloginAction int

const (
	// actionPassThrough returns the fault without re-authenticating. Used for
	// login/logout/session bodies, where acting on the fault can recurse.
	actionPassThrough reloginAction = iota

	// actionReloginOnly re-authenticates but returns the original fault,
	// because replaying would target session-scoped server state that the
	// re-login just destroyed.
	actionReloginOnly

	// actionReloginAndReplay re-authenticates and replays the request once.
	actionReloginAndReplay
)

// reloginSOAP is a soap.RoundTripper wrapper that re-authenticates the session
// and replays the request in place when the first attempt faults with
// NotAuthenticated. It wraps the raw *soap.Client so that a re-login issued
// through the shared *session.Manager traverses the full round tripper chain,
// including this wrapper and any keepalive handler above it.
type reloginSOAP struct {
	rt     soap.RoundTripper
	keeper *sessionKeeper
}

// newReloginSOAP returns the inline re-login round tripper for the given
// underlying round tripper.
func newReloginSOAP(
	rt soap.RoundTripper,
	keeper *sessionKeeper) *reloginSOAP {

	return &reloginSOAP{
		rt:     rt,
		keeper: keeper,
	}
}

// RoundTrip implements soap.RoundTripper. On a NotAuthenticated fault it
// re-authenticates the session and, unless the request is excluded by the
// deny-list or the caller's context, replays it exactly once. Transport, TLS
// and HTTP errors, and faults other than NotAuthenticated, pass through
// untouched.
func (r *reloginSOAP) RoundTrip(
	ctx context.Context,
	req, res soap.HasFault) error {

	// Read the generation before the first attempt, not after the fault: a
	// login that lands while this call was in flight must still be observed.
	gen := r.keeper.soapGeneration()

	err := r.rt.RoundTrip(ctx, req, res)
	if err == nil || !soap.IsSoapFault(err) {
		return err
	}
	if !IsNotAuthenticatedError(err) {
		return err
	}

	trigger := fmt.Sprintf("%T", req)
	switch classifyReloginAction(ctx, req) {
	case actionPassThrough:
		// Re-entrancy guard: acting on these faults can recurse.
		return err
	case actionReloginOnly:
		if lerr := r.keeper.reloginSOAP(ctx, gen, trigger); lerr != nil {
			return errors.Join(err, lerr)
		}
		// The "This" of the request names session-scoped state that the
		// re-login destroyed, so replaying is guaranteed to fail. The
		// caller's own restart logic is the correct recovery.
		return err
	case actionReloginAndReplay:
		// Continue below to re-authenticate and replay the request.
	}

	if lerr := r.keeper.reloginSOAP(ctx, gen, trigger); lerr != nil {
		return errors.Join(err, lerr)
	}
	if cerr := resetResponse(res); cerr != nil {
		return err
	}
	return r.rt.RoundTrip(ctx, req, res)
}

// classifyReloginAction returns the action for the given request body. The
// deny-list below deliberately includes both how a session is established or
// torn down, and every request bound to session-scoped server state.
func classifyReloginAction(
	ctx context.Context,
	req soap.HasFault) reloginAction {

	switch req.(type) {
	// Re-entrancy guard: these are how a session is established or torn
	// down. Acting on their faults can recurse.
	case *methods.LoginBody,
		*methods.LoginByTokenBody,
		*methods.LoginExtensionByCertificateBody,
		*methods.LogoutBody,
		*methods.CloneSessionBody,
		*methods.SessionIsActiveBody:
		return actionPassThrough

	// The "This" of these requests names session-scoped state that the
	// re-login just destroyed. Replaying is guaranteed to fail; the
	// caller's own restart logic is the correct recovery. See
	// .sdd/specs/012-vc-client-inline-relogin/research.md section 4.
	case *methods.WaitForUpdatesExBody,
		*methods.WaitForUpdatesBody,
		*methods.CancelWaitForUpdatesBody,
		*methods.CreateFilterBody,
		*methods.DestroyPropertyFilterBody,
		*methods.DestroyPropertyCollectorBody,
		*methods.DestroyViewBody,
		*methods.ModifyListViewBody:
		return actionReloginOnly
	}

	// The watcher and other owners of session-scoped state opt out
	// explicitly. The deny-list above is the backstop for callers that
	// forgot to.
	if isNoReplay(ctx) {
		return actionReloginOnly
	}

	// Creation calls -- CreatePropertyCollector, CreateContainerView and
	// CreateListView -- are deliberately absent from the deny-list: they
	// build fresh state on the new session and are safe to replay.
	return actionReloginAndReplay
}

// resetResponse zeroes the response struct in place before a replay.
//
// This is not cosmetic: soap.Client.RoundTrip wraps and reports the fault from
// the response whenever one is present, and a successful second response has
// no fault element to overwrite the first attempt's. Without this, the caller
// would receive NotAuthenticated from a retry that actually succeeded. Zeroing
// the whole struct, rather than only the fault field, also drops any partial
// result and needs no field-name knowledge. It is safe because the generated
// method bodies pass distinct request and response values.
func resetResponse(res soap.HasFault) error {
	v := reflect.ValueOf(res)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("response is not a non-nil pointer: %T", res)
	}
	e := v.Elem()
	if e.Kind() != reflect.Struct || !e.CanSet() {
		return fmt.Errorf("response is not a settable struct: %T", res)
	}
	e.Set(reflect.Zero(e.Type()))
	return nil
}
