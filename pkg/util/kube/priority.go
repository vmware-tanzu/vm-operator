// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"

	"github.com/go-logr/logr"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pkgnil "github.com/vmware-tanzu/vm-operator/pkg/util/nil"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// EventType represents a Kubernetes event.
type EventType uint8

const (
	// EventCreate is the create event.
	EventCreate EventType = iota
	// EventUpdate is the update event.
	EventUpdate
	// EventDelete is the delete event.
	EventDelete
	// EventGeneric is the generic event.
	EventGeneric
)

func (e EventType) String() string {
	switch e {
	case EventCreate:
		return "create"
	case EventUpdate:
		return "update"
	case EventDelete:
		return "delete"
	case EventGeneric:
		return "generic"
	}
	return "unknown"
}

const (
	// PriorityGeneric is the priority for a generic event.
	PriorityGeneric = iota + 1
	// PriorityDelete is the priority for a delete event.
	PriorityDelete
	// PriorityUpdate is the priority for an update event.
	PriorityUpdate
	// PriorityCreate is the priority for a create event.
	PriorityCreate
)

// GetPriorityFn is a function that may be used with a mapper that allows the
// specification of a custom priority.
type GetPriorityFn[T client.Object] func(
	ctx context.Context,
	eventType EventType,
	newObj, oldObj T,
	defaultPriority int) int

var _ handler.EventHandler = &EnqueueRequestForObject{}

// EnqueueRequestForObject enqueues a Request containing the Name and Namespace
// of the object that is the source of the Event, e.g. the created / deleted /
// updated objects Name and Namespace).
type EnqueueRequestForObject = TypedEnqueueRequestForObject[client.Object]

// TypedEnqueueRequestForObject enqueues a Request containing the Name and
// Namespace of the object that is the source of the Event, e.g. the created /
// deleted / updated objects Name and Namespace).
type TypedEnqueueRequestForObject[T client.Object] struct {
	Logger logr.Logger

	// GetPriority returns the priority to use when submitting the object to the
	// queue.
	GetPriority GetPriorityFn[T]
}

// Create implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Create(
	ctx context.Context,
	evt event.TypedCreateEvent[T],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	if pkgnil.IsNil(evt.Object) {
		e.Logger.Error(
			nil,
			"CreateEvent received with no metadata",
			"event", evt)
		return
	}

	item := reconcile.Request{
		NamespacedName: apitypes.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		},
	}

	addToQueueCreate(
		logr.NewContext(ctx, e.Logger),
		q,
		evt,
		item,
		PriorityCreate,
		e.GetPriority)
}

// Update implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Update(
	ctx context.Context,
	evt event.TypedUpdateEvent[T],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	switch {
	case !pkgnil.IsNil(evt.ObjectNew):
		item := reconcile.Request{
			NamespacedName: apitypes.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			},
		}
		addToQueueUpdate(
			logr.NewContext(ctx, e.Logger),
			q,
			evt,
			item,
			PriorityUpdate,
			e.GetPriority)

	case !pkgnil.IsNil(evt.ObjectOld):
		item := reconcile.Request{
			NamespacedName: apitypes.NamespacedName{
				Name:      evt.ObjectOld.GetName(),
				Namespace: evt.ObjectOld.GetNamespace(),
			},
		}

		addToQueueUpdate(
			logr.NewContext(ctx, e.Logger),
			q,
			evt,
			item,
			PriorityUpdate,
			e.GetPriority)
	default:
		e.Logger.Error(
			nil,
			"UpdateEvent received with no metadata",
			"event", evt)
	}
}

// Delete implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Delete(
	ctx context.Context,
	evt event.TypedDeleteEvent[T],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	if pkgnil.IsNil(evt.Object) {
		e.Logger.Error(
			nil,
			"DeleteEvent received with no metadata",
			"event", evt)
		return
	}

	item := reconcile.Request{
		NamespacedName: apitypes.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		},
	}

	addToQueueDelete(
		logr.NewContext(ctx, e.Logger),
		q,
		evt,
		item,
		PriorityDelete,
		e.GetPriority)
}

// Generic implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Generic(
	ctx context.Context,
	evt event.TypedGenericEvent[T],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	if pkgnil.IsNil(evt.Object) {
		e.Logger.Error(
			nil,
			"GenericEvent received with no metadata",
			"event", evt)
		return
	}

	item := reconcile.Request{
		NamespacedName: apitypes.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		},
	}

	addToQueueGeneric(
		logr.NewContext(ctx, e.Logger),
		q,
		evt,
		item,
		PriorityGeneric,
		e.GetPriority)
}

