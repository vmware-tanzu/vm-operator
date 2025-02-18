// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package exit_test

import (
	"context"
	"fmt"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgexit "github.com/vmware-tanzu/vm-operator/pkg/exit"
)

var _ = DescribeTable("Restart",
	func(nilCtx, nilClient, errOnGet, errOnPatch bool) {

		var (
			client    ctrlclient.Client
			withFuncs interceptor.Funcs
			config    = pkgcfg.Default()
			ctx       = pkgcfg.WithConfig(config)
			numExits  = 10
			obj       = appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: config.PodNamespace,
					Name:      config.DeploymentName,
				},
			}
			objKey              = ctrlclient.ObjectKeyFromObject(&obj)
			uniqueLastExitTimes = map[time.Time]struct{}{}
		)

		if errOnGet {
			withFuncs.Get = func(
				_ context.Context,
				_ ctrlclient.WithWatch,
				_ ctrlclient.ObjectKey,
				_ ctrlclient.Object,
				_ ...ctrlclient.GetOption) error {

				return fmt.Errorf("fake")
			}
		}
		if errOnPatch {
			withFuncs.Patch = func(
				_ context.Context,
				_ ctrlclient.WithWatch,
				_ ctrlclient.Object,
				_ ctrlclient.Patch,
				_ ...ctrlclient.PatchOption) error {

				return fmt.Errorf("fake")
			}
		}

		client = fake.NewClientBuilder().
			WithInterceptorFuncs(withFuncs).
			WithObjects(&obj).
			Build()

		if nilCtx {
			ctx = nil
		}
		if nilClient {
			client = nil
		}

		fn := func() {
			exitErr := pkgexit.Restart(ctx, client, "")
			if errOnGet {
				Expect(exitErr).To(HaveOccurred())
				Expect(exitErr.Error()).To(HavePrefix("failed to get deployment"))
			} else if errOnPatch {
				Expect(exitErr).To(HaveOccurred())
				Expect(exitErr.Error()).To(HavePrefix("failed to patch deployment"))
			} else {
				Expect(client.Get(ctx, objKey, &obj)).To(Succeed())
				lastExitTimeStr := obj.Spec.Template.
					Annotations[pkgconst.LastRestartTimeAnnotationKey]

				Expect(lastExitTimeStr).ToNot(BeEmpty())
				lastExitTime, err := time.Parse(time.RFC3339Nano, lastExitTimeStr)
				Expect(err).ToNot(HaveOccurred())
				uniqueLastExitTimes[lastExitTime] = struct{}{}
			}
		}

		switch {
		case nilCtx:
			Expect(fn).To(PanicWith("ctx is nil"))
		case nilClient:
			Expect(fn).To(PanicWith("k8sClient is nil"))
		default:
			for i := 0; i < numExits; i++ {
				Expect(fn).ToNot(Panic())
			}
			switch {
			case errOnGet, errOnPatch:
				Expect(uniqueLastExitTimes).To(BeEmpty())
			default:
				Expect(uniqueLastExitTimes).To(HaveLen(numExits))
			}
		}
	},
	Entry(
		"should panic with nil context",
		true,  // nilCtx
		false, // nilClient
		false, // errOnGet
		false, // errOnPatch
	),
	Entry(
		"should panic with nil client",
		false, // nilCtx
		true,  // nilClient
		false, // errOnGet
		false, // errOnPatch
	),
	Entry(
		"should restart",
		false, // nilCtx
		false, // nilClient
		false, // errOnGet
		false, // errOnPatch
	),
	Entry(
		"get deployment error",
		false, // nilCtx
		false, // nilClient
		true,  // errOnGet
		false, // errOnPatch
	),
	Entry(
		"patch deployment error",
		false, // nilCtx
		false, // nilClient
		false, // errOnGet
		true,  // errOnPatch
	),
)

