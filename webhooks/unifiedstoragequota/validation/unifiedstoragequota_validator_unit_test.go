// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/context/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/unifiedstoragequota/validation"
)

const (
	dummyVMName        = "dummy-vm"
	dummyNamespaceName = "dummy-vm-namespace-for-webhook-validation"
)

func TestRequestedCapacityHandler_HandleCreate(t *testing.T) {
	type testCase struct {
		description    string
		vm             *vmopv1.VirtualMachine
		sc             *v1.StorageClass
		vmi            *vmopv1.VirtualMachineImage
		cvmi           *vmopv1.ClusterVirtualMachineImage
		interceptors   interceptor.Funcs
		dummyConverter *DummyConverter
		dummyDecoder   DummyDecoder
		allowed        bool
		status         int32
		expected       validation.RequestedCapacity
	}

	testCases := []testCase{
		{
			description: "Error converting from unstructured",
			vm:          builder.DummyVirtualMachine(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: true,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusBadRequest,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Storage Class not found",
			vm:          builder.DummyVirtualMachine(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Error getting storage class",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			interceptors: interceptor.Funcs{
				Get: func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*v1.StorageClass); ok {
						return errors.New("fake error")
					}

					return client.Get(ctx, key, obj, opts...)
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusInternalServerError,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Image is nil",
			vm:          &vmopv1.VirtualMachine{},
			sc:          builder.DummyStorageClass(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusBadRequest,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "VMI not found",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Error getting VMI",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			vmi:         builder.DummyVirtualMachineImage(builder.DummyVMIName),
			interceptors: interceptor.Funcs{
				Get: func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*vmopv1.VirtualMachineImage); ok {
						return errors.New("fake error")
					}

					return client.Get(ctx, key, obj, opts...)
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusInternalServerError,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "CVMI not found",
			vm: &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Image: &vmopv1.VirtualMachineImageRef{
						Kind: "ClusterVirtualMachineImage",
					},
				},
			},
			sc: builder.DummyStorageClass(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Error getting CVMI",
			vm: &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Image: &vmopv1.VirtualMachineImageRef{
						Kind: "ClusterVirtualMachineImage",
					},
				},
			},
			sc:   builder.DummyStorageClass(),
			cvmi: builder.DummyClusterVirtualMachineImage(builder.DummyVMIName),
			interceptors: interceptor.Funcs{
				Get: func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*vmopv1.ClusterVirtualMachineImage); ok {
						return errors.New("fake error")
					}

					return client.Get(ctx, key, obj, opts...)
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusInternalServerError,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Disks is empty in image status",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			vmi:         builder.DummyVirtualMachineImage(builder.DummyVMIName),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "Boot disk has empty capacity",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			vmi: &vmopv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: builder.DummyVMIName,
				},
				Status: vmopv1.VirtualMachineImageStatus{
					Name: builder.DummyVMIName,
					ProductInfo: vmopv1.VirtualMachineImageProductInfo{
						FullVersion: builder.DummyDistroVersion,
					},
					OSInfo: vmopv1.VirtualMachineImageOSInfo{
						Type: builder.DummyOSType,
					},
					Disks: []vmopv1.VirtualMachineImageDiskInfo{
						{
							Capacity: nil,
						},
					},
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "vm uses vmi",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			vmi: &vmopv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      builder.DummyVMIName,
					Namespace: dummyNamespaceName,
				},
				Status: vmopv1.VirtualMachineImageStatus{
					Name: builder.DummyVMIName,
					ProductInfo: vmopv1.VirtualMachineImageProductInfo{
						FullVersion: builder.DummyDistroVersion,
					},
					OSInfo: vmopv1.VirtualMachineImageOSInfo{
						Type: builder.DummyOSType,
					},
					Disks: []vmopv1.VirtualMachineImageDiskInfo{
						{
							Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed: true,
			status:  http.StatusOK,
			expected: validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			},
		},
		{
			description: "vm uses cvmi",
			vm: &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Image: &vmopv1.VirtualMachineImageRef{
						Name: builder.DummyVMIName,
						Kind: "ClusterVirtualMachineImage",
					},
				},
			},
			sc: builder.DummyStorageClass(),
			cvmi: &vmopv1.ClusterVirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: builder.DummyVMIName,
				},
				Status: vmopv1.VirtualMachineImageStatus{
					Name: builder.DummyVMIName,
					ProductInfo: vmopv1.VirtualMachineImageProductInfo{
						FullVersion: builder.DummyDistroVersion,
					},
					OSInfo: vmopv1.VirtualMachineImageOSInfo{
						Type: builder.DummyOSType,
					},
					Disks: []vmopv1.VirtualMachineImageDiskInfo{
						{
							Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed: true,
			status:  http.StatusOK,
			expected: validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			},
		},
		{
			description: "invalid image kind",
			vm: &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Image: &vmopv1.VirtualMachineImageRef{
						Name: builder.DummyVMIName,
						Kind: "INVALID",
					},
				},
			},
			sc: builder.DummyStorageClass(),
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusBadRequest,
			expected: validation.RequestedCapacity{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			tc.vm.Name = dummyVMName
			tc.vm.Namespace = dummyNamespaceName

			var objects []ctrlclient.Object
			if tc.sc != nil {
				objects = append(objects, tc.sc)
				tc.vm.Spec.StorageClass = tc.sc.Name
			}
			if tc.vmi != nil {
				objects = append(objects, tc.vmi)
			}
			if tc.cvmi != nil {
				objects = append(objects, tc.cvmi)
			}
			obj, _ := builder.ToUnstructured(tc.vm)

			var oldObj *unstructured.Unstructured

			fakeClient := builder.NewFakeClientWithInterceptors(tc.interceptors, objects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeWebhookRequestContext := fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter:      tc.dummyConverter,
				Decoder:        tc.dummyDecoder,
			}
			resp := fakeHandler.HandleCreate(fakeWebhookRequestContext)

			g := NewWithT(t)

			g.Expect(resp.Allowed).To(Equal(tc.allowed))
			g.Expect(resp.Result.Code).To(Equal(tc.status))

			g.Expect(resp.Capacity.String()).To(Equal(tc.expected.Capacity.String()))
			g.Expect(resp.StoragePolicyID).To(Equal(tc.expected.StoragePolicyID))
			g.Expect(resp.StorageClassName).To(Equal(tc.expected.StorageClassName))
		})
	}
}