// addToQueueCreate adds the reconcile.Request to the priorityqueue in the
// handler for Create requests if and only if the workqueue being used is of
// type priorityqueue.PriorityQueue[reconcile.Request].
func addToQueueCreate[T client.Object, request comparable](
	ctx context.Context,
	q workqueue.TypedRateLimitingInterface[request],
	evt event.TypedCreateEvent[T],
	item request,
	defaultPriority int,
	getPriorityFn GetPriorityFn[T]) {

	priorityQueue, isPriorityQueue := q.(priorityqueue.PriorityQueue[request])
	if !isPriorityQueue {
		q.Add(item)
		return
	}

	var priority *int
	if evt.IsInInitialList {
		priority = ptr.To(handler.LowPriority)
	} else {
		priority = getPriority(
			ctx,
			EventCreate,
			evt.Object, evt.Object,
			defaultPriority,
			getPriorityFn)
	}

	logr.FromContextOrDiscard(ctx).V(4).Info(
		"Adding to priority queue for event",
		"eventType", EventCreate,
		"priority", priority,
		"item", item)
	priorityQueue.AddWithOpts(priorityqueue.AddOpts{Priority: priority}, item)
}

// addToQueueUpdate adds the reconcile.Request to the priorityqueue in the
// handler for Update requests if and only if the workqueue being used is of
// type priorityqueue.PriorityQueue[reconcile.Request].
func addToQueueUpdate[T client.Object, request comparable](
	ctx context.Context,
	q workqueue.TypedRateLimitingInterface[request],
	evt event.TypedUpdateEvent[T],
	item request,
	defaultPriority int,
	getPriorityFn GetPriorityFn[T]) {

	priorityQueue, isPriorityQueue := q.(priorityqueue.PriorityQueue[request])
	if !isPriorityQueue {
		q.Add(item)
		return
	}

	var priority *int
	if evt.ObjectOld.GetResourceVersion() == evt.ObjectNew.GetResourceVersion() {
		priority = ptr.To(handler.LowPriority)
	} else {
		priority = getPriority(
			ctx,
			EventUpdate,
			evt.ObjectNew, evt.ObjectOld,
			defaultPriority,
			getPriorityFn)
	}

	logr.FromContextOrDiscard(ctx).V(4).Info(
		"Adding to priority queue for event",
		"eventType", EventUpdate,
		"priority", priority,
		"item", item)
	priorityQueue.AddWithOpts(priorityqueue.AddOpts{Priority: priority}, item)
}

// addToQueueDelete adds the reconcile.Request to the priorityqueue in the
// handler for Delete requests if and only if the workqueue being used is of
// type priorityqueue.PriorityQueue[reconcile.Request].
func addToQueueDelete[T client.Object, request comparable](
	ctx context.Context,
	q workqueue.TypedRateLimitingInterface[request],
	evt event.TypedDeleteEvent[T],
	item request,
	defaultPriority int,
	getPriorityFn GetPriorityFn[T]) {

	priorityQueue, isPriorityQueue := q.(priorityqueue.PriorityQueue[request])
	if !isPriorityQueue {
		q.Add(item)
		return
	}

	priority := getPriority(
		ctx,
		EventDelete,
		evt.Object, evt.Object,
		defaultPriority,
		getPriorityFn)

	logr.FromContextOrDiscard(ctx).V(4).Info(
		"Adding to priority queue for event",
		"eventType", EventDelete,
		"priority", priority,
		"item", item)
	priorityQueue.AddWithOpts(priorityqueue.AddOpts{Priority: priority}, item)
}

// addToQueueGeneric adds the reconcile.Request to the priorityqueue in the
// handler for Generic requests if and only if the workqueue being used is of
// type priorityqueue.PriorityQueue[reconcile.Request].
func addToQueueGeneric[T client.Object, request comparable](
	ctx context.Context,
	q workqueue.TypedRateLimitingInterface[request],
	evt event.TypedGenericEvent[T],
	item request,
	defaultPriority int,
	getPriorityFn GetPriorityFn[T]) {

	priorityQueue, isPriorityQueue := q.(priorityqueue.PriorityQueue[request])
	if !isPriorityQueue {
		q.Add(item)
		return
	}

	priority := getPriority(
		ctx,
		EventGeneric,
		evt.Object, evt.Object,
		defaultPriority,
		getPriorityFn)

	logr.FromContextOrDiscard(ctx).V(4).Info(
		"Adding to priority queue for event",
		"eventType", EventGeneric,
		"priority", priority,
		"item", item)
	priorityQueue.AddWithOpts(priorityqueue.AddOpts{Priority: priority}, item)
}

func getPriority[T client.Object](
	ctx context.Context,
	eventType EventType,
	newObj, oldObj T,
	defaultPriority int,
	getPriorityFn GetPriorityFn[T]) *int {

	var priority int

	if getPriorityFn == nil {
		priority = defaultPriority
	} else {
		priority = getPriorityFn(
			ctx,
			eventType,
			newObj,
			oldObj,
			defaultPriority)
	}

	return &priority
}
