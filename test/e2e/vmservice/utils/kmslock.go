// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
)

const (
	// defaultKMSProviderLockName and defaultKMSProviderLockNamespace back a
	// Lease used to serialize mutation of vCenter's default KMS provider
	// (CryptoManagerKmip.SetDefaultKmsCluster/GetDefaultKmsCluster called
	// with entity=nil) across concurrently-running E2E jobs that target the
	// same vCenter appliance — that setting is VC-wide, not scoped per
	// namespace, session, or test.
	//
	// The lock lives in the target supervisor cluster rather than in
	// vCenter itself because only jobs pointed at the same supervisor
	// cluster/vCenter share that cluster's API server — exactly the set of
	// jobs capable of racing on that vCenter's global setting.
	defaultKMSProviderLockName      = "vmservice-e2e-default-kms-provider-lock"
	defaultKMSProviderLockNamespace = "vmware-system-vmop"

	// defaultKMSProviderLockLeaseDuration bounds how long a holder that
	// stops renewing (crash, panic, hard timeout) can block other jobs. A
	// live holder never approaches this: it renews on
	// defaultKMSProviderLockRenewInterval, well inside this window,
	// regardless of how long its actual test work takes. Keeping this short
	// and fixed - rather than derived from the suite's --timeout - means a
	// dead holder is reclaimed in roughly this long, not in however long
	// the suite's overall timeout happens to be.
	defaultKMSProviderLockLeaseDuration = 90 * time.Second

	// defaultKMSProviderLockRenewInterval is how often a live holder
	// refreshes the Lease's RenewTime. It must comfortably clear
	// defaultKMSProviderLockLeaseDuration so transient API server hiccups
	// don't cause a live holder to be mistaken for a dead one.
	defaultKMSProviderLockRenewInterval = 30 * time.Second
)

// AcquireDefaultKMSProviderLock blocks until this process holds the
// exclusive lock on vCenter's default KMS provider setting, then returns a
// function that releases it. Callers must arrange for the returned function
// to run (e.g. via DeferCleanup) even if the calling spec fails.
//
// While held, the lock is kept alive by a background renewal loop, so a
// live holder can run for as long as its test actually takes - the caller's
// timeout only bounds how long to wait to *acquire* the lock, not how long
// it may be held afterward.
func AcquireDefaultKMSProviderLock(
	ctx context.Context,
	c ctrlclient.Client,
	timeout time.Duration) func() {

	holder := fmt.Sprintf("%s-%d-%s", kmsLockHostname(), os.Getpid(), capiutil.RandomString(6))

	framework.Byf("Acquiring default KMS provider lock as %q", holder)
	Eventually(func(g Gomega) {
		g.Expect(tryAcquireKMSLease(ctx, c, holder)).To(Succeed())
	}, timeout, 10*time.Second).Should(Succeed(),
		"timed out waiting to acquire the default KMS provider lock (namespace %s, lease %s)",
		defaultKMSProviderLockNamespace, defaultKMSProviderLockName)

	stopRenewing := make(chan struct{})
	var renewWG sync.WaitGroup
	renewWG.Go(func() {
		renewKMSLeaseUntilStopped(ctx, c, holder, stopRenewing)
	})

	return func() {
		close(stopRenewing)
		renewWG.Wait()
		releaseKMSLease(ctx, c, holder)
	}
}

// renewKMSLeaseUntilStopped refreshes the Lease's RenewTime on
// defaultKMSProviderLockRenewInterval until stop is closed, keeping a live
// holder from being mistaken for a dead one no matter how long its actual
// work takes.
func renewKMSLeaseUntilStopped(ctx context.Context, c ctrlclient.Client, holder string, stop <-chan struct{}) {
	ticker := time.NewTicker(defaultKMSProviderLockRenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewKMSLease(ctx, c, holder)
		}
	}
}

