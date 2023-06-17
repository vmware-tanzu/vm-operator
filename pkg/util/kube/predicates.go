// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// VMForControllerPredicateOptions is used to configure the behavior of the
// predicate created with VMForControllerPredicate.
type VMForControllerPredicateOptions struct {
	MatchIfVMClassNotFound            bool
	MatchIfControllerNameFieldEmpty   bool
	MatchIfControllerNameFieldMissing bool
}

// VMForControllerPredicate returns a predicate.Predicate that filters
// VirtualMachine resources and returns only those that map to a
// VirtualMachineClass resource that uses the provided controller.
//
// Please note this predicate is schema agnostic and should work with all
// VM Operator API schema versions.
func VMForControllerPredicate(
	c client.Client,
	log logr.Logger,
	controllerName string,
	opts VMForControllerPredicateOptions) predicate.Predicate {

	return &vmForVMClassPredicate{
		client:         c,
		log:            log,
		controllerName: controllerName,
		opts:           opts,
	}
}

type vmForVMClassPredicate struct {
	client         client.Client
	log            logr.Logger
	controllerName string
	opts           VMForControllerPredicateOptions
}

// Create returns true if the Create event should be processed.
func (r *vmForVMClassPredicate) Create(e event.CreateEvent) bool {
	if ok, err := r.matches(e.Object); !ok {
		if err != nil {
			r.log.Error(
				err,
				"Failed to match object for VMForVMClassPredicate.CreateEvent")
		}
		return false
	}
	return true
}

// Delete returns true if the Delete event should be processed.
func (r *vmForVMClassPredicate) Delete(e event.DeleteEvent) bool {
	if ok, err := r.matches(e.Object); !ok {
		if err != nil {
			r.log.Error(
				err,
				"Failed to match object for VMForVMClassPredicate.DeleteEvent")
		}
		return false
	}
	return true
}

// Update returns true if the Update event should be processed.
func (r *vmForVMClassPredicate) Update(e event.UpdateEvent) bool {
	if ok, err := r.matches(e.ObjectNew); !ok {
		if err != nil {
			r.log.Error(
				err,
				"Failed to match object for VMForVMClassPredicate.UpdateEvent")
		}
		return false
	}
	return true
}

// Generic returns true if the Generic event should be processed.
func (r *vmForVMClassPredicate) Generic(e event.GenericEvent) bool {
	if ok, err := r.matches(e.Object); !ok {
		if err != nil {
			r.log.Error(
				err,
				"Failed to match object for VMForVMClassPredicate.GenericEvent")
		}
		return false
	}
	return true
}

func (r *vmForVMClassPredicate) matches(obj runtime.Object) (bool, error) {
	obj = obj.DeepCopyObject() // do not mutate the original

	vm, err := r.getVM(obj)
	if err != nil {
		return false, err
	}

	// Get name of the VM Class used by the VM.
	className, ok, err := unstructured.NestedString(
		vm.Object, "spec", "className")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("spec.className not found for %s", vm)
	}

	// Get the VM Class referenced by the VM.
	vmClass, err := r.getVMClass(
		vm.GroupVersionKind().GroupVersion(),
		vm.GetNamespace(),
		className)
	if err != nil {
		if apierrors.IsNotFound(err) && r.opts.MatchIfVMClassNotFound {
			return true, nil
		}
		return false, err
	}

	controllerName, ok, err := unstructured.NestedString(
		vmClass.Object,
		"spec",
		"controllerName")
	if err != nil { // Will be nil if field is not present
		return false, err
	}

	if !ok { // Field was not found.
		if !r.opts.MatchIfControllerNameFieldMissing {
			return false, errors.New("spec.controllerName field missing")
		}
		return true, nil
	}

	if controllerName == "" {
		if !r.opts.MatchIfControllerNameFieldEmpty {
			return false, errors.New("spec.controllerName field is empty")
		}
		return true, nil
	}

	if controllerName != r.controllerName {
		return false, fmt.Errorf(
			"spec.controllerName=%q, expected=%q",
			controllerName, r.controllerName)
	}

	return true, nil
}

func (r *vmForVMClassPredicate) getVM(
	inObj runtime.Object) (*unstructured.Unstructured, error) {

	// Controller-runtime does not assign an object's TypeMeta prior to sending
	// the object into a predicate. In order to ascertain the object's group,
	// version, and kind, we need to use the scheme's object mapper. The code
	// below gets all of the available group, version, kind combinations for
	// the provided object.
	gvks, unversioned, err := r.client.Scheme().ObjectKinds(inObj)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get group, version, & kinds for object: %w", err)
	}
	if unversioned {
		return nil, errors.New("object is unversioned")
	}

	// Update the inObj with one of the discovered GVKs. The correct version
	// is not important as all versions of VirtualMachine have a spec.className
	// field and all versions of VirtualMachineClass have a spec.controllerName
	// field.
	inObj.GetObjectKind().SetGroupVersionKind(gvks[0])

	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(inObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured: %w", err)
	}
	outObj := unstructured.Unstructured{Object: data}

	if kind := outObj.GetKind(); kind != "VirtualMachine" {
		return nil, fmt.Errorf("kind=%q, expected kind=VirtualMachine", kind)
	}

	return &outObj, nil
}

func (r *vmForVMClassPredicate) getVMClass(
	groupVersion schema.GroupVersion,
	namespace, name string) (*unstructured.Unstructured, error) {

	obj := unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   groupVersion.Group,
		Version: groupVersion.Version,
		Kind:    "VirtualMachineClass",
	})

	var (
		ctx = context.Background()
		key = client.ObjectKey{Name: name}
	)

	// First attempt to get the VM Class as a cluster-scoped resource and,
	// if that fails, attempt to get it as a namespaced resource.
	if err := r.client.Get(ctx, key, &obj); err != nil {
		key.Namespace = namespace
		if err := r.client.Get(ctx, key, &obj); err != nil {
			return nil, fmt.Errorf("failed to get VirtualMachineClass: %w", err)
		}
	}

	return &obj, nil
}