var _ = DescribeTable("RestartSignalHandler",
	Ordered,
	func(
		nilCtx,
		nilClient,
		nilElected,
		closeSansSig,
		isNotLeader,
		errOnRestart bool) {

		var (
			client    ctrlclient.Client
			withFuncs interceptor.Funcs
			elected   = make(chan struct{})
			config    = pkgcfg.Default()
			ctx       = pkgcfg.WithConfig(config)
			obj       = appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: config.PodNamespace,
					Name:      config.DeploymentName,
				},
			}
			objKey     = ctrlclient.ObjectKeyFromObject(&obj)
			sigHandler pkgexit.RestartSignalHandler
		)

		if errOnRestart {
			withFuncs.Patch = func(
				_ context.Context,
				_ ctrlclient.WithWatch,
				_ ctrlclient.Object,
				_ ctrlclient.Patch,
				_ ...ctrlclient.PatchOption) error {

				return fmt.Errorf("fake")
			}
		}

		client = fake.NewClientBuilder().
			WithInterceptorFuncs(withFuncs).
			WithObjects(&obj).
			Build()

		if nilCtx {
			ctx = nil
		}
		if nilClient {
			client = nil
		}
		if nilElected {
			elected = nil
		}

		fn := func() {
			sigHandler = pkgexit.NewRestartSignalHandler(ctx, client, elected)
		}

		switch {
		case nilCtx:
			Expect(fn).To(PanicWith("ctx is nil"))
		case nilClient:
			Expect(fn).To(PanicWith("k8sClient is nil"))
		case nilElected:
			Expect(fn).To(PanicWith("elected is nil"))
		default:
			Expect(fn).ToNot(Panic())

			switch {
			case closeSansSig:
				sigHandler.Close()
			case !isNotLeader:
				close(elected)
			}

			// Send this process the signal that causes the restart.
			Expect(syscall.Kill(syscall.Getpid(), pkgexit.RestartSignal)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(client.Get(ctx, objKey, &obj)).To(Succeed())
				lastExitTimeStr := obj.Spec.Template.
					Annotations[pkgconst.LastRestartTimeAnnotationKey]

				switch {
				case closeSansSig, isNotLeader, errOnRestart:
					g.Expect(lastExitTimeStr).To(BeEmpty())
				default:
					g.Expect(lastExitTimeStr).ToNot(BeEmpty())
				}

			}, 3*time.Second).Should(Succeed())

			// Close the signal handler.
			sigHandler.Close()

			Eventually(func(g Gomega) {
				g.Expect(sigHandler.Closed()).Should(BeClosed())
			}, 3*time.Second).Should(Succeed())
		}
	},
	Entry(
		"should panic with nil context",
		true,  // nilCtx
		false, // nilClient
		false, // nilElected
		false, // closeSansSig
		false, // isNotLeader
		false, // errOnRestart
	),
	Entry(
		"should panic with nil client",
		false, // nilCtx
		true,  // nilClient
		false, // nilElected
		false, // closeSansSig
		false, // isNotLeader
		false, // errOnRestart
	),
	Entry(
		"should panic with nil elected",
		false, // nilCtx
		false, // nilClient
		true,  // nilElected
		false, // closeSansSig
		false, // isNotLeader
		false, // errOnRestart
	),
	Entry(
		"should close sans signal",
		false, // nilCtx
		false, // nilClient
		false, // nilElected
		true,  // closeSansSig
		false, // isNotLeader
		false, // errOnRestart
	),
	Entry(
		"should receive signal and ignore",
		false, // nilCtx
		false, // nilClient
		false, // nilElected
		false, // closeSansSig
		true,  // isNotLeader
		false, // errOnRestart
	),
	Entry(
		"should receive signal and fail to restart",
		false, // nilCtx
		false, // nilClient
		false, // nilElected
		false, // closeSansSig
		false, // isNotLeader
		true,  // errOnRestart
	),
	Entry(
		"should receive signal and restart",
		false, // nilCtx
		false, // nilClient
		false, // nilElected
		false, // closeSansSig
		false, // isNotLeader
		false, // errOnRestart
	),
)
