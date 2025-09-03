// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/unifiedstoragequota/validation"
)

const (
	dummyVMName         = "dummy-vm"
	dummyVMSnapshotName = "dummy-vm-snapshot"
	dummyNamespaceName  = "dummy-vm-namespace-for-webhook-validation"
	badRequestMsg       = "bad request"
)

func unitTests() {
	Describe("VMWriteResponse", testVMRequestedCapacityHandlerWriteResponse)
	Describe("VMServeHTTP", testVMRequestedCapacityHandlerServeHTTP)
	Describe("VMHandle", testVMRequestedCapacityHandlerHandle)
	Describe("VMHandleCreate", testVMRequestedCapacityHandlerHandleCreate)
	Describe("VMHandleUpdate", testVMRequestedCapacityHandlerHandleUpdate)

	Describe("VMSnapshotWriteResponse", testVMSnapshotRequestedCapacityHandlerWriteResponse)
	Describe("VMSnapshotRequestedCapacityHandler ServeHTTP", testVMSnapshotRequestedCapacityHandlerServeHTTP)
	Describe("VMSnapshotHandle", testVMSnapshotRequestedCapacityHandlerHandle)
	Describe("VMSnapshotHandleCreate", testVMSnapshotRequestedCapacityHandlerHandleCreate)
}

