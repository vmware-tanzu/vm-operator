// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	goctx "context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/context/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// UnitTestContext is used for general purpose unit testing.
type UnitTestContext struct {
	goctx.Context
	Client client.Client
	Scheme *runtime.Scheme
}

// NewUnitTestContext returns a new UnitTestContext
func NewUnitTestContext(initObjects ...runtime.Object) *UnitTestContext {
	fakeClient, scheme := NewFakeClient(initObjects...)
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

	// Key may be used to lookup Ctx.Obj with Ctx.Client.Get.
	Key client.ObjectKey

	// validator is the builder.Validator being unit tested.
	builder.Validator
}

// NewUnitTestContextForController returns a new UnitTestContextForController
// for unit testing controllers.
func NewUnitTestContextForController(initObjects []runtime.Object) *UnitTestContextForController {
	fakeClient, scheme := NewFakeClient(initObjects...)
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
	validator builder.Validator,
	obj, oldObj *unstructured.Unstructured,
	initObjects ...runtime.Object) *UnitTestContextForValidatingWebhook {

	_, scheme := NewFakeClient(initObjects...)
	fakeManagerContext := fake.NewControllerManagerContext(scheme)
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

	_, scheme := NewFakeClient()
	fakeManagerContext := fake.NewControllerManagerContext(scheme)
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
	_ = ncpv1alpha1.AddToScheme(scheme)
	_ = cnsv1alpha1.AddToScheme(scheme)
	_ = netopv1alpha1.AddToScheme(scheme)

	fakeClient := clientfake.NewFakeClientWithScheme(scheme, initObjects...)

	return fakeClient, scheme
}

func NewFakeRecorder() (record.Recorder, chan string) {
	fakeEventRecorder := clientgorecord.NewFakeRecorder(1024)
	recorder := record.New(fakeEventRecorder)
	return recorder, fakeEventRecorder.Events
}
