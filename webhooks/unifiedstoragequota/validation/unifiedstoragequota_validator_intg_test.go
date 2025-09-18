// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/unifiedstoragequota/validation"
)

const (
	url           = "https://127.0.0.1:%d/getrequestedcapacityforvirtualmachine"
	vmSnapshotURL = "https://127.0.0.1:%d/getrequestedcapacityforvirtualmachinesnapshot"
	contentType   = "application/json"
)

func intgTests() {

	Describe(
		"CreateVM",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCreateVM,
	)
	Describe(
		"UpdateVM",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdate,
	)

	Describe(
		"CreateVMSnapshot",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCreateVMSnapshot,
	)
}

func intgTestsValidateCreateVM() {
	var (
		ctx *builder.IntegrationTestContext

		ar          *admissionv1.AdmissionReview
		sc          *v1.StorageClass
		vm, oldVM   *vmopv1.VirtualMachine
		obj, oldObj []byte

		r *validation.LegacyCapacityResponse

		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = builder.DummyVirtualMachine()
		vm.Name = dummyVMName
		vm.Namespace = ctx.Namespace

		sc = builder.DummyStorageClass()
		Expect(ctx.Client.Create(ctx, sc)).To(Succeed())

		vm.Spec.StorageClass = sc.Name

		ar = &admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				Kind:       "AdmissionReview",
				APIVersion: "admission.k8s.io/v1",
			},
			Request: &admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
			},
		}

	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, sc)).To(Succeed())

		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		var (
			err  error
			resp *http.Response
		)

		obj, _ = json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())

		oldObj, _ = json.Marshal(oldVM)
		Expect(err).NotTo(HaveOccurred())

		ar.Request.Object.Raw = obj
		ar.Request.OldObject.Raw = oldObj

		port := suite.GetManager().GetWebhookServer().(*webhook.DefaultServer).Options.Port
		body, _ := json.Marshal(ar)

		resp, err = httpClient.Post(fmt.Sprintf(url, port), contentType, bytes.NewBuffer(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		r = &validation.LegacyCapacityResponse{}
		Expect(json.NewDecoder(resp.Body).Decode(r)).To(Succeed())

		Expect(resp.Body.Close()).To(Succeed())
	})

	When("create is called", func() {
		var imageStatus vmopv1.VirtualMachineImageStatus

		BeforeEach(func() {
			imageStatus = vmopv1.VirtualMachineImageStatus{
				Disks: []vmopv1.VirtualMachineImageDiskInfo{
					{
						Capacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
					},
				},
			}
		})

		When("vm has skip annotation", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					pkgconst.SkipValidationAnnotationKey: "",
				}
			})

			JustBeforeEach(func() {
				if r.Capacity == resource.MustParse("0") {
					r.Capacity = resource.Quantity{}
				}
			})

			It("should return zero capacity", func() {
				Expect(r).ToNot(BeNil())
				Expect(*r).To(Equal(validation.LegacyCapacityResponse{Response: webhook.Allowed(pkgbuilder.SkipValidationAllowed)}))
			})
		})

		When("vm uses VirtualMachineImage", func() {
			var vmi *vmopv1.VirtualMachineImage

			BeforeEach(func() {
				vmi = builder.DummyVirtualMachineImage(builder.DummyVMIName)
				vmi.Namespace = ctx.Namespace

				Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())

				vmi.Status = imageStatus
				Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())
			})

			It("should return the correct capacity from the VMI", func() {
				Expect(r.Capacity.String()).To(Equal(vmi.Status.Disks[0].Capacity.String()))
			})
		})

		When("vm uses ClusterVirtualMachineImage", func() {
			var cvmi *vmopv1.ClusterVirtualMachineImage

			BeforeEach(func() {
				vm.Spec.Image.Kind = "ClusterVirtualMachineImage"
				vm.Spec.Image.Name = builder.DummyCVMIName

				cvmi = builder.DummyClusterVirtualMachineImage(builder.DummyCVMIName)
				Expect(ctx.Client.Create(ctx, cvmi)).To(Succeed())

				cvmi.Status = imageStatus
				Expect(ctx.Client.Status().Update(ctx, cvmi)).To(Succeed())
			})

			It("should return the correct capacity from the CVMI", func() {
				Expect(r.Capacity.String()).To(Equal(cvmi.Status.Disks[0].Capacity.String()))
			})
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *builder.IntegrationTestContext

		ar          *admissionv1.AdmissionReview
		sc          *v1.StorageClass
		vm, oldVM   *vmopv1.VirtualMachine
		obj, oldObj []byte

		r *validation.LegacyCapacityResponse

		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = builder.DummyVirtualMachine()
		vm.Name = dummyVMName
		vm.Namespace = ctx.Namespace

		sc = builder.DummyStorageClass()
		Expect(ctx.Client.Create(ctx, sc)).To(Succeed())

		vm.Spec.StorageClass = sc.Name

		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			BootDiskCapacity: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
		}

		oldVM = vm.DeepCopy()

		ar = &admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				Kind:       "AdmissionReview",
				APIVersion: "admission.k8s.io/v1",
			},
			Request: &admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
			},
		}

	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, sc)).To(Succeed())

		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		var (
			err  error
			resp *http.Response
		)

		obj, err = json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())

		oldObj, err = json.Marshal(oldVM)
		Expect(err).NotTo(HaveOccurred())

		ar.Request.Object.Raw = obj
		ar.Request.OldObject.Raw = oldObj

		port := suite.GetManager().GetWebhookServer().(*webhook.DefaultServer).Options.Port
		body, _ := json.Marshal(ar)

		resp, err = httpClient.Post(fmt.Sprintf(url, port), contentType, bytes.NewBuffer(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		r = &validation.LegacyCapacityResponse{}
		Expect(json.NewDecoder(resp.Body).Decode(r)).To(Succeed())

		Expect(resp.Body.Close()).To(Succeed())
	})

	When("update is called", func() {
		When("vm has skip annotation", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					pkgconst.SkipValidationAnnotationKey: "",
				}
			})

			JustBeforeEach(func() {
				if r.Capacity == resource.MustParse("0") {
					r.Capacity = resource.Quantity{}
				}
			})

			It("should return zero capacity", func() {
				Expect(r).ToNot(BeNil())
				Expect(*r).To(Equal(validation.LegacyCapacityResponse{Response: webhook.Allowed(pkgbuilder.SkipValidationAllowed)}))
			})
		})

		When("boot disk size has not changed", func() {

			It("should return an empty response", func() {
				Expect(r.Capacity.String()).To(Equal("0"))
			})
		})

		When("boot disk size has changed", func() {
			BeforeEach(func() {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					BootDiskCapacity: resource.NewQuantity(15*1024*1024*1024, resource.BinarySI),
				}
			})

			It("should return the correct capacity as the updated boot disk size", func() {
				expected := resource.NewQuantity(5*1024*1024*1024, resource.BinarySI)
				Expect(r.Capacity.String()).To(Equal(expected.String()))
			})
		})
	})
}

