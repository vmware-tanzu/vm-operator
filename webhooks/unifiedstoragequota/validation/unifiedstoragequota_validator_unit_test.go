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

func unitTests() {
	Describe("WriteResponse", testRequestedCapacityHandlerWriteResponse)
	Describe("ServeHTTP", testRequestedCapacityHandlerServeHTTP)
	Describe("Handle", testRequestedCapacityHandlerHandle)
	Describe("HandleCreate", testRequestedCapacityHandlerHandleCreate)
	Describe("HandleUpdate", testRequestedCapacityHandlerHandleUpdate)
}

func testRequestedCapacityHandlerWriteResponse() {

	var (
		capacityResponse  *validation.CapacityResponse
		requestedCapacity *validation.RequestedCapacity

		w       *httptest.ResponseRecorder
		handler *validation.RequestedCapacityHandler
	)

	BeforeEach(func() {
		capacityResponse = &validation.CapacityResponse{}

		fakeClient := builder.NewFakeClient()
		fakeManagerContext := fake.NewControllerManagerContext()
		fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

		handler = &validation.RequestedCapacityHandler{
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
		err := json.Unmarshal([]byte(respBody), &requestedCapacity)

		Expect(err).NotTo(HaveOccurred())

		Expect(requestedCapacity.Capacity.String()).To(Equal(capacityResponse.Capacity.String()))
		Expect(requestedCapacity.StorageClassName).To(Equal(capacityResponse.StorageClassName))
		Expect(requestedCapacity.StoragePolicyID).To(Equal(capacityResponse.StoragePolicyID))
	})

	AfterEach(func() {
		w = nil
		capacityResponse = nil
		requestedCapacity = nil
	})

	When("request is not allowed", func() {
		BeforeEach(func() {
			capacityResponse.Response = webhook.Errored(http.StatusBadRequest, errors.New("bad request"))
		})

		It("should write the correct http response code and reason in response body", func() {
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(requestedCapacity.Reason).To(Equal(capacityResponse.Response.Result.Message))
		})
	})

	When("request is allowed", func() {
		BeforeEach(func() {
			capacityResponse.RequestedCapacity = validation.RequestedCapacity{
				Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				StoragePolicyID:  "id42",
				StorageClassName: "dummy-storage-class",
			}
			capacityResponse.Response = webhook.Allowed("")
		})

		It("should write http status ok response code and empty reason in response body", func() {
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(requestedCapacity.Reason).To(BeEmpty())
		})
	})
}

func testRequestedCapacityHandlerServeHTTP() {
	Context("With an invalid request", func() {
		var (
			req  *http.Request
			resp *httptest.ResponseRecorder

			handler *validation.RequestedCapacityHandler
		)

		BeforeEach(func() {
			resp = httptest.NewRecorder()

			fakeClient := builder.NewFakeClient()
			fakeManagerContext := fake.NewControllerManagerContext()
			fakeWebhookContext := fake.NewWebhookContext(fakeManagerContext)

			handler = &validation.RequestedCapacityHandler{
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

			handler *validation.RequestedCapacityHandler

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

			handler = &validation.RequestedCapacityHandler{
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

func testRequestedCapacityHandlerHandle() {
	var (
		vm, oldVM   *vmopv1.VirtualMachine
		obj, oldObj []byte

		handler *validation.RequestedCapacityHandler
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

			handler = &validation.RequestedCapacityHandler{
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

func testRequestedCapacityHandlerHandleCreate() {
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

			fakeHandler := &validation.RequestedCapacityHandler{
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
			})
		})

		When("storage class is not found", func() {
			It("should write StatusNotFound code to the response object", func() {
				Expect(resp.Allowed).To(BeFalse())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

					Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
					Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
					Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
				})
			})

			When("vm image is nil", func() {
				BeforeEach(func() {
					vm = &vmopv1.VirtualMachine{}
				})

				It("should write StatusBadRequest code and an empty RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeFalse())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusBadRequest))

					Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
					Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
					Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
					})
				})

				When("boot disk has an empty capacity", func() {
					BeforeEach(func() {
						vmi.Status.Disks = []vmopv1.VirtualMachineImageDiskInfo{
							{
								Capacity: nil,
							},
						}
					})

					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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
							RequestedCapacity: validation.RequestedCapacity{
								Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
								StoragePolicyID:  "id42",
								StorageClassName: "dummy-storage-class",
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
					})
				})

				When("disks section is empty in image status", func() {
					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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
					})

					It("should write StatusNotFound code and an empty RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeFalse())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusNotFound))

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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
							RequestedCapacity: validation.RequestedCapacity{
								Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
								StoragePolicyID:  "id42",
								StorageClassName: "dummy-storage-class",
							},
						}
					})

					It("should write StatusOK code and the correct RequestedCapacity to the response", func() {
						Expect(resp.Allowed).To(BeTrue())
						Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

						Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
						Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
						Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

					Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
					Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
					Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
				})
			})
		})
	})
}

func testRequestedCapacityHandlerHandleUpdate() {
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

			fakeHandler := &validation.RequestedCapacityHandler{
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
			})
		})

		When("old VM boot disk size is nil", func() {

			When("VM has no volumes", func() {
				BeforeEach(func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					}

					expected = validation.CapacityResponse{
						RequestedCapacity: validation.RequestedCapacity{
							Capacity:         *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
							StoragePolicyID:  "id42",
							StorageClassName: "dummy-storage-class",
						},
					}
				})

				It("should write StatusOK and the correct RequestedCapacity to the response", func() {
					Expect(resp.Allowed).To(BeTrue())
					Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

					Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
					Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
					Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

					Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
					Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
					Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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
					RequestedCapacity: validation.RequestedCapacity{
						Capacity:         *resource.NewQuantity(5*1024*1024*1024, resource.BinarySI),
						StoragePolicyID:  "id42",
						StorageClassName: "dummy-storage-class",
					},
				}
			})

			It("should write the difference to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

			It("should write zero capacity to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

			It("should write zero capacity to the response", func() {
				Expect(resp.Allowed).To(BeTrue())
				Expect(int(resp.Result.Code)).To(Equal(http.StatusOK))

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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

				Expect(resp.Capacity.String()).To(Equal(expected.Capacity.String()))
				Expect(resp.StoragePolicyID).To(Equal(expected.StoragePolicyID))
				Expect(resp.StorageClassName).To(Equal(expected.StorageClassName))
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
			Type:  vmopv1.VirtualMachineStorageDiskTypeClassic,
			Limit: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
		},
	}

	return vm
}
