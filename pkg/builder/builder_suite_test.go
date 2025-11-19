// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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

const sampleCert = `
-----BEGIN CERTIFICATE-----
MIIDBTCCAe2gAwIBAgIUd+LVFzm10EplTR/Kz7js2oMOZiIwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNTA2MTEwODI2MzFaFw0zNTA2MDkw
ODI2MzFaMBIxEDAOBgNVBAMMB1Rlc3QgQ0EwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQDgNMBksqjxiYjIHCcb0A4068tIw3XKU+gsTp5qHXcLJy3qM2gK
qO7Y6aM7R07OO+yjZyLmHelYF3AmogD8Hc6wuWzOf6DQU1FxvdIWqtv8HYwgMevi
kZWRoyDavKwlCcvDhHCdrp8B+vzA8YO/hc/VuOwN1Pey/AdYd+bJ+PDdJhrQW7Hq
zApDyOA04R2K7ksgsNZUaUSfGFlJZAodYz1YSJR1z+UUTLxz0ICIQYFd10XetL4y
vf5LK+K93q462YUC2QkTVDOKd14YkkqXNgfCt9vGaY3pUdEEWpOojEz2uNviaVCo
0RmB7GUELd02G2kIlFoho5mma94IXk+eYB0nAgMBAAGjUzBRMB0GA1UdDgQWBBQE
OVztc4YU5n3FbbCFKqx6m8bq6zAfBgNVHSMEGDAWgBQEOVztc4YU5n3FbbCFKqx6
m8bq6zAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQA3kNzQYa1T
Scm/cmogV0wkPsgobQPW1Gzl1bt5dFvSbypa7torsT/vA1+X8eNLt/tFOqbpFriL
+tC+Ps6Z7LElIiuQM8DZDHsTeOumuXSqe15Qz+38fC6TVLAJzTBUULwYD8lspSdp
LJvFc9W/HWAM5SWDrQ/AyZzRIq9rvB0kc/lDPVCoCcmKZ4NFnf41RpRG0w42q0N5
0aP89sa7ZdbYY1awyh7FMFdO2XOcHw4TuWc9VoCk/5h/3FYlMucnPJ4lyp1y20l+
8zNwdFhcpsvY48pcrabtRWJS/KdD87ksraRXECfIgzW7zokKTzedLqR+RtV/5tx8
MZefw1eNNkHc
-----END CERTIFICATE-----
`

const sampleClientCert = `
-----BEGIN CERTIFICATE-----
MIIDBTCCAe2gAwIBAgIUMQs3WiYy/AvLxbRIjt8rnpAU/CswDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNTA2MTEwODI2MzJaFw0yNjA2MTEw
ODI2MzJaMCMxITAfBgNVBAMMGGFwaXNlcnZlci13ZWJob29rLWNsaWVudDCCASIw
DQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAK7DnvxTi1pDXyo5LN453tijkjwQ
hXt+TE3ZCX/LXVTjbU/FfhaHn0A+1rHjZQ5WjenVPtO8gLmtuf9DxmTwR2Zy/TrX
xQKMm29I+/9zFPtiQo9Y5l228iKlHJ0dWL//KV0NO/DbS7oWcTwndYlnYs6wuxs2
dXxhfATn0shEu4//80VjgpWncjFWgf0rDCUqe+jOXBmwGPZEQEJRioeTFeCrmPJO
C5wa2+bPrLj8iafftmlVSctnd9N+twcZcrUv6NLVMAu7WHpcPKrKPm+vvrUC9J6I
fusOHoO5sHrLLKcH4kCvOGEatWHMdR0SL0oQzBpr+uOaNCa6bhpNvYY7U2cCAwEA
AaNCMEAwHQYDVR0OBBYEFDAd2Beczo5jzLCpBeP8IpZykde6MB8GA1UdIwQYMBaA
FAQ5XO1zhhTmfcVtsIUqrHqbxurrMA0GCSqGSIb3DQEBCwUAA4IBAQDcKBelxvFs
ejSzzI82jlKk//csxMWoWE/76yZTbCLskkOGGAgx8GMDRqSAZ0QcmOTvxn8ryN8Q
O96zjson2aUi1kXblewgaPjUMN3oDS6CtwSSAI2z4gpNAtzaht0lkcctPYDc+iS2
SRQOJ5e2fYcMhrryTjPEofZJGAL5jOaLCgboxfH61NCxX0/zt0izLXmNGpdwVpgK
snx+g/B094rbZj2qGlwQewV7tt8XB7Li9deVG3RqO6D1PxK2XrpiZRZ+YHKvRKU0
ipRMO2DOCWX7KYOz+CQcHlKlODW1Iy5TMX/JFLFgcpQ0jRsNZjOeRcbhK/0bY99r
dlthlcOv+egU
-----END CERTIFICATE-----
`

const invalidClientCert = `
-----BEGIN CERTIFICATE-----
MIIC+TCCAeGgAwIBAgIUMQs3WiYy/AvLxbRIjt8rnpAU/CwwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNTA2MTEwODI2MzJaFw0yNjA2MTEw
ODI2MzJaMBcxFTATBgNVBAMMDG90aGVyLWNsaWVudDCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBAK7DnvxTi1pDXyo5LN453tijkjwQhXt+TE3ZCX/LXVTj
bU/FfhaHn0A+1rHjZQ5WjenVPtO8gLmtuf9DxmTwR2Zy/TrXxQKMm29I+/9zFPti
Qo9Y5l228iKlHJ0dWL//KV0NO/DbS7oWcTwndYlnYs6wuxs2dXxhfATn0shEu4//
80VjgpWncjFWgf0rDCUqe+jOXBmwGPZEQEJRioeTFeCrmPJOC5wa2+bPrLj8iaff
tmlVSctnd9N+twcZcrUv6NLVMAu7WHpcPKrKPm+vvrUC9J6IfusOHoO5sHrLLKcH
4kCvOGEatWHMdR0SL0oQzBpr+uOaNCa6bhpNvYY7U2cCAwEAAaNCMEAwHQYDVR0O
BBYEFDAd2Beczo5jzLCpBeP8IpZykde6MB8GA1UdIwQYMBaAFAQ5XO1zhhTmfcVt
sIUqrHqbxurrMA0GCSqGSIb3DQEBCwUAA4IBAQC9lTd1kctWqfnYdLq/yPJ10rh5
0C1E6fMAdCaaAd3Sl9g8Zj7FLYxZuCGdxIEiEsHJ/6TthD9NT85HTfoNzdo9VKzP
aKHMglcI4Z+ZqVL9MKhmQPHWjwnBVBJxt3w76lJra20gvrZYTEs33Oki0RTjqOuo
5Ykh7uwHtN+Qj1ZIHx19D7uaKTtHPMGwHyDQeHR2+zEPrXo58TFYDKpL/o46R08u
MKrYUYx1AdEqAt4U+m00cMMkbSL7gi28JXvjGh0tk+K28w8RzFIzca/9TH0sboWq
LFSiMxM649a2W2B9bOCkmeyfvFigYTTvKJGqsxLz3uL8LK2Sjz1lnRyQlwZu
-----END CERTIFICATE-----
`