func testVMRequestedCapacityHandlerWriteResponse() {
	Context("VMSnapshot feature gate disabled", func() {
		var (
			capacityResponse  *validation.CapacityResponse
			requestedCapacity *validation.RequestedCapacity

			w       *httptest.ResponseRecorder
			handler *validation.VMRequestedCapacityHandler
			errMsg  string
		)

		BeforeEach(func() {
			capacityResponse = &validation.CapacityResponse{}

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			w = httptest.NewRecorder()

			handler.WriteResponse(w, *capacityResponse)

			respBody := w.Body.String()

			if len(errMsg) > 0 {
				Expect(w.Body.String()).To(ContainSubstring(errMsg))
			} else {
				err := json.Unmarshal([]byte(respBody), &requestedCapacity)

				Expect(err).NotTo(HaveOccurred())
				Expect(requestedCapacity.Capacity.String()).To(Equal(capacityResponse.RequestedCapacities[0].Capacity.String()))
				Expect(requestedCapacity.StorageClassName).To(Equal(capacityResponse.RequestedCapacities[0].StorageClassName))
				Expect(requestedCapacity.StoragePolicyID).To(Equal(capacityResponse.RequestedCapacities[0].StoragePolicyID))
			}
		})

		AfterEach(func() {
			w = nil
			capacityResponse = nil
			requestedCapacity = nil
			errMsg = ""
		})

		When("request is not allowed", func() {
			BeforeEach(func() {
				errMsg = badRequestMsg
				capacityResponse.Response = webhook.Errored(http.StatusBadRequest, errors.New(errMsg))
			})

			It("should write the correct http response code and reason in response body", func() {
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request is allowed", func() {
			BeforeEach(func() {
				capacityResponse.RequestedCapacities = []*validation.RequestedCapacity{
					{
						Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						StoragePolicyID:  "id42",
						StorageClassName: "dummy-storage-class",
					},
				}
				capacityResponse.Response = webhook.Allowed("")
			})

			It("should write http status ok response code and empty reason in response body", func() {
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(requestedCapacity).To(Equal(&validation.RequestedCapacity{
					Capacity:         resource.MustParse("10Gi"),
					StoragePolicyID:  "id42",
					StorageClassName: "dummy-storage-class",
				}))
			})
		})
	})

	Context("VMSnapshot feature gate enabled", func() {
		var (
			capacityResponse *validation.CapacityResponse
			// The response should be a An array of RequestedCapacity type
			response []*validation.RequestedCapacity

			w       *httptest.ResponseRecorder
			handler *validation.VMRequestedCapacityHandler
			errMsg  string
		)

		BeforeEach(func() {

			capacityResponse = &validation.CapacityResponse{}

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()

			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)
			pkgcfg.UpdateContext(fakeWebhookContext.Context, func(config *pkgcfg.Config) {
				config.Features.VMSnapshots = true
			})

			handler = &validation.VMRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			w = httptest.NewRecorder()

			handler.WriteResponse(w, *capacityResponse)

			respBody := w.Body.String()

			if len(errMsg) > 0 {
				Expect(w.Body.String()).To(ContainSubstring(errMsg))
			} else {
				err := json.Unmarshal([]byte(respBody), &response)

				Expect(err).NotTo(HaveOccurred())

				Expect(response).To(HaveLen(1))
				Expect(response[0].Capacity.String()).To(Equal(capacityResponse.RequestedCapacities[0].Capacity.String()))
				Expect(response[0].StorageClassName).To(Equal(capacityResponse.RequestedCapacities[0].StorageClassName))
				Expect(response[0].StoragePolicyID).To(Equal(capacityResponse.RequestedCapacities[0].StoragePolicyID))
			}
		})

		AfterEach(func() {
			w = nil
			capacityResponse = nil
			response = nil
			errMsg = ""
		})

		When("request is not allowed", func() {
			BeforeEach(func() {
				errMsg = badRequestMsg
				capacityResponse.Response = webhook.Errored(http.StatusBadRequest, errors.New(errMsg))
			})

			It("should write the correct http response code and reason in response body", func() {
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request is allowed", func() {
			BeforeEach(func() {
				capacityResponse.RequestedCapacities = []*validation.RequestedCapacity{
					{
						Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						StoragePolicyID:  "id42",
						StorageClassName: "dummy-storage-class",
					},
				}
				capacityResponse.Response = webhook.Allowed("")
			})

			It("should write http status ok response code and empty reason in response body", func() {
				Expect(w.Code).To(Equal(http.StatusOK))
			})
		})
	})
}

func testVMRequestedCapacityHandlerServeHTTP() {
	Context("With an invalid request", func() {
		var (
			req  *http.Request
			resp *httptest.ResponseRecorder

			handler *validation.VMRequestedCapacityHandler
		)

		BeforeEach(func() {
			resp = httptest.NewRecorder()

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			handler.ServeHTTP(resp, req)
		})

		AfterEach(func() {
			req = nil
			resp = nil
		})

		When("request has an empty body", func() {
			BeforeEach(func() {
				req = &http.Request{Body: nil}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request has a NoBody body", func() {
			BeforeEach(func() {
				req = &http.Request{Body: http.NoBody}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request body cannot be decoded", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/json"}},
					Body:   nopCloser{Reader: bytes.NewBufferString("{")},
				}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request body is infinite", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/json"}},
					Method: http.MethodPost,
					Body:   nopCloser{Reader: rand.Reader},
				}
			})

			It("should write StatusRequestEntityTooLarge code", func() {
				Expect(resp.Code).To(Equal(http.StatusRequestEntityTooLarge))
			})
		})

		When("request body has wrong content-type", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/foo"}},
					Body:   nopCloser{Reader: bytes.NewBuffer(nil)},
				}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Context("With a valid request", func() {
		var (
			actual, expected *validation.RequestedCapacity

			handler *validation.VMRequestedCapacityHandler

			withObjects []ctrlclient.Object
			sc          *v1.StorageClass
			vmi         *vmopv1.VirtualMachineImage

			operation admissionv1.Operation
			ar        *admissionv1.AdmissionReview
		)

		BeforeEach(func() {
			sc = builder.DummyStorageClass()

			vm := builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = sc.Name

			oldVM := vm.DeepCopy()

			obj, _ := json.Marshal(vm)
			oldObj, _ := json.Marshal(oldVM)

			ar = &admissionv1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Request: &admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: obj,
					},
					OldObject: runtime.RawExtension{
						Raw: oldObj,
					},
				},
			}

			vmi = builder.DummyVirtualMachineImage(builder.DummyVMIName)
			vmi.Namespace = dummyNamespaceName
			vmi.Status = vmopv1.VirtualMachineImageStatus{
				Disks: []vmopv1.VirtualMachineImageDiskInfo{
					{
						Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					},
				},
			}
			withObjects = []ctrlclient.Object{sc, vmi}

			fakeClient := builder.NewFakeClient(withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMRequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
				},
				Decoder: DummyDecoder{
					decoder: admission.NewDecoder(builder.NewScheme()),
				},
			}
		})

		JustBeforeEach(func() {
			ar.Request.Operation = operation

			body, _ := json.Marshal(ar)

			req := httptest.NewRequest(http.MethodPost, "/getrequestedcapacityforvirtualmachine", bytes.NewReader(body))
			req.Header.Add("Content-Type", "application/json")

			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			respBody := resp.Body.String()
			err := json.Unmarshal([]byte(respBody), &actual)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(http.StatusOK))
		})

		When("admission request operation is create", func() {
			BeforeEach(func() {
				operation = admissionv1.Create
			})

			JustBeforeEach(func() {
				expected = &validation.RequestedCapacity{
					Capacity:         *vmi.Status.Disks[0].Capacity,
					StorageClassName: sc.Name,
					StoragePolicyID:  sc.Parameters["storagePolicyID"],
				}
			})

			It("should write StatusOK response code and correct response body", func() {
				Expect(actual.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(actual.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(actual.StorageClassName).To(Equal(expected.StorageClassName))
			})
		})

		When("admission request operation is update", func() {
			BeforeEach(func() {
				operation = admissionv1.Update
			})

			JustBeforeEach(func() {
				expected = &validation.RequestedCapacity{}
			})

			It("should write StatusOK response code and correct response body", func() {
				Expect(actual.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(actual.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(actual.StorageClassName).To(Equal(expected.StorageClassName))
			})
		})
	})
}

func testVMRequestedCapacityHandlerHandle() {
	var (
		vm, oldVM   *vmopv1.VirtualMachine
		obj, oldObj []byte

		handler *validation.VMRequestedCapacityHandler
		req     admission.Request
		resp    validation.CapacityResponse

		operation admissionv1.Operation
	)

	Context("Handle", func() {
		BeforeEach(func() {
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

			withObjects := []ctrlclient.Object{sc, vmi}

			fakeClient := builder.NewFakeClient(withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: operation,
					Object: runtime.RawExtension{
						Raw: obj,
					},
					OldObject: runtime.RawExtension{
						Raw: oldObj,
					},
				},
			}

			resp = handler.Handle(req)
		})

		AfterEach(func() {
			vm = nil
			oldVM = nil
		})

		When("there is an admission request with a create operation", func() {
			BeforeEach(func() {
				operation = admissionv1.Create
			})

			When("there is an error decoding the raw object", func() {

				It("should write StatusBadRequest code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))
				})
			})

			When("a valid request object is passed in", func() {
				BeforeEach(func() {
					vm = builder.DummyVirtualMachine()
					vm.Name = dummyVMName
					vm.Namespace = dummyNamespaceName
					vm.Spec.StorageClass = builder.DummyStorageClassName
					obj, _ = json.Marshal(vm)
				})

				It("should write StatusOK code to the response object", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))
				})
			})
		})

		When("there is an admission request with an update operation", func() {
			BeforeEach(func() {
				operation = admissionv1.Update
			})

			When("there is an error decoding the raw object", func() {

				It("should write StatusBadRequest code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))
				})
			})

			When("there is an error decoding the raw old object", func() {
				BeforeEach(func() {
					vm = builder.DummyVirtualMachine()
					vm.Name = dummyVMName
					vm.Namespace = dummyNamespaceName
					vm.Spec.StorageClass = builder.DummyStorageClassName

					obj, _ = json.Marshal(vm)
				})

				It("should write StatusBadRequest code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))
				})
			})

			When("a valid request object is passed in", func() {
				BeforeEach(func() {
					vm = builder.DummyVirtualMachine()
					vm.Name = dummyVMName
					vm.Namespace = dummyNamespaceName
					vm.Spec.StorageClass = builder.DummyStorageClassName
					obj, _ = json.Marshal(vm)

					oldVM = vm.DeepCopy()
					oldObj, _ = json.Marshal(oldVM)
				})

				It("should write StatusOK code to the response object", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))
				})
			})
		})
	})
}

