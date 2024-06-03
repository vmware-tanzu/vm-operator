// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
)

func init() {
	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klog.Background())
}

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Builder Test Suite")
}

type fakeManager struct {
	pkgmgr.Manager
	scheme *runtime.Scheme
}

func (f fakeManager) GetEventRecorderFor(name string) record.EventRecorder {
	return nil
}

func (f fakeManager) GetScheme() *runtime.Scheme {
	return f.scheme
}

func mustGetRawExtension(obj runtime.Object) runtime.RawExtension {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{
		Raw:    data,
		Object: obj,
	}
}

func getConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "world",
		},
	}
}

func getConfigMapGVK() metav1.GroupVersionKind {
	return metav1.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
}

func getConfigMapGVR() metav1.GroupVersionResource {
	return metav1.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
}

type metaRuntimeObj interface {
	metav1.Object
	runtime.Object
}

func getAdmissionRequest(
	obj metaRuntimeObj,
	op admissionv1.Operation) admission.Request {

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "123",
			Kind:      getConfigMapGVK(),
			Object:    mustGetRawExtension(obj),
			OldObject: mustGetRawExtension(obj),
			Operation: op,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Resource:  getConfigMapGVR(),
			UserInfo: authenticationv1.UserInfo{
				Username: "jdoe",
			},
		},
	}
}

func getAdmissionResponseOkay() admission.Response {
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code: 200,
			},
		},
	}
}
