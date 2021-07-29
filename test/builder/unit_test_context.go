// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	goctx "context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/context/fake"
)

// UnitTestContext is used for general purpose unit testing.
type UnitTestContext struct {
	goctx.Context
	Client client.Client
	Scheme *runtime.Scheme
}

// NewUnitTestContext returns a new UnitTestContext
func NewUnitTestContext(initObjects ...client.Object) *UnitTestContext {
	fakeClient, scheme := NewFakeClientAndScheme(initObjects...)
	return &UnitTestContext{
		Context: goctx.Background(),
		Client:  fakeClient,
		Scheme:  scheme,
	}
}

// AfterEach should be invoked by ginkgo.AfterEach to cleanup
func (ctx *UnitTestContext) AfterEach() {
	// Nothing yet to do.
}

// UnitTestContextForController is used for unit testing controllers.
type UnitTestContextForController struct {
	// context is the context.ControllerManagerContext for being tested.
	context.ControllerManagerContext

	// Client is the k8s client to access resources.
	Client client.Client

	// reconciler is the reconcile.Reconciler being unit tested.
	Reconciler reconcile.Reconciler

	// Events is a channel that fake recorder records events to.
	Events chan string
}

// UnitTestContextForValidatingWebhook is used for unit testing validating webhooks.
type UnitTestContextForValidatingWebhook struct {
	// WebhookRequestContext is initialized with fake.NewWebhookRequestContext
	// and is used for unit testing.
	context.WebhookRequestContext

	// Client is the k8s client to access resources.
	Client client.Client

	// Key may be used to lookup Ctx.Obj with Ctx.Client.Get.
	Key client.ObjectKey

	// validator is the builder.Validator being unit tested.
	builder.Validator
}

// NewUnitTestContextForController returns a new UnitTestContextForController
// for unit testing controllers.
func NewUnitTestContextForController(initObjects []client.Object) *UnitTestContextForController {
	fakeClient, scheme := NewFakeClientAndScheme(initObjects...)
	fakeControllerManagerContext := fake.NewControllerManagerContext(scheme)
	recorder, events := NewFakeRecorder()
	fakeControllerManagerContext.Recorder = recorder
	ctx := &UnitTestContextForController{
		ControllerManagerContext: *fakeControllerManagerContext,
		Client:                   fakeClient,
		Events:                   events,
	}

	return ctx
}

// AfterEach should be invoked by ginkgo.AfterEach to cleanup
func (ctx *UnitTestContextForController) AfterEach() {
	// Nothing yet to do.
}

// NewUnitTestContextForValidatingWebhook returns a new
// UnitTestContextForValidatingWebhook for unit testing validating webhooks.
func NewUnitTestContextForValidatingWebhook(
	validatorFn builder.ValidatorFunc,
	obj, oldObj *unstructured.Unstructured,
	initObjects ...client.Object) *UnitTestContextForValidatingWebhook {

	fakeClient, scheme := NewFakeClientAndScheme(initObjects...)
	fakeManagerContext := fake.NewControllerManagerContext(scheme)
	fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

	ctx := &UnitTestContextForValidatingWebhook{
		WebhookRequestContext: *(fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)),
		Client:                fakeClient,
		Key:                   client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()},
		Validator:             validatorFn(fakeClient),
	}

	return ctx
}

// UnitTestContextForMutatingWebhook is used for unit testing mutating webhooks.
type UnitTestContextForMutatingWebhook struct {
	// WebhookRequestContext is initialized with fake.NewWebhookRequestContext
	// and is used for unit testing.
	context.WebhookRequestContext

	// Client is the k8s client to access resources.
	Client client.Client

	// Key may be used to lookup Ctx.Cluster with Ctx.Client.Get.
	Key client.ObjectKey

	// mutator is the builder.Mutator being unit tested.
	builder.Mutator
}

// NewUnitTestContextForMutatingWebhook returns a new UnitTestContextForMutatingWebhook for unit testing mutating webhooks.
func NewUnitTestContextForMutatingWebhook(
	mutatorFn builder.MutatorFunc,
	obj *unstructured.Unstructured) *UnitTestContextForMutatingWebhook {

	fakeClient, scheme := NewFakeClientAndScheme(DummyAvailabilityZone())
	fakeManagerContext := fake.NewControllerManagerContext(scheme)
	fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

	ctx := &UnitTestContextForMutatingWebhook{
		WebhookRequestContext: *(fake.NewWebhookRequestContext(fakeWebhookContext, obj, nil)),
		Client:                fakeClient,
		Key:                   client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()},
		Mutator:               mutatorFn(fakeClient),
	}

	return ctx
}