func testVMRequestedCapacityHandlerHandleCreate() {
	var (
		interceptors interceptor.Funcs
		withObjects  []ctrlclient.Object

		dummyConverter *DummyConverter
		dummyDecoder   *DummyDecoder

		obj, oldObj *unstructured.Unstructured
		vm          *vmopv1.VirtualMachine

		resp, expected validation.CapacityResponse
	)

	When("HandleCreate is called", func() {
		BeforeEach(func() {
			expected = validation.CapacityResponse{}

			interceptors = interceptor.Funcs{}
			withObjects = []ctrlclient.Object{}

			dummyConverter = &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
			}
			dummyDecoder = &DummyDecoder{
				decoder: admission.NewDecoder(builder.NewScheme()),
			}

			vm = builder.DummyVirtualMachine()
		})

		JustBeforeEach(func() {
			fakeClient := builder.NewFakeClientWithInterceptors(interceptors, withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = builder.DummyStorageClassName

			obj, _ = builder.ToUnstructured(vm)
			fakeWebhookRequestContext := fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)

			fakeHandler := &validation.VMRequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter:      dummyConverter,
				Decoder:        *dummyDecoder,
			}
			resp = fakeHandler.HandleCreate(fakeWebhookRequestContext)
		})

		When("there is an error converting from unstructured", func() {
			BeforeEach(func() {
				dummyConverter.shouldErr = true
			})

			It("should write StatusBadRequest code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})

		When("storage class is not found", func() {
			It("should write StatusNotFound code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})

		Context("storage class is present", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, builder.DummyStorageClass())
			})

			When("there is an error getting storage class", func() {
				BeforeEach(func() {
					interceptors = interceptor.Funcs{
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
					}
				})

				It("should write StatusInternalServerError and an empty RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))

					Expect(resp.RequestedCapacities).To(BeNil())
				})
			})

			Context("vmi is specified as vm image kind", func() {
				var vmi *vmopv1.VirtualMachineImage

				BeforeEach(func() {
					vmi = builder.DummyVirtualMachineImage(builder.DummyVMIName)
					vmi.Namespace = dummyNamespaceName
					withObjects = append(withObjects, vmi)
				})

				AfterEach(func() {
					vmi = nil
				})

				When("VMI is not found", func() {
					BeforeEach(func() {
						vm.Spec.Image.Name = builder.DummyCVMIName
					})

					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("there is an error getting VMI for vm", func() {
					BeforeEach(func() {
						interceptors = interceptor.Funcs{
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
						}
					})

					It("should write StatusInternalServerError and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("image type is ISO", func() {
					BeforeEach(func() {
						vmi.Status.Type = "ISO"
					})

					It("should write StatusOK code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))
					})
				})

				When("disks section is empty in image status", func() {
					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("boot disk has an empty capacity", func() {
					BeforeEach(func() {
						vmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: nil,
							},
						}

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(0, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})

				When("when boot disk capacity is non-empty", func() {
					BeforeEach(func() {
						vmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
						}

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})

				When("there are multiple disks listed", func() {
					BeforeEach(func() {
						vmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
							{
								Capacity: nil,
							},
						}

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(3*10*1024*1024*1024, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should return the sum of all disks in the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})
			})

			Context("cvmi is specified as vm image kind", func() {
				var cvmi *vmopv1.ClusterVirtualMachineImage

				BeforeEach(func() {
					cvmi = builder.DummyClusterVirtualMachineImage(builder.DummyCVMIName)
					withObjects = append(withObjects, cvmi)

					vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "ClusterVirtualMachineImage",
						Name: builder.DummyCVMIName,
					}
				})

				AfterEach(func() {
					cvmi = nil
				})

				When("CVMI is not found", func() {
					BeforeEach(func() {
						vm.Spec.Image.Name = builder.DummyVMIName
					})

					It("should write StatusNotFound and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("there is an error getting CVMI for vm", func() {
					BeforeEach(func() {
						interceptors = interceptor.Funcs{
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
						}
					})

					It("should write StatusInternalServerError and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("disks section is empty in image status", func() {
					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.RequestedCapacities).To(BeNil())
					})
				})

				When("boot disk has an empty capacity", func() {
					BeforeEach(func() {
						cvmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: nil,
							},
						}
						withObjects = append([]ctrlclient.Object{withObjects[0]}, cvmi)

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(0, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})

				When("when boot disk capacity is non-empty", func() {
					BeforeEach(func() {
						cvmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
						}
						withObjects = append([]ctrlclient.Object{withObjects[0]}, cvmi)

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})

				When("there are multiple disks listed", func() {
					BeforeEach(func() {
						cvmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
							{
								Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							},
						}
						withObjects = append([]ctrlclient.Object{withObjects[0]}, cvmi)

						expected = validation.CapacityResponse{
							RequestedCapacities: []*validation.RequestedCapacity{
								{
									Capacity:         *resource.NewQuantity(3*10*1024*1024*1024, resource.BinarySI),
									StoragePolicyID:  "id42",
									StorageClassName: "dummy-storage-class",
								},
							},
						}
					})

					It("should return the sum of all disks in the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.RequestedCapacities).To(HaveLen(1))
						Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
						Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
						Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
					})
				})
			})

			When("vm uses invalid image kind", func() {
				BeforeEach(func() {
					vm = &vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Image: &vmopv1.VirtualMachineImageRef{
								Name: builder.DummyVMIName,
								Kind: "INVALID",
							},
						},
					}
				})

				It("should write StatusBadRequest and an empty RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

					Expect(resp.RequestedCapacities).To(BeNil())
				})
			})
		})
	})
}

func testVMRequestedCapacityHandlerHandleUpdate() {
	var (
		interceptors interceptor.Funcs
		withObjects  []ctrlclient.Object

		dummyConverter *DummyConverter
		dummyDecoder   *DummyDecoder

		obj, oldObj *unstructured.Unstructured
		vm, oldVM   *vmopv1.VirtualMachine

		resp, expected validation.CapacityResponse
	)

	When("HandleUpdate is called", func() {

		BeforeEach(func() {
			expected = validation.CapacityResponse{}

			interceptors = interceptor.Funcs{}
			withObjects = []ctrlclient.Object{builder.DummyStorageClass()}

			dummyConverter = &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
			}
			dummyDecoder = &DummyDecoder{
				decoder: admission.NewDecoder(builder.NewScheme()),
			}

			vm = builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = builder.DummyStorageClassName
			oldVM = vm.DeepCopy()
		})

		JustBeforeEach(func() {
			fakeClient := builder.NewFakeClientWithInterceptors(interceptors, withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			obj, _ = builder.ToUnstructured(vm)
			oldObj, _ = builder.ToUnstructured(oldVM)
			fakeWebhookRequestContext := fake.NewWebhookRequestContext(fakeWebhookContext, obj, oldObj)

			fakeHandler := &validation.VMRequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter:      dummyConverter,
				Decoder:        *dummyDecoder,
			}
			resp = fakeHandler.HandleUpdate(fakeWebhookRequestContext)
		})

		When("there is an error converting obj from unstructured", func() {
			BeforeEach(func() {
				dummyConverter.shouldErr = true
				dummyConverter.invocations = 0
				dummyConverter.errThreshold = 0
			})

			It("should write StatusBadRequest and an empty RequestedCapacity to the response", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})

		When("there is an error converting old obj from unstructured", func() {
			BeforeEach(func() {
				dummyConverter.shouldErr = true
				dummyConverter.invocations = 0
				dummyConverter.errThreshold = 1
			})

			It("should write StatusBadRequest and an empty RequestedCapacity to the response", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})

		When("updated boot disk size is nil", func() {
			BeforeEach(func() {
				vm.Spec.Advanced = nil
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}
			})

			It("should write StatusOK and an empty RequestedCapacity to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})

		When("old VM boot disk size is nil", func() {

			When("VM has no volumes", func() {
				BeforeEach(func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					}

					expected = validation.CapacityResponse{
						RequestedCapacities: []*validation.RequestedCapacity{
							{
								Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
								StoragePolicyID:  "id42",
								StorageClassName: "dummy-storage-class",
							},
						},
					}
				})

				It("should write StatusOK and the correct RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

					Expect(resp.RequestedCapacities).To(HaveLen(1))
					Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
					Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
					Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
				})
			})

			When("VM has volumes", func() {
				BeforeEach(func() {
					vm = dummyVMWithStatusVolumes()
					vm.Name = dummyVMName
					vm.Namespace = dummyNamespaceName
					vm.Spec.StorageClass = builder.DummyStorageClassName
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					}
					oldVM = vm.DeepCopy()
				})

				It("should write correct RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

					Expect(resp.RequestedCapacities).To(HaveLen(0))
				})
			})
		})

		When("there is an increase in boot disk size", func() {
			BeforeEach(func() {
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}

				expected = validation.CapacityResponse{
					RequestedCapacities: []*validation.RequestedCapacity{
						{
							Capacity:         *resource.NewQuantity(5*1024*1024*1024, resource.BinarySI),
							StoragePolicyID:  "id42",
							StorageClassName: "dummy-storage-class",
						},
					},
				}
			})

			It("should write the difference to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.RequestedCapacities).To(HaveLen(1))
				Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
				Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
				Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
			})
		})

		When("there is a decrease in boot disk size", func() {

			BeforeEach(func() {
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
			})

			It("should set an empty RequestedCapacity to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.RequestedCapacities).To(HaveLen(0))
			})
		})

		When("there is no change in boot disk size", func() {
			BeforeEach(func() {
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
			})

			It("should set an empty RequestedCapacity to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.RequestedCapacities).To(HaveLen(0))
			})
		})

		When("storage class is not found", func() {
			BeforeEach(func() {
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.StorageClass = "NOTFOUND"
			})

			It("should write StatusNotFound to the response", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

				Expect(resp.RequestedCapacities).To(HaveLen(0))
			})
		})

		When("there is an error getting storage class", func() {
			BeforeEach(func() {
				oldVM.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				}
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}
				interceptors = interceptor.Funcs{
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
				}
			})

			It("should write StatusInternalServerError to the response", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))

				Expect(resp.RequestedCapacities).To(BeNil())
			})
		})
	})
}