func TestRequestedCapacityHandler_HandleUpdate(t *testing.T) {
	type testCase struct {
		description    string
		vm             *vmopv1.VirtualMachine
		sc             *v1.StorageClass
		bootDisk       *vmopv1.VirtualMachineAdvancedSpec
		oldBootDisk    *vmopv1.VirtualMachineAdvancedSpec
		interceptors   interceptor.Funcs
		dummyConverter *DummyConverter
		dummyDecoder   DummyDecoder
		allowed        bool
		status         int32
		expected       validation.RequestedCapacity
	}

	testCases := []testCase{
		{
			description: "error converting obj from unstructured",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter:    runtime.DefaultUnstructuredConverter,
				invocations:  0,
				shouldErr:    true,
				errThreshold: 0,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusBadRequest,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "error converting old obj from unstructured",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter:    runtime.DefaultUnstructuredConverter,
				invocations:  0,
				shouldErr:    true,
				errThreshold: 1,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusBadRequest,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "updated boot disk size is nil",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk:    nil,
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  true,
			status:   http.StatusOK,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "old boot disk size is nil, no change in capacity",
			vm:          dummyVMWithStatusVolumes(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: nil,
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  true,
			status:   http.StatusOK,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "old boot disk size is nil, no volumes",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: nil,
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed: true,
			status:  http.StatusOK,
			expected: validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			},
		},
		{
			description: "change in boot disk size, increase",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed: true,
			status:  http.StatusOK,
			expected: validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(5*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			},
		},
		{
			description: "change in boot disk size, decrease",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  true,
			status:   http.StatusOK,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "no change in boot disk size",
			vm:          builder.DummyVirtualMachine(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  true,
			status:   http.StatusOK,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "storage class not found",
			vm:          builder.DummyVirtualMachine(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusNotFound,
			expected: validation.RequestedCapacity{},
		},
		{
			description: "error getting storage class",
			vm:          builder.DummyVirtualMachine(),
			sc:          builder.DummyStorageClass(),
			bootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
			},
			oldBootDisk: &vmopv1.VirtualMachineAdvancedSpec{
				BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			interceptors: interceptor.Funcs{
				Get: func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {
					if _, ok := obj.(*v1.StorageClass); ok {
						return errors.New("fake error")
					}

					return client.Get(ctx, key, obj, opts...)
				},
			},
			dummyConverter: &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
				shouldErr: false,
			},
			dummyDecoder: DummyDecoder{
				decoder:   admission.NewDecoder(builder.NewScheme()),
				shouldErr: false,
			},
			allowed:  false,
			status:   http.StatusInternalServerError,
			expected: validation.RequestedCapacity{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			tc.vm.Name = dummyVMName
			tc.vm.Namespace = dummyNamespaceName
			tc.vm.Spec.Advanced = tc.bootDisk

			var objects []ctrlclient.Object
			if tc.sc != nil {
				objects = append(objects, tc.sc)
				tc.vm.Spec.StorageClass = tc.sc.Name
			}

			obj, _ := builder.ToUnstructured(tc.vm)

			var oldVM *vmopv1.VirtualMachine
			var oldObj *unstructured.Unstructured
			oldVM = tc.vm.DeepCopy()
			oldVM.Spec.Advanced = tc.oldBootDisk
			oldObj, _ = builder.ToUnstructured(oldVM)

			fakeClient := builder.NewFakeClientWithInterceptors(tc.interceptors, objects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeWebhookRequestContext := fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter:      tc.dummyConverter,
				Decoder:        tc.dummyDecoder,
			}
			resp := fakeHandler.HandleUpdate(fakeWebhookRequestContext)

			g := NewWithT(t)

			g.Expect(resp.Allowed).To(Equal(tc.allowed))
			g.Expect(resp.Result.Code).To(Equal(tc.status))

			g.Expect(resp.Capacity.String()).To(Equal(tc.expected.Capacity.String()))
			g.Expect(resp.StoragePolicyID).To(Equal(tc.expected.StoragePolicyID))
			g.Expect(resp.StorageClassName).To(Equal(tc.expected.StorageClassName))
		})
	}
}

func TestRequestedCapacityHandler_Handle(t *testing.T) {
	type testCase struct {
		description  string
		vm           *vmopv1.VirtualMachine
		oldVM        *vmopv1.VirtualMachine
		operation    admissionv1.Operation
		interceptors interceptor.Funcs
		allowed      bool
		status       int32
	}

	testCases := []testCase{
		{
			description: "Create - Error decoding raw obj",
			operation:   admissionv1.Create,
			allowed:     false,
			status:      http.StatusBadRequest,
		},
		{
			description: "Update - Error decoding raw obj",
			operation:   admissionv1.Update,
			allowed:     false,
			status:      http.StatusBadRequest,
		},
		{
			description: "Update - Error decoding raw old obj",
			vm:          builder.DummyVirtualMachine(),
			operation:   admissionv1.Update,
			allowed:     false,
			status:      http.StatusBadRequest,
		},
		{
			description: "Create",
			vm:          builder.DummyVirtualMachine(),
			operation:   admissionv1.Create,
			allowed:     true,
			status:      http.StatusOK,
		},
		{
			description: "Update",
			vm:          builder.DummyVirtualMachine(),
			oldVM:       builder.DummyVirtualMachine(),
			operation:   admissionv1.Update,
			allowed:     true,
			status:      http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			sc := builder.DummyStorageClass()
			vmi := builder.DummyVirtualMachineImage(builder.DummyVMIName)
			vmi.Namespace = dummyNamespaceName
			vmi.Status = vmopv1.VirtualMachineImageStatus{
				Disks: []vmopv1.VirtualMachineImageDiskInfo{
					{
						Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					},
				},
			}

			if tc.vm != nil {
				tc.vm.Name = dummyVMName
				tc.vm.Namespace = dummyNamespaceName
				tc.vm.Spec.StorageClass = sc.Name
			}

			objects := []ctrlclient.Object{
				sc,
				vmi,
			}

			fakeClient := builder.NewFakeClientWithInterceptors(tc.interceptors, objects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
					shouldErr: false,
				},
				Decoder: DummyDecoder{
					decoder:   admission.NewDecoder(builder.NewScheme()),
					shouldErr: false,
				},
			}

			var obj, oldObj []byte

			if tc.vm != nil {
				obj, _ = json.Marshal(tc.vm)
			}
			if tc.oldVM != nil {
				oldObj, _ = json.Marshal(tc.oldVM)
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tc.operation,
					Object: runtime.RawExtension{
						Raw: obj,
					},
					OldObject: runtime.RawExtension{
						Raw: oldObj,
					},
				},
			}

			resp := fakeHandler.Handle(req)

			g := NewWithT(t)

			g.Expect(resp.Allowed).To(Equal(tc.allowed))
			g.Expect(resp.Result.Code).To(Equal(tc.status))
		})
	}
}

