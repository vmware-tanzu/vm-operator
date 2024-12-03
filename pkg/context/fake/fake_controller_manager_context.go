// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	clientrecord "k8s.io/client-go/tools/record"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
)

// NewControllerManagerContext returns a fake ControllerManagerContext for unit
// testing reconcilers and webhooks with a fake client.
func NewControllerManagerContext() *pkgctx.ControllerManagerContext {
	ctx := pkgcfg.NewContext()
	ctx = ovfcache.WithContext(ctx)

	return &pkgctx.ControllerManagerContext{
		Context:                 ctx,
		Logger:                  ctrllog.Log.WithName(ControllerManagerName),
		Namespace:               ControllerManagerNamespace,
		Name:                    ControllerManagerName,
		ServiceAccountName:      ServiceAccountName,
		LeaderElectionNamespace: LeaderElectionNamespace,
		LeaderElectionID:        LeaderElectionID,
		Recorder:                record.New(clientrecord.NewFakeRecorder(1024)),
		VMProvider:              providerfake.NewVMProvider(),
	}
}