func testVMSnapshotRequestedCapacityHandlerWriteResponse() {
	var (
		capacityResponse    *validation.CapacityResponse
		requestedCapacities []*validation.RequestedCapacity

		w       *httptest.ResponseRecorder
		handler *validation.VMSnapshotRequestedCapacityHandler
		errMsg  string
	)

	BeforeEach(func() {
		capacityResponse = &validation.CapacityResponse{}

		fakeClient := builder.NewFakeClient()
		fakeManagerContext := fake.NewControllerManagerContext()
		fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

		handler = &validation.VMSnapshotRequestedCapacityHandler{
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
	})

	JustBeforeEach(func() {
		w = httptest.NewRecorder()

		handler.WriteResponse(w, *capacityResponse)

		respBody := w.Body.String()

		if len(errMsg) > 0 {
			Expect(w.Body.String()).To(ContainSubstring(errMsg))
		} else {
			Expect(json.Unmarshal([]byte(respBody), &requestedCapacities)).Should(Succeed())
		}
	})

	AfterEach(func() {
		w = nil
		capacityResponse = nil
		requestedCapacities = nil
		errMsg = ""
	})

	When("request is not allowed", func() {
		BeforeEach(func() {
			errMsg = badRequestMsg
			capacityResponse.Response = webhook.Errored(http.StatusBadRequest, errors.New(errMsg))
		})

		It("should write the correct http response code and reason in response body", func() {
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("request is allowed", func() {
		BeforeEach(func() {
			capacityResponse.RequestedCapacities = []*validation.RequestedCapacity{
				{
					Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					StoragePolicyID:  "id42",
					StorageClassName: "dummy-storage-class",
				},
			}
			capacityResponse.Response = webhook.Allowed("")
		})

		It("should write http status ok response code and empty reason in response body", func() {
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(requestedCapacities).To(Not(BeNil()))
			Expect(requestedCapacities).To(HaveLen(1))
			Expect((requestedCapacities)[0].Reason).To(BeEmpty())
		})
	})
}

func testVMSnapshotRequestedCapacityHandlerServeHTTP() {
	Context("With an invalid request", func() {
		var (
			req  *http.Request
			resp *httptest.ResponseRecorder

			handler *validation.VMSnapshotRequestedCapacityHandler
		)

		BeforeEach(func() {
			resp = httptest.NewRecorder()

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMSnapshotRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			handler.ServeHTTP(resp, req)
		})

		AfterEach(func() {
			req = nil
			resp = nil
		})

		When("request has an empty body", func() {
			BeforeEach(func() {
				req = &http.Request{Body: nil}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request has a NoBody body", func() {
			BeforeEach(func() {
				req = &http.Request{Body: http.NoBody}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request body cannot be decoded", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/json"}},
					Body:   nopCloser{Reader: bytes.NewBufferString("{")},
				}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})

		When("request body is infinite", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/json"}},
					Method: http.MethodPost,
					Body:   nopCloser{Reader: rand.Reader},
				}
			})

			It("should write StatusRequestEntityTooLarge code", func() {
				Expect(resp.Code).To(Equal(http.StatusRequestEntityTooLarge))
			})
		})

		When("request body has wrong content-type", func() {
			BeforeEach(func() {
				req = &http.Request{
					Header: http.Header{"Content-Type": []string{"application/foo"}},
					Body:   nopCloser{Reader: bytes.NewBuffer(nil)},
				}
			})

			It("should write StatusBadRequest code", func() {
				Expect(resp.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Context("With a valid request", func() {
		var (
			actual, expected []*validation.RequestedCapacity

			handler *validation.VMSnapshotRequestedCapacityHandler

			withObjects []ctrlclient.Object
			sc          *v1.StorageClass

			ar *admissionv1.AdmissionReview
		)

		BeforeEach(func() {
			sc = builder.DummyStorageClass()

			vm := builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = sc.Name
			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:      "vm-1-classic-1",
					Requested: ptr.To(resource.MustParse("1Gi")),
					Type:      vmopv1.VolumeTypeClassic,
				},
			}

			vmSnapshot := builder.DummyVirtualMachineSnapshot(dummyNamespaceName, dummyVMName, dummyVMName)
			obj, _ := json.Marshal(vmSnapshot)

			ar = &admissionv1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Request: &admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: obj,
					},
					Operation: admissionv1.Create,
				},
			}

			withObjects = []ctrlclient.Object{sc, vmSnapshot, vm}

			fakeClient := builder.NewFakeClient(withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMSnapshotRequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter: &DummyConverter{
					converter: runtime.DefaultUnstructuredConverter,
				},
				Decoder: DummyDecoder{
					decoder: admission.NewDecoder(builder.NewScheme()),
				},
			}
		})

		JustBeforeEach(func() {
			body, _ := json.Marshal(ar)

			req := httptest.NewRequest(http.MethodPost, "/getrequestedcapacityforvirtualmachinesnapshot", bytes.NewReader(body))
			req.Header.Add("Content-Type", "application/json")

			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			respBody := resp.Body.String()
			Expect(json.Unmarshal([]byte(respBody), &actual)).Should(Succeed())
			Expect(resp.Code).To(Equal(http.StatusOK), "respBody: %s", respBody)
		})

		When("admission request operation is create", func() {
			BeforeEach(func() {
				expected = []*validation.RequestedCapacity{
					{
						Capacity:         resource.MustParse("1Gi"),
						StorageClassName: sc.Name,
						StoragePolicyID:  sc.Parameters["storagePolicyID"],
					},
				}
			})

			It("should write StatusOK response code and correct response body", func() {
				Expect(actual[0].Capacity.String()).To(Equal(expected[0].Capacity.String()))
				Expect(actual[0].StoragePolicyID).To(Equal(expected[0].StoragePolicyID))
				Expect(actual[0].StorageClassName).To(Equal(expected[0].StorageClassName))
			})
		})
	})
}

func testVMSnapshotRequestedCapacityHandlerHandle() {
	var (
		vmSnapshot *vmopv1.VirtualMachineSnapshot
		obj        []byte

		handler *validation.VMSnapshotRequestedCapacityHandler
		req     admission.Request
		resp    validation.CapacityResponse
	)

	Context("Handle", func() {
		BeforeEach(func() {
			sc := builder.DummyStorageClass()
			vmSnapshot := builder.DummyVirtualMachineSnapshot(dummyNamespaceName, dummyVMSnapshotName, dummyVMName)
			vm := builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = sc.Name
			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:      "vm-1-classic-1",
					Requested: ptr.To(resource.MustParse("1Gi")),
					Type:      vmopv1.VolumeTypeClassic,
				},
			}

			withObjects := []ctrlclient.Object{sc, vmSnapshot, vm}

			fakeClient := builder.NewFakeClient(withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.VMSnapshotRequestedCapacityHandler{
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
		})

		JustBeforeEach(func() {
			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: obj,
					},
				},
			}

			resp = handler.Handle(req)
		})

		AfterEach(func() {
			vmSnapshot = nil
		})

		When("there is an admission request with a create operation", func() {
			When("there is an error decoding the raw object", func() {
				It("should write StatusBadRequest code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))
				})
			})

			When("a valid request object is passed in", func() {
				BeforeEach(func() {
					vmSnapshot = builder.DummyVirtualMachineSnapshot(dummyNamespaceName, dummyVMSnapshotName, dummyVMName)
					obj, _ = json.Marshal(vmSnapshot)
				})

				It("should write StatusOK code to the response object", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))
				})
			})
		})
	})
}

