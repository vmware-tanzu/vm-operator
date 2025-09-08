// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package linuxprep_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/linuxprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("GetLinuxPrepSecretData", func() {
	const namespace = "linux-prep-secret-ns"

	var (
		ctx                 context.Context
		linuxPrepSpec       *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
		k8sClient           ctrlclient.Client
		initObjects         []ctrlclient.Object
		linuxPrepSecretData linuxprep.SecretData
		err                 error
	)

	BeforeEach(func() {
		linuxPrepSpec = &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClient(initObjects...)
		linuxPrepSecretData, err = linuxprep.GetLinuxPrepSecretData(ctx, k8sClient, namespace, linuxPrepSpec)
	})

	AfterEach(func() {
		initObjects = nil
	})

	Context("Password", func() {
		const key = "passwd"
		var secret *corev1.Secret

		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "password",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					key: []byte("my-password"),
				},
			}
			linuxPrepSpec.Password = &vmopv1common.PasswordSecretKeySelector{
				Name: secret.Name,
				Key:  key,
			}
		})

		Context("Secret does not exist", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		Context("Secret exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, secret)
			})

			Context("key does not exist", func() {
				BeforeEach(func() {
					linuxPrepSpec.Password.Key = "bogus"
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("does not contain required password key"))
				})
			})

			It("returns success", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(linuxPrepSecretData.Password).To(Equal("my-password"))
			})
		})
	})

	Context("ScriptText", func() {
		const key = "text"
		var secret *corev1.Secret

		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "script-text",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					key: []byte("my-secret-script"),
				},
			}
			linuxPrepSpec.ScriptText = &vmopv1common.ValueOrSecretKeySelector{}
		})

		Context("Inline script value", func() {
			BeforeEach(func() {
				linuxPrepSpec.ScriptText.Value = ptr.To("my-inlined-script")
			})

			It("returns success", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(linuxPrepSecretData.ScriptText).To(Equal("my-inlined-script"))
			})
		})

		Context("From Secret value", func() {
			BeforeEach(func() {
				linuxPrepSpec.ScriptText.From = &vmopv1common.SecretKeySelector{
					Name: secret.Name,
					Key:  key,
				}
			})

			Context("Secret does not exist", func() {
				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, secret)
				})

				Context("key does not exist", func() {
					BeforeEach(func() {
						linuxPrepSpec.ScriptText.From.Key = "bogus"
					})

					It("returns an error", func() {
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("does not contain required script text key"))
					})
				})

				It("returns success", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(linuxPrepSecretData.ScriptText).To(Equal("my-secret-script"))
				})
			})
		})
	})
})