func TestRequestedCapacityHandler_ServeHTTP(t *testing.T) {
	type errTestCase struct {
		description string
		req         *http.Request
		status      int32
	}

	errTestCases := []errTestCase{
		{
			description: "empty body should return BadRequest",
			req:         &http.Request{Body: nil},
			status:      http.StatusBadRequest,
		},
		{
			description: "NoBody should return BadRequest",
			req:         &http.Request{Body: http.NoBody},
			status:      http.StatusBadRequest,
		},
		{
			description: "undecodable body should return BadRequest",
			req: &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString("{")},
			},
			status: http.StatusBadRequest,
		},
		{
			description: "infinite body should return RequestEntityTooLarge",
			req: &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Method: http.MethodPost,
				Body:   nopCloser{Reader: rand.Reader},
			},
			status: http.StatusRequestEntityTooLarge,
		},
		{
			description: "wrong content-type should return BadRequest",
			req: &http.Request{
				Header: http.Header{"Content-Type": []string{"application/foo"}},
				Body:   nopCloser{Reader: bytes.NewBuffer(nil)},
			},
			status: http.StatusBadRequest,
		},
	}

	for _, tc := range errTestCases {
		t.Run(tc.description, func(t *testing.T) {
			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
					shouldErr: false,
				},
				Decoder: DummyDecoder{
					decoder:   admission.NewDecoder(builder.NewScheme()),
					shouldErr: false,
				},
			}
			resp := httptest.NewRecorder()

			fakeHandler.ServeHTTP(resp, tc.req)

			g := NewWithT(t)

			g.Expect(resp.Code).To(Equal(int(tc.status)))
		})
	}

	type testCase struct {
		description string
		operation   admissionv1.Operation
		status      int32
		expected    validation.RequestedCapacity
	}

	testCases := []testCase{
		{
			description: "should return correct response code and body on create",
			operation:   admissionv1.Create,
			status:      http.StatusOK,
			expected: validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			},
		},
		{
			description: "should return correct response code and body on update",
			operation:   admissionv1.Update,
			status:      http.StatusOK,
			expected:    validation.RequestedCapacity{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			sc := builder.DummyStorageClass()

			vm := builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = sc.Name

			oldVM := vm.DeepCopy()

			vmi := builder.DummyVirtualMachineImage(builder.DummyVMIName)
			vmi.Namespace = dummyNamespaceName
			vmi.Status = vmopv1.VirtualMachineImageStatus{
				Disks: []vmopv1.VirtualMachineImageDiskInfo{
					{
						Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					},
				},
			}

			objects := []ctrlclient.Object{
				sc,
				vmi,
			}

			fakeClient := builder.NewFakeClient(objects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
					shouldErr: false,
				},
				Decoder: DummyDecoder{
					decoder:   admission.NewDecoder(builder.NewScheme()),
					shouldErr: false,
				},
			}

			var obj, oldObj []byte

			obj, _ = json.Marshal(vm)
			oldObj, _ = json.Marshal(oldVM)

			ar := admissionv1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Request: &admissionv1.AdmissionRequest{
					Operation: tc.operation,
					Object: runtime.RawExtension{
						Raw: obj,
					},
					OldObject: runtime.RawExtension{
						Raw: oldObj,
					},
				},
			}

			body, _ := json.Marshal(ar)

			req := httptest.NewRequest(http.MethodPost, "/getrequestedcapacityforvirtualmachine", bytes.NewReader(body))
			req.Header.Add("Content-Type", "application/json")

			resp := httptest.NewRecorder()

			fakeHandler.ServeHTTP(resp, req)

			g := NewWithT(t)

			g.Expect(resp.Code).To(Equal(int(tc.status)))

			var actual *validation.RequestedCapacity
			respBody := resp.Body.String()
			err := json.Unmarshal([]byte(respBody), &actual)

			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(actual.Capacity.String()).To(Equal(tc.expected.Capacity.String()))
			g.Expect(actual.StoragePolicyID).To(Equal(tc.expected.StoragePolicyID))
			g.Expect(actual.StorageClassName).To(Equal(tc.expected.StorageClassName))
		})
	}
}