func testVMSnapshotRequestedCapacityHandlerHandleCreate() {
	var (
		interceptors interceptor.Funcs
		withObjects  []ctrlclient.Object

		dummyConverter *DummyConverter
		dummyDecoder   *DummyDecoder

		obj        *unstructured.Unstructured
		vmSnapshot *vmopv1.VirtualMachineSnapshot
		vm         *vmopv1.VirtualMachine
		sc         *v1.StorageClass

		resp, expected validation.CapacityResponse
	)

	When("HandleCreate is called", func() {
		BeforeEach(func() {
			expected = validation.CapacityResponse{}

			interceptors = interceptor.Funcs{}
			withObjects = []ctrlclient.Object{}

			dummyConverter = &DummyConverter{
				converter: runtime.DefaultUnstructuredConverter,
			}
			dummyDecoder = &DummyDecoder{
				decoder: admission.NewDecoder(builder.NewScheme()),
			}

			sc = builder.DummyStorageClass()
			vm = builder.DummyVirtualMachine()
			vm.Name = dummyVMName
			vm.Namespace = dummyNamespaceName
			vm.Spec.StorageClass = sc.Name
			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:      "vm-1-classic-1",
					Requested: ptr.To(resource.MustParse("5Gi")),
					Type:      vmopv1.VolumeTypeClassic,
				},
			}

			vmSnapshot = builder.DummyVirtualMachineSnapshot(dummyNamespaceName, dummyVMSnapshotName, dummyVMName)
		})

		JustBeforeEach(func() {
			fakeClient := builder.NewFakeClientWithInterceptors(interceptors, withObjects...)
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			obj, _ = builder.ToUnstructured(vmSnapshot)
			fakeWebhookRequestContext := fake.NewWebhookRequestContext(fakeWebhookContext, obj, nil)

			fakeHandler := &validation.VMSnapshotRequestedCapacityHandler{
				Client:         fakeClient,
				WebhookContext: fakeWebhookContext,
				Converter:      dummyConverter,
				Decoder:        *dummyDecoder,
			}
			resp = fakeHandler.HandleCreate(fakeWebhookRequestContext)
		})

		When("there is an error converting from unstructured", func() {
			BeforeEach(func() {
				dummyConverter.shouldErr = true
			})

			It("should write StatusBadRequest code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

				Expect(resp.RequestedCapacities).To(HaveLen(0))
				Expect(resp.Result.Message).To(ContainSubstring("fake error"))
			})
		})

		When("virtual machine snapshot's vm ref is not set", func() {
			BeforeEach(func() {
				vmSnapshot.Spec.VMRef = nil
				withObjects = []ctrlclient.Object{vmSnapshot}
			})

			It("should write StatusInternalServerError code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))
				Expect(resp.RequestedCapacities).To(HaveLen(0))
				Expect(resp.Result.Message).To(ContainSubstring("vmRef is not set"))
			})
		})

		When("virtual machine is not found", func() {
			BeforeEach(func() {
				withObjects = []ctrlclient.Object{vmSnapshot}
			})

			It("should write StatusInternalServerError code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))
				Expect(resp.RequestedCapacities).To(HaveLen(0))
				Expect(resp.Result.Message).To(ContainSubstring("failed to get VM"))
			})
		})

		When("Memory is enabled", func() {
			BeforeEach(func() {
				vmSnapshot.Spec.Memory = true
			})

			When("virtual machine class is not found", func() {
				BeforeEach(func() {
					vm.Spec.ClassName = "non-existent-vm-class"
					withObjects = []ctrlclient.Object{vmSnapshot, vm}
				})

				It("should write StatusInternalServerError code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))
					Expect(resp.RequestedCapacities).To(HaveLen(0))
					Expect(resp.Result.Message).To(ContainSubstring("failed to get VMClass"))
				})
			})
		})

		When("storage class is not found", func() {
			BeforeEach(func() {
				withObjects = []ctrlclient.Object{vmSnapshot, vm}
			})

			It("should write StatusNotFound code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

				Expect(resp.RequestedCapacities).To(HaveLen(0))
				Expect(resp.Result.Message).To(ContainSubstring("storageclasses.storage.k8s.io \"dummy-storage-class\" not found"))
			})
		})

		When("VM has managed disk", func() {
			BeforeEach(func() {
				vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
					Name:      "vm-1-managed-1",
					Requested: ptr.To(resource.MustParse("5Gi")),
					Type:      vmopv1.VolumeTypeManaged,
				},
				)
				vm.Spec.Volumes = append(vm.Spec.Volumes,
					vmopv1.VirtualMachineVolume{
						Name: "vm-1-managed-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
							},
						},
					})
				withObjects = []ctrlclient.Object{vmSnapshot, vm, sc}
			})

			When("pvc is not found", func() {
				It("should write StatusInternalServerError code to the response object", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusInternalServerError))
					Expect(resp.RequestedCapacities).To(HaveLen(0))
					Expect(resp.Result.Message).To(ContainSubstring("failed to get pvc"))
				})
			})

			When("pvc is found", func() {
				BeforeEach(func() {
					pvc := builder.DummyPersistentVolumeClaim()
					pvc.Name = "pvc-1"
					pvc.Namespace = dummyNamespaceName
					pvc.Spec.StorageClassName = &sc.Name
					withObjects = []ctrlclient.Object{vmSnapshot, vm, sc, pvc}

					expected = validation.CapacityResponse{
						RequestedCapacities: []*validation.RequestedCapacity{
							{
								Capacity:         resource.MustParse("10Gi"),
								StorageClassName: sc.Name,
								StoragePolicyID:  sc.Parameters["storagePolicyID"],
							},
						},
					}
				})

				It("should write StatusOK code to the response object", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))
					Expect(resp.RequestedCapacities).To(HaveLen(1))
					Expect(resp.RequestedCapacities[0].Capacity.String()).To(Equal(expected.RequestedCapacities[0].Capacity.String()))
					Expect(resp.RequestedCapacities[0].StoragePolicyID).To(Equal(expected.RequestedCapacities[0].StoragePolicyID))
					Expect(resp.RequestedCapacities[0].StorageClassName).To(Equal(expected.RequestedCapacities[0].StorageClassName))
				})
			})
		})
	})
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
			Type:  vmopv1.VolumeTypeClassic,
			Limit: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
		},
	}

	return vm
}
