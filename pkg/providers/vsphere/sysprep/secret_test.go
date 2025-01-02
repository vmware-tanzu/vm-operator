// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package sysprep_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/sysprep"
)

var _ = Describe("GetSysprepSecretData", func() {
	var (
		err               error
		ctx               context.Context
		k8sClient         ctrlclient.Client
		initialObjects    []ctrlclient.Object
		secretNamespace   string
		sysprepSecretData sysprep.SecretData
		inlineSysprep     vmopv1sysprep.Sysprep
	)

	const anotherKey = "some_other_key"

	BeforeEach(func() {
		err = nil
		initialObjects = nil
		ctx = context.Background()
		secretNamespace = "default"
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().WithObjects(initialObjects...).Build()
		sysprepSecretData, err = sysprep.GetSysprepSecretData(
			ctx,
			k8sClient,
			secretNamespace,
			&inlineSysprep)
	})

	Context("for UserData", func() {
		productIDSecretName := "product_id_secret"
		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				UserData: vmopv1sysprep.UserData{
					ProductID: &vmopv1sysprep.ProductIDSecretKeySelector{
						Name: productIDSecretName,
						Key:  "product_id",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      productIDSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"product_id": []byte("foo_product_id"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(sysprepSecretData.ProductID).To(Equal("foo_product_id"))
			})

			When("key from selector is not present", func() {
				BeforeEach(func() {
					inlineSysprep.UserData.ProductID.Key = anotherKey
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(fmt.Sprintf(`no data found for key "%s" for secret default/%s`, anotherKey, productIDSecretName)))
				})
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf(`secrets "%s" not found`, productIDSecretName)))
			})
		})

		When("secret selector is absent", func() {

			BeforeEach(func() {
				inlineSysprep.UserData.FullName = "foo"
				inlineSysprep.UserData.ProductID = nil
			})

			It("does not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("for GUIUnattended", func() {
		pwdSecretName := "password_secret"

		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				GUIUnattended: &vmopv1sysprep.GUIUnattended{
					AutoLogon: true,
					Password: &vmopv1sysprep.PasswordSecretKeySelector{
						Name: pwdSecretName,
						Key:  "password",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pwdSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"password": []byte("foo_bar123"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(sysprepSecretData.Password).To(Equal("foo_bar123"))
			})

			When("key from selector is not present", func() {
				BeforeEach(func() {
					inlineSysprep.GUIUnattended.Password.Key = anotherKey
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(fmt.Sprintf(`no data found for key "%s" for secret default/%s`, anotherKey, pwdSecretName)))
				})
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf(`secrets "%s" not found`, pwdSecretName)))
			})
		})
	})

	Context("for Identification", func() {
		pwdSecretName := "domain_password_secret"

		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				Identification: &vmopv1sysprep.Identification{
					DomainAdminPassword: &vmopv1sysprep.DomainPasswordSecretKeySelector{
						Name: pwdSecretName,
						Key:  "domain_password",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pwdSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"domain_password": []byte("foo_bar_fizz123"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(sysprepSecretData.DomainPassword).To(Equal("foo_bar_fizz123"))
			})

			When("key from selector is not present", func() {
				BeforeEach(func() {
					inlineSysprep.Identification.DomainAdminPassword.Key = anotherKey
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(fmt.Sprintf(`no data found for key "%s" for secret default/%s`, anotherKey, pwdSecretName)))
				})
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf(`secrets "%s" not found`, pwdSecretName)))
			})
		})
	})
})

var _ = Describe("Sysprep GetSecretResources", func() {
	var (
		err             error
		ctx             context.Context
		k8sClient       ctrlclient.Client
		initialObjects  []ctrlclient.Object
		secretNamespace string
		secrets         []ctrlclient.Object
		inlineSysprep   vmopv1sysprep.Sysprep
	)

	BeforeEach(func() {
		err = nil
		initialObjects = nil
		ctx = context.Background()
		secretNamespace = "default"
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().WithObjects(initialObjects...).Build()
		secrets, err = sysprep.GetSecretResources(
			ctx,
			k8sClient,
			secretNamespace,
			&inlineSysprep)
	})

	Context("for UserData", func() {
		productIDSecretName := "product_id_secret"
		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				UserData: vmopv1sysprep.UserData{
					ProductID: &vmopv1sysprep.ProductIDSecretKeySelector{
						Name: productIDSecretName,
						Key:  "product_id",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      productIDSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"product_id": []byte("foo_product_id"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(secrets).To(HaveLen(1))
				Expect(secrets[0].GetName()).To(Equal(productIDSecretName))
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(secrets).To(BeNil())
			})
		})
	})

	Context("for GUIUnattended", func() {
		pwdSecretName := "password_secret"

		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				GUIUnattended: &vmopv1sysprep.GUIUnattended{
					AutoLogon: true,
					Password: &vmopv1sysprep.PasswordSecretKeySelector{
						Name: pwdSecretName,
						Key:  "password",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pwdSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"password": []byte("foo_bar123"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(secrets).To(HaveLen(1))
				Expect(secrets[0].GetName()).To(Equal(pwdSecretName))
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(secrets).To(BeNil())
			})
		})
	})

	Context("for Identification", func() {
		pwdSecretName := "domain_password_secret"

		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				Identification: &vmopv1sysprep.Identification{
					DomainAdminPassword: &vmopv1sysprep.DomainPasswordSecretKeySelector{
						Name: pwdSecretName,
						Key:  "domain_password",
					},
				},
			}
		})

		When("secret is present", func() {
			BeforeEach(func() {
				initialObjects = append(initialObjects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pwdSecretName,
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"domain_password": []byte("foo_bar_fizz123"),
					},
				})
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(secrets).To(HaveLen(1))
				Expect(secrets[0].GetName()).To(Equal(pwdSecretName))
			})
		})

		When("secret is not present", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(secrets).To(BeNil())
			})
		})
	})

	Context("when same secret name is used for all selectors", func() {
		secretName := "same-secret"
		BeforeEach(func() {
			inlineSysprep = vmopv1sysprep.Sysprep{
				UserData: vmopv1sysprep.UserData{
					ProductID: &vmopv1sysprep.ProductIDSecretKeySelector{
						Name: secretName,
						Key:  "product_id",
					},
				},
				GUIUnattended: &vmopv1sysprep.GUIUnattended{
					AutoLogon: true,
					Password: &vmopv1sysprep.PasswordSecretKeySelector{
						Name: secretName,
						Key:  "password",
					},
				},
				Identification: &vmopv1sysprep.Identification{
					DomainAdminPassword: &vmopv1sysprep.DomainPasswordSecretKeySelector{
						Name: secretName,
						Key:  "domain_password",
					},
				},
			}

			initialObjects = append(initialObjects, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNamespace,
				},
				Data: map[string][]byte{
					"product_id":      []byte("foo_product_id"),
					"password":        []byte("foo_bar123"),
					"domain_password": []byte("foo_bar_fizz123"),
				},
			})
		})

		It("returns a single secret", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveLen(1))
			Expect(secrets[0].GetName()).To(Equal(secretName))
			Expect(secrets[0].GetNamespace()).To(Equal(secretNamespace))
		})
	})
})