// renewKMSLease refreshes the Lease's RenewTime if, and only if, it is still
// held by holder. A failure here (lost race, transient API error) is
// logged, not fatal: the next tick tries again, and if the Lease is
// genuinely gone or held by someone else, the caller's own work will
// eventually surface that as a correctness problem elsewhere rather than
// silently corrupting vCenter's global setting.
func renewKMSLease(ctx context.Context, c ctrlclient.Client, holder string) {
	key := ctrlclient.ObjectKey{Namespace: defaultKMSProviderLockNamespace, Name: defaultKMSProviderLockName}

	lease := &coordinationv1.Lease{}
	if err := c.Get(ctx, key, lease); err != nil {
		framework.Byf("WARNING: failed to read default KMS provider lock for renewal (holder %q): %v", holder, err)
		return
	}
	if ptr.DerefWithDefault(lease.Spec.HolderIdentity, "") != holder {
		framework.Byf("WARNING: default KMS provider lock no longer held by %q; another job may believe it holds it concurrently", holder)
		return
	}
	lease.Spec.RenewTime = ptr.To(metav1.NowMicro())
	// Update carries the Lease's resourceVersion, so a takeover that landed
	// between our Get and Update (e.g. this Lease was judged expired just
	// before this renewal) fails here with a conflict instead of silently
	// overwriting the new holder's claim.
	if err := c.Update(ctx, lease); err != nil {
		framework.Byf("WARNING: failed to renew default KMS provider lock held by %q: %v", holder, err)
	}
}

// tryAcquireKMSLease makes a single attempt to create or take over the KMS
// provider lock's Lease. A non-nil error (including "lock held by
// <someone>") just means the caller should retry.
func tryAcquireKMSLease(ctx context.Context, c ctrlclient.Client, holder string) error {
	key := ctrlclient.ObjectKey{Namespace: defaultKMSProviderLockNamespace, Name: defaultKMSProviderLockName}

	lease := &coordinationv1.Lease{}
	err := c.Get(ctx, key, lease)
	switch {
	case apierrors.IsNotFound(err):
		return c.Create(ctx, newKMSLease(holder))
	case err != nil:
		return err
	case kmsLeaseExpired(lease):
		now := metav1.NowMicro()
		lease.Spec.HolderIdentity = ptr.To(holder)
		lease.Spec.AcquireTime = ptr.To(now)
		lease.Spec.RenewTime = ptr.To(now)
		lease.Spec.LeaseDurationSeconds = ptr.To(int32(defaultKMSProviderLockLeaseDuration.Seconds()))
		// Update carries the Lease's resourceVersion, so a concurrent
		// takeover attempt by another job fails with a conflict here rather
		// than both believing they hold the lock.
		return c.Update(ctx, lease)
	default:
		return fmt.Errorf("lock held by %q", ptr.DerefWithDefault(lease.Spec.HolderIdentity, "<unknown>"))
	}
}

// releaseKMSLease releases the lock if, and only if, it is still held by
// holder. Deleting the Lease (rather than clearing HolderIdentity) lets the
// next waiter acquire immediately instead of waiting out the lease
// duration.
func releaseKMSLease(ctx context.Context, c ctrlclient.Client, holder string) {
	key := ctrlclient.ObjectKey{Namespace: defaultKMSProviderLockNamespace, Name: defaultKMSProviderLockName}

	lease := &coordinationv1.Lease{}
	if err := c.Get(ctx, key, lease); err != nil {
		return
	}
	if ptr.DerefWithDefault(lease.Spec.HolderIdentity, "") != holder {
		// Lease already expired and another job reclaimed it.
		return
	}
	// Precondition the delete on the UID/resourceVersion we just read, so a
	// take-over that lands between our Get and Delete (e.g. another job
	// reclaiming an expired lease) fails this delete with a conflict instead
	// of deleting a Lease it doesn't hold.
	precondition := ctrlclient.Preconditions{UID: &lease.UID, ResourceVersion: &lease.ResourceVersion}
	if err := c.Delete(ctx, lease, precondition); err != nil {
		framework.Byf("WARNING: failed to release default KMS provider lock held by %q: %v", holder, err)
		return
	}
	framework.Byf("Released default KMS provider lock held by %q", holder)
}

func newKMSLease(holder string) *coordinationv1.Lease {
	now := metav1.NowMicro()
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultKMSProviderLockName,
			Namespace: defaultKMSProviderLockNamespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(holder),
			AcquireTime:          ptr.To(now),
			RenewTime:            ptr.To(now),
			LeaseDurationSeconds: ptr.To(int32(defaultKMSProviderLockLeaseDuration.Seconds())),
		},
	}
}

func kmsLeaseExpired(lease *coordinationv1.Lease) bool {
	renew := lease.Spec.RenewTime
	duration := ptr.DerefWithDefault(lease.Spec.LeaseDurationSeconds, 0)
	if renew == nil || duration <= 0 {
		return true
	}
	return time.Since(renew.Time) > time.Duration(duration)*time.Second
}

func kmsLockHostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown-host"
}
