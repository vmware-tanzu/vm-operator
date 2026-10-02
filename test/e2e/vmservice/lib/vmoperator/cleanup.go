// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
)

// DumpFunc captures diagnostic state, such as the output of kubectl describe,
// for a spec that has failed. A DumpFunc should log problems rather than
// assert, so that a failure to dump never prevents the cleanup that follows.
type DumpFunc func(ctx context.Context)

// DescribeResource returns a DumpFunc that logs the output of
// `kubectl describe <resource> -n <ns> <name>`.
func DescribeResource(kubeconfigPath, resource, ns, name string) DumpFunc {
	return func(ctx context.Context) {
		describeResource(ctx, kubeconfigPath, resource, ns, name)
	}
}

// DeferCleanupWithDumpOnFailure registers a Ginkgo DeferCleanup node that,
// if the current spec has failed, runs each dump and then runs cleanup.
// Dumping and cleaning up in the same node guarantees the resources are
// described before they are deleted.
//
// The cleanup may be nil, in which case only the dumps run. This is useful
// when cleanup is being skipped, e.g. input.SkipCleanup, since the dumps
// should still run.
//
// Call this right after the resources are created, before waiting on them,
// so that a failed wait still produces a dump. Because DeferCleanup nodes run
// in LIFO order, the resources are dumped and cleaned up before anything that
// was registered earlier, such as the namespace or VM class they depend on.
//
// Note that DeferCleanup nodes run after all AfterEach nodes, so a resource
// must not also be deleted in an AfterEach or it will be gone before the dump.
func DeferCleanupWithDumpOnFailure(
	cleanup func(ctx context.Context),
	dumps ...DumpFunc) {

	GinkgoHelper()

	DeferCleanup(func(ctx context.Context) {
		if CurrentSpecReport().Failed() {
			for _, dump := range dumps {
				dump(ctx)
			}
		}

		if cleanup != nil {
			cleanup(ctx)
		}
	})
}

// DeferCleanupVirtualMachine registers a DeferCleanup node that describes the
// VirtualMachine, along with anything in extraDumps, if the current spec has
// failed, and then deletes the VirtualMachine and waits for it to be gone.
// Deleting the VirtualMachine is a no-op if it does not exist, so this may be
// called before the VirtualMachine is created. When skipCleanup is true, the
// dumps still run but the VirtualMachine is not deleted.
func DeferCleanupVirtualMachine(
	config *config.E2EConfig,
	client ctrlclient.Client,
	kubeconfigPath, ns, name string,
	skipCleanup bool,
	extraDumps ...DumpFunc) {

	GinkgoHelper()

	var cleanup func(context.Context)
	if !skipCleanup {
		cleanup = func(ctx context.Context) {
			DeleteVirtualMachineAndWait(ctx, config, client, ns, name)
		}
	}

	dumps := append([]DumpFunc{DescribeResource(kubeconfigPath, "vm", ns, name)}, extraDumps...)
	DeferCleanupWithDumpOnFailure(cleanup, dumps...)
}

// describeResource logs the output of `kubectl describe` for the given
// resource. It never fails the spec: a resource that does not exist, or an
// error running kubectl, is logged and otherwise ignored.
func describeResource(ctx context.Context, kubeconfigPath, resource, ns, name string) {
	stdout, stderr, err := framework.KubectlDescribeWithNamespacedName(ctx, kubeconfigPath, resource, ns, name)
	if bytes.Contains(stderr, []byte("NotFound")) {
		e2eframework.Logf("Skip kubectl describe output as the resource %s '%s/%s' doesn't exist", resource, ns, name)
		return
	}
	if err != nil {
		e2eframework.Logf("Failed to run kubectl describe for resource %s '%s/%s': %v: %s", resource, ns, name, err, stderr)
		return
	}

	e2eframework.Logf("kubectl describe %s -n %s %s:\n%s", resource, ns, name, stdout)
}
