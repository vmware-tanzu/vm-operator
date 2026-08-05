// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"os"
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
	// across concurrently-running E2E jobs that target the same vCenter
	// appliance.
	//
	// vCenter's default KMS provider (CryptoManagerKmip.SetDefaultKmsCluster
	// / GetDefaultKmsCluster called with entity=nil) is a single, VC-wide
	// setting — it is not scoped per-namespace, per-session, or per-test.
	// Encryption specs save/restore it around each spec as if it were
	// exclusively theirs; when two E2E jobs run encryption specs
	// concurrently against the same vCenter, one job's cleanup can clear or
	// overwrite the value the other job just set and is relying on,
	// producing a spurious VirtualMachineEncryptionSynced=NoDefaultKeyProvider
	// failure.
	//
	// The lock is a Lease in the target supervisor cluster rather than
	// something stored in vCenter itself, because only jobs pointed at the
	// same supervisor cluster/vCenter share that cluster's API server —
	// which is exactly the set of jobs capable of racing on that vCenter's
	// global setting.
	defaultKMSProviderLockName      = "vmservice-e2e-default-kms-provider-lock"
	defaultKMSProviderLockNamespace = "vmware-system-vmop"

	// defaultKMSProviderLockLeaseDuration bounds how long a lock holder that
	// crashes or panics without releasing can block other jobs. It must
	// exceed the slowest encryption spec's full BeforeEach+It+AfterEach
	// runtime.
	defaultKMSProviderLockLeaseDuration = 15 * time.Minute
)

// AcquireDefaultKMSProviderLock blocks until this process holds the
// exclusive lock on vCenter's default KMS provider setting, then returns a
// function that releases it. Callers must arrange for the returned function
// to run (e.g. via DeferCleanup) even if the calling spec fails.
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

	return func() {
		releaseKMSLease(ctx, c, holder)
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
		// Our lease already expired and another job reclaimed it; nothing
		// to release.
		return
	}
	if err := c.Delete(ctx, lease); err != nil {
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
