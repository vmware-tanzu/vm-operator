// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	clientrecord "k8s.io/client-go/tools/record"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
)

// NewControllerManagerContext returns a fake ControllerManagerContext for unit
// testing reconcilers and webhooks with a fake client.
func NewControllerManagerContext() *context.ControllerManagerContext {
	return &context.ControllerManagerContext{
		Context:                 pkgconfig.NewContext(),
		Logger:                  ctrllog.Log.WithName(ControllerManagerName),
		Namespace:               ControllerManagerNamespace,
		Name:                    ControllerManagerName,
		ServiceAccountName:      ServiceAccountName,
		LeaderElectionNamespace: LeaderElectionNamespace,
		LeaderElectionID:        LeaderElectionID,
		Recorder:                record.New(clientrecord.NewFakeRecorder(1024)),
		VMProviderA2:            providerfake.NewVMProviderA2(),
	}
}
