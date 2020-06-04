// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	//nolint
	//. "github.com/onsi/ginkgo"
	//nolint
	//. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/context/fake"
)

// UnitTestContextForController is used for unit testing controllers.
type UnitTestContextForController struct {
	// context is the context.ControllerContext for being tested.
	context.ControllerContext

	// Key may be used to lookup Ctx.Cluster with Ctx.Client.Get.
	Key client.ObjectKey

	// reconciler is the builder.Reconciler being unit tested.
	reconciler builder.Reconciler
}

// UnitTestContextForValidatingWebhook is used for unit testing validating webhooks.
type UnitTestContextForValidatingWebhook struct {
	// WebhookRequestContext is initialized with fake.NewWebhookRequestContext
	// and is used for unit testing.
	context.WebhookRequestContext

	// Key may be used to lookup Ctx.Obj with Ctx.Client.Get.
	Key client.ObjectKey

	// validator is the builder.Validator being unit tested.
	builder.Validator
}

// NewUnitTestContextForController returns a new UnitTestContextForController
// for unit testing controllers.
func NewUnitTestContextForController(newReconcilerFn builder.NewReconcilerFunc, initObjects []runtime.Object) *UnitTestContextForController {
	fakeClient, scheme := NewFakeClient(initObjects...)
	fakeManagerContext := fake.NewControllerManagerContext(fakeClient, scheme)
	fakeControllerContext := fake.NewControllerContext(fakeManagerContext)
	//GuestClusterContext: *(fake.NewGuestClusterContext(fake.NewClusterContext(fakeControllerContext)))

	reconciler := newReconcilerFn(fakeClient)

	ctx := &UnitTestContextForController{
		ControllerContext: *(fakeControllerContext),
		reconciler:        reconciler,
	}
	//ctx.Key = client.ObjectKey{Namespace: ctx.Cluster.Namespace, Name: ctx.Cluster.Name}

	return ctx
}

// NewUnitTestContextForValidatingWebhook returns a new
// UnitTestContextForValidatingWebhook for unit testing validating webhooks.
func NewUnitTestContextForValidatingWebhook(
	validator builder.Validator,
	obj, oldObj *unstructured.Unstructured,
	initObjects ...runtime.Object) *UnitTestContextForValidatingWebhook {

	fakeClient, scheme := NewFakeClient(initObjects...)
	fakeManagerContext := fake.NewControllerManagerContext(fakeClient, scheme)
	fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

	ctx := &UnitTestContextForValidatingWebhook{
		WebhookRequestContext: *(fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)),
		Key:                   client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()},
		Validator:             validator,
	}

	return ctx
}

// UnitTestContextForMutatingWebhook is used for unit testing mutating webhooks.
type UnitTestContextForMutatingWebhook struct {
	// WebhookRequestContext is initialized with fake.NewWebhookRequestContext
	// and is used for unit testing.
	context.WebhookRequestContext

	// Key may be used to lookup Ctx.Cluster with Ctx.Client.Get.
	Key client.ObjectKey

	// mutator is the builder.Mutator being unit tested.
	builder.Mutator
}

// NewUnitTestContextForMutatingWebhook returns a new UnitTestContextForMutatingWebhook for unit testing mutating webhooks.
func NewUnitTestContextForMutatingWebhook(
	mutator builder.Mutator,
	obj *unstructured.Unstructured) *UnitTestContextForMutatingWebhook {

	fakeClient, scheme := NewFakeClient()
	fakeManagerContext := fake.NewControllerManagerContext(fakeClient, scheme)
	fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

	ctx := &UnitTestContextForMutatingWebhook{
		WebhookRequestContext: *(fake.NewWebhookRequestContext(fakeWebhookContext, obj, nil)),
		Key:                   client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()},
		Mutator:               mutator,
	}

	return ctx
}

func NewFakeClient(initObjects ...runtime.Object) (client.Client, *runtime.Scheme) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = vmopv1.AddToScheme(scheme)
	_ = cnsv1alpha1.AddToScheme(scheme)
	_ = netopv1alpha1.AddToScheme(scheme)

	fakeClient := clientfake.NewFakeClientWithScheme(scheme, initObjects...)

	return fakeClient, scheme
}

// ReconcileNormal manually invokes the ReconcileNormal method on the controller
func (ctx UnitTestContextForController) ReconcileNormal() error {
	//var err error
	switch ctx.reconciler.(type) {
	default:
		panic("Unexpected type")
		/*
			case builder.VirtualMachineReconciler:
				vmr, ok := ctx.reconciler.(builder.VirtualMachineReconciler)
				if !ok {
					panic("Unable to convert reconciler to VirtualMachineReconciler")
				}
				_, err = vmr.ReconcileNormal(...)
			case builder.GuestClusterReconciler:
				gcr, ok := ctx.reconciler.(builder.GuestClusterReconciler)
				if !ok {
					panic("Unable to convert reconciler to GuestClusterReconciler")
				}
				_, err = gcr.ReconcileNormal(&ctx.GuestClusterContext)
			case builder.ManagedClusterReconciler:
				mcr, ok := ctx.reconciler.(builder.ManagedClusterReconciler)
				if !ok {
					panic("Unable to convert reconciler to ManagedClusterReconciler")
				}
				_, err = mcr.ReconcileNormal(ctx.GuestClusterContext.ClusterContext)
		*/
	}
	//return err
}
