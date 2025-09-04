// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// FromContextOrDefault returns a Logger from ctx. If no Logger is found, this
// returns the default controller-runtime logger so we at least don't accidentally
// discard logs. Prefer using this over logr.FromContextOrDiscard().
func FromContextOrDefault(ctx context.Context) logr.Logger {
	if logger, err := logr.FromContext(ctx); err == nil {
		return logger
	}
	return ctrl.Log.WithName("DEFAULT")
}

// ControllerLogConstructor returns the controller-runtime log constructor for a controller. It is
// similar to the default constructor, but doesn't add duplicated values - like separate namespace
// and name values - to reduce log line length.
func ControllerLogConstructor(
	controllerName string,
	object runtime.Object,
	scheme *runtime.Scheme,
) func(request *reconcile.Request) logr.Logger {

	var (
		gvk    schema.GroupVersionKind
		hasGVK bool
	)

	if object != nil {
		var err error
		gvk, err = apiutil.GVKForObject(object, scheme)
		hasGVK = err == nil
	}

	return func(in *reconcile.Request) logr.Logger {
		log := ctrl.Log.WithName(controllerName)

		if req, ok := any(in).(*reconcile.Request); ok && req != nil {
			if hasGVK {
				log = log.WithValues(gvk.Kind, klog.KRef(req.Namespace, req.Name))
			} else {
				log = log.WithValues("namespace", req.NamespacedName, "name", req.Name)
			}
		}
		return log
	}
}