func TestRequestedCapacityHandler_WriteResponse(t *testing.T) {
	type testCase struct {
		description string
		err         error
		allowed     bool
		status      int32
	}

	testCases := []testCase{
		{
			description: "request not allowed should have correct reason and status code",
			err:         errors.New("bad request"),
			allowed:     false,
			status:      http.StatusBadRequest,
		},
		{
			description: "request allowed; should have correct status",
			allowed:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			testResponse := validation.CapacityResponse{}

			if tc.allowed {
				testResponse.RequestedCapacity = validation.RequestedCapacity{
					Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					StoragePolicyID:  "id42",
					StorageClassName: "dummy-storage-class",
				}
				testResponse.Response = webhook.Allowed("")
			} else {
				testResponse.Response = webhook.Errored(tc.status, tc.err)
			}

			w := httptest.NewRecorder()

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			fakeHandler := &validation.RequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
					shouldErr: false,
				},
				Decoder: DummyDecoder{
					decoder:   admission.NewDecoder(builder.NewScheme()),
					shouldErr: false,
				},
			}

			fakeHandler.WriteResponse(w, testResponse)

			g := NewWithT(t)

			var actual *validation.RequestedCapacity
			respBody := w.Body.String()
			err := json.Unmarshal([]byte(respBody), &actual)

			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(actual.Capacity.String()).To(Equal(testResponse.Capacity.String()))
			g.Expect(actual.StoragePolicyID).To(Equal(testResponse.StoragePolicyID))
			g.Expect(actual.StorageClassName).To(Equal(testResponse.StorageClassName))

			if tc.allowed {
				g.Expect(w.Code).To(Equal(http.StatusOK))
				g.Expect(actual.Reason).To(BeEmpty())
			} else {
				g.Expect(w.Code).To(Equal(int(tc.status)))
				g.Expect(actual.Reason).To(Equal(tc.err.Error()))
			}
		})
	}
}

