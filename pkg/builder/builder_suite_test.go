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
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	_ "github.com/vmware-tanzu/vm-operator/test/builder/log"
)

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Builder Test Suite")
}

type fakeManager struct {
	pkgmgr.Manager
	scheme *runtime.Scheme
}

func (f fakeManager) GetEventRecorder(name string) events.EventRecorder {
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
MIIDBTCCAe2gAwIBAgIUWTEJC229+nP75zImvnpY/qK/4e0wDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNjA2MTExOTI5MDhaFw0zNjA2MDgx
OTI5MDhaMBIxEDAOBgNVBAMMB1Rlc3QgQ0EwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQCiZOk35bRfoe5BCWuB3mafUGl0gNykjknNSfxcThAbb90jCyd7
wBRF3gr2Jdv+43C6GEXKvTpVICdA/egaNlKSaGOTLFxR6oknrEEPsp7tKaC5EMIX
fBZvQUXnv7ySF4ZVa4BQszYhNg9Lt+Qrf/wfyp3WpdzplRuLeq1KwOehOIbNXiUj
piOUisTcrNfuzVVuaI79z5/AmwSmnnUF0xwjqtsR9zjjSd8K1hJm8vAaRBGLW3vF
XRi+UILAvJtbGN6DR/55bxKF6/E6QAPJ5/969eqFDZckhTfQvgIDddDr7ImyfJy6
cxYAR82mIA4l9D1KhTSqUzip9y5Jzwe8EitdAgMBAAGjUzBRMB0GA1UdDgQWBBQv
Fo5BSlp1rHDNj1TNWxROyz4/0zAfBgNVHSMEGDAWgBQvFo5BSlp1rHDNj1TNWxRO
yz4/0zAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQB3FGhgESZv
pzS9+JuNL7rdXogGO53BfOCf1S5San5ijS183tdmeQGd/PkOGgNsVxsAOg7gGK7F
TojuR3reLJ2OGa9dETlqrRtCdMl3D7WHyAj0wdVNdBzoGU04G7utBdRq5GUjApJQ
q2a/WnORN54nu3NblnEHbUDroo4IRJzVCqzvcIUvUJey41Sf3euvhsVL1Adf3pAP
jJ3yi4Bz14tjqfdsayHCOhWqleXL4SXVc0p5ApJ8vgdrkn9zIbC2OMRiRxH9hhAA
yMyfkLf2gkTl2G1Tg/efkWKQzcewjPnjsJujsO3czJ9fmkafZUjtqS6sE4uCMvYM
c4i70LvJK3+Q
-----END CERTIFICATE-----
`

const sampleClientCert = `
-----BEGIN CERTIFICATE-----
MIIDGjCCAgKgAwIBAgIUfocVdzOifUFs9yGD5n9y7f/hqG8wDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNjA2MTExOTI5MTRaFw0zNjA2MDgx
OTI5MTRaMCMxITAfBgNVBAMMGGFwaXNlcnZlci13ZWJob29rLWNsaWVudDCCASIw
DQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBANZHPUl5eWVEmWGBpBjWkhx2iBRr
jtmGmhFDwzLue+16Yk80ieqI1Q1OZJOW2DUbpSxklNEmDVsjlXbUHhYsz5fs57cN
ma/x1UQ0gR4bDUpY3ywWk3UqSOlzoi4KRw4ZHQEzYB3p6y9JLKqkAc4QNbRoAiVK
JCzpbADM/r1zRn5wIqQrJuKPvwN1W5XwrHF7ZSFD1O1AlZtEkuU8YjCzPbgb2LTT
bZQ2/vWIpBJpm98JffGSzm0jgsroB49d7XVqEFcASs3PIkqHv8kXh2ySm6s5MP0q
0hNU7TUEv3RDz8PdmlAIXjkkoOkABgcIn8CKOy+gsxh5lw4Cgq64V1LXyJ0CAwEA
AaNXMFUwEwYDVR0lBAwwCgYIKwYBBQUHAwIwHQYDVR0OBBYEFEsVwKG+H3b6CChj
STS1wZUuFANiMB8GA1UdIwQYMBaAFC8WjkFKWnWscM2PVM1bFE7LPj/TMA0GCSqG
SIb3DQEBCwUAA4IBAQACMGaXF9NH2GX9OmHcqLT50tqZK+Ol6uhKffTllW9GSs1O
dkrbkCF1H0DUUmfXvLPRSdaGpSBJ+sRIB2367qfb3NaNsEx4PyMBt7KNBdI3/YOl
mifv/Cwdeu6hl7PRD2/ODveD611rHrxoLlesUOR1nRKCm8jxxHLBGM4sJbyUp6V5
FInRL3pNqO8S3UwzcFa5Q/8SF4oQ+TH8sbINGkB6Y/JvBKQ54+sniUTCPZWBUE8U
qi24dE3q6XR+hhR62C9S8iTxkc4MP5MeO9wkrKZW7XiIPYAZidmEyD8DcKFEyBMZ
Kz9xCBwWij5ouYNXTYpnUEartwlQCaTPC4XBWqyW
-----END CERTIFICATE-----
`

const invalidClientCert = `
-----BEGIN CERTIFICATE-----
MIIDDjCCAfagAwIBAgIUfocVdzOifUFs9yGD5n9y7f/hqHAwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNjA2MTExOTI5MjFaFw0zNjA2MDgx
OTI5MjFaMBcxFTATBgNVBAMMDG90aGVyLWNsaWVudDCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBAL+hPWhLPlnLRfMaZEU3W5xybefJ1xQ8eh3EQeXECWJ2
FI4cfwjutlP3SSbNo7W65kwUyrDq2a+/t8FxWiPxJQtcagRNsN5tQ8NNoMHUYiuG
LtmEK8a62tnnWXTuRmPEpfDdwCtc51+4DUzksxv0xy8SXP8vf2eoYziG+cmQLqRu
Z8wnp127dylcVnXRAV3SCCiHci+2/EF0YP2UUxOwqTgiqX1Vma7JlClqJm2HyszK
QAmiopRqyLDVK8/ooo5bRt3LEwxbB/mhJXLAhMtpF2LaWc3kl0Al+W3HBvhQkuT1
MGHJ5l9rKEY7Vswecs2TTA/fWl9xdUGRjf+FF2DM1t0CAwEAAaNXMFUwEwYDVR0l
BAwwCgYIKwYBBQUHAwIwHQYDVR0OBBYEFPTsnsQyDNi6bpvIPZA99Wi2Azk5MB8G
A1UdIwQYMBaAFC8WjkFKWnWscM2PVM1bFE7LPj/TMA0GCSqGSIb3DQEBCwUAA4IB
AQB6NfWdb9ueakR9WE+v0E+igfl9mjfZvruVANcqf/zgmNbO9jj+9XIz5L/xIdfa
3UHE7rBL3p5i5r0cVINFe6mP/6sTLEmIZFjlXPPj3RliPzxncBhm2fQW4IPgGKAz
Hv75TFiNOoWwjIswH3N+lcMMWXdyxkQCMuVvy9xpprZrmzqWvtNQBSsQb7Xf0sR+
xaB0/9TIKMXKGQC1ytoFCw+nqtFXLWbv0bQ4v6T/8l6AnodHrzrlEleiWgJmqb7W
jhzbuxtFmTgZUgffE3/QBDpEnORriVlcY5+5nj1ryDIMjAO+Orq3jrAMsH1oFYvA
b/I9LTjiYhVTeIS4wYVLiHzZ
-----END CERTIFICATE-----
`
