// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package exit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;patch

// Restart instructs the Deployment responsible for this pod to do a rollout,
// restarting all of the replicas. By using this method, at least one pod is
// always up and responsive.
func Restart(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	reason string) error {

	if ctx == nil {
		panic("ctx is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}

	logger := pkglog.FromContextOrDefault(ctx)

	config := pkgcfg.FromContext(ctx)

	var (
		obj    appsv1.Deployment
		objKey = ctrlclient.ObjectKey{
			Name:      config.DeploymentName,
			Namespace: config.PodNamespace,
		}
	)

	lastExitTime := time.Now().UTC().Format(time.RFC3339Nano)
	logger.Info(
		"Restarting pod",
		"reason", reason,
		"lastExitTime", lastExitTime,
		"deploymentKey", objKey.String())

	if err := k8sClient.Get(ctx, objKey, &obj); err != nil {
		return fmt.Errorf(
			"failed to get deployment %s while restarting pod: %w",
			objKey, err)
	}

	patch := ctrlclient.StrategicMergeFrom(obj.DeepCopy())

	tplSpecAnno := obj.Spec.Template.Annotations
	if tplSpecAnno == nil {
		tplSpecAnno = map[string]string{}
	}

	tplSpecAnno[pkgconst.LastRestartTimeAnnotationKey] = lastExitTime
	tplSpecAnno[pkgconst.LastRestartReasonAnnotationKey] = reason
	obj.Spec.Template.Annotations = tplSpecAnno

	if err := k8sClient.Patch(ctx, &obj, patch); err != nil {
		return fmt.Errorf(
			"failed to patch deployment %s while restarting pod: %w",
			objKey, err)
	}

	return nil
}

// RestartSignal is the signal that causes the pod to be restarted.
const RestartSignal = syscall.SIGUSR2

// RestartSignalHandler waits to receive a signal that tells the pod to restart.
type RestartSignalHandler interface {
	Close()
	Closed() <-chan struct{}
}

type restartSignalHandler struct {
	c chan os.Signal
	d chan struct{}
	o sync.Once
}

// Closed returns a channel that is closed when the signal handler is no longer
// monitoring for a signal.
func (h *restartSignalHandler) Closed() <-chan struct{} {
	return h.d
}

// Close closes the signal handler and stops receiving signals. This function
// is idempotent and may be called multiple times.
func (h *restartSignalHandler) Close() {
	h.o.Do(func() {
		signal.Stop(h.c)
		close(h.c)
		close(h.d)
	})
}

// NewRestartSignalHandler returns a new signal handler that waits to receive
// a SIGUSR2 signal, upon which the pod is restarted via the package's
// Restart(context.Context, ctrlclient.Client, string) error method.
// Calling the handler's Close() method will stop the signal handler.
func NewRestartSignalHandler(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	elected <-chan struct{}) RestartSignalHandler {

	if ctx == nil {
		panic("ctx is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if elected == nil {
		panic("elected is nil")
	}

	h := &restartSignalHandler{
		c: make(chan os.Signal, 1),
		d: make(chan struct{}),
	}

	logger := pkglog.FromContextOrDefault(ctx)

	// Receive the restart signal.
	signal.Notify(h.c, RestartSignal)

	go func() {
		for {
			// Wait for the signal.
			s := <-h.c

			if s == nil {
				logger.Info("Signal channel closed sans signal")
				h.Close()
				return
			}

			sigLogger := logger.WithValues("signal", s)
			sigLogger.Info("Received signal")

			select {
			case <-elected:
				// When the leader or when there is no leader election, proceed
				// with the restart.
				if err := Restart(
					ctx,
					k8sClient,
					fmt.Sprintf("received %s", s)); err != nil {

					sigLogger.Error(err, "failed to restart pod upon signal")
				}
			default:
				// When not the leader, do nothing. The leader is responsible
				// restarting the pod(s).
				sigLogger.Info("Ignore signal for non-leader")
			}
		}
	}()

	return h
}