type DummyConverter struct {
	converter    runtime.UnstructuredConverter
	shouldErr    bool
	invocations  int
	errThreshold int
}

func (dc *DummyConverter) ToUnstructured(obj interface{}) (map[string]interface{}, error) {
	return nil, nil
}

func (dc *DummyConverter) FromUnstructured(u map[string]interface{}, obj interface{}) error {
	if dc.shouldErr {
		if dc.invocations == dc.errThreshold {
			return errors.New("fake error")
		}
		dc.invocations++
	}
	return dc.converter.FromUnstructured(u, obj)
}

type DummyDecoder struct {
	decoder   admission.Decoder
	shouldErr bool
}

func (dd DummyDecoder) Decode(req admission.Request, into runtime.Object) error {
	return nil
}

func (dd DummyDecoder) DecodeRaw(rawObj runtime.RawExtension, into runtime.Object) error {
	if dd.shouldErr {
		return fmt.Errorf("fake")
	}
	if rawObj.Raw == nil {
		return fmt.Errorf("fake")
	}
	err := dd.decoder.DecodeRaw(rawObj, into)

	return err
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func dummyVMWithStatusVolumes() *vmopv1.VirtualMachine {
	vm := builder.DummyVirtualMachine()
	vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
		{
			Type:  vmopv1.VirtualMachineStorageDiskTypeClassic,
			Limit: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
		},
	}

	return vm
}
