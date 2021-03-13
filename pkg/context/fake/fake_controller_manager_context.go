// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	goctx "context"

	"k8s.io/apimachinery/pkg/runtime"
	clientrecord "k8s.io/client-go/tools/record"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
)

// NewControllerManagerContext returns a fake ControllerManagerContext for unit
// testing reconcilers and webhooks with a fake client.
func NewControllerManagerContext(scheme *runtime.Scheme) *context.ControllerManagerContext {
	return &context.ControllerManagerContext{
		Context:                 goctx.Background(),
		Logger:                  ctrllog.Log.WithName(ControllerManagerName),
		Scheme:                  scheme,
		Namespace:               ControllerManagerNamespace,
		Name:                    ControllerManagerName,
		LeaderElectionNamespace: LeaderElectionNamespace,
		LeaderElectionID:        LeaderElectionID,
		Recorder:                record.New(clientrecord.NewFakeRecorder(1024)),
		VmProvider:              providerfake.NewFakeVmProvider(),
	}
}