func intgTestsValidateCreateVMSnapshot() {
	var (
		ctx *builder.IntegrationTestContext

		ar         *admissionv1.AdmissionReview
		vm         *vmopv1.VirtualMachine
		vmSnapshot *vmopv1.VirtualMachineSnapshot
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = builder.DummyVirtualMachine()

		vm.Name = dummyVMName
		vm.Namespace = ctx.Namespace
		vmSnapshot = builder.DummyVirtualMachineSnapshot(ctx.Namespace, dummyVMSnapshotName, dummyVMName)
		vmSnapshot.Spec.VMRef = &vmopv1.VirtualMachinePartialRef{
			Name: vm.Name,
		}
		ar = &admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				Kind:       "AdmissionReview",
				APIVersion: "admission.k8s.io/v1",
			},
			Request: &admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
			},
		}

	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("create vmsnapshot is called", func() {
		When("VMSnapshot feature gate is disabled", func() {
			It("should return 404 when reaching the webhook, since getrequestedcapacityforvirtualmachinesnapshot is not registered", func() {
				obj, err := json.Marshal(vmSnapshot)
				Expect(err).NotTo(HaveOccurred())

				ar.Request.Object.Raw = obj
				port := suite.GetManager().GetWebhookServer().(*webhook.DefaultServer).Options.Port
				body, _ := json.Marshal(ar)

				httpClient := &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}
				resp, err := httpClient.Post(fmt.Sprintf(vmSnapshotURL, port), contentType, bytes.NewBuffer(body))
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Body.Close()).To(Succeed())
			})
		})
	})
}
