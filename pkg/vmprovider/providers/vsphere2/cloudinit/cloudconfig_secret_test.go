// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudinit_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/cloudinit"
)

var _ = Describe("CloudConfig GetCloudConfigSecretData", func() {
	var (
		err                   error
		ctx                   context.Context
		k8sClient             ctrlclient.Client
		initialObjects        []ctrlclient.Object
		secretNamespace       string
		cloudConfig           vmopv1cloudinit.CloudConfig
		cloudConfigSecretData cloudinit.CloudConfigSecretData
	)

	BeforeEach(func() {
		err = nil
		initialObjects = nil
		ctx = context.Background()
		secretNamespace = "default"
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().WithObjects(initialObjects...).Build()
		cloudConfigSecretData, err = cloudinit.GetCloudConfigSecretData(
			ctx,
			k8sClient,
			secretNamespace,
			cloudConfig)
	})

	When("CloudConfig has a user that references data from a secret", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				Users: []vmopv1cloudinit.User{
					{
						Name: "bob.wilson",
						Passwd: &common.SecretKeySelector{
							Name: "my-bootstrap-data",
							Key:  "bob-wilsons-password",
						},
					},
				},
			}
		})
		When("The secret does not exist", func() {
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`secrets "my-bootstrap-data" not found`))
			})
		})
		When("The secret exists but the key is missing", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
					},
				}
			})
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`no data found for key "bob-wilsons-password" for secret default/my-bootstrap-data`))
			})
		})
		When("The secret exists and they key is present but has no data", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
						Data: map[string][]byte{
							"bob-wilsons-password": nil,
						},
					},
				}
			})
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`no data found for key "bob-wilsons-password" for secret default/my-bootstrap-data`))
			})
		})
		When("The secret exists and they key is present and has data", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
						Data: map[string][]byte{
							"bob-wilsons-password": []byte("password"),
						},
					},
				}
			})
			It("Should return valid data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(cloudConfigSecretData.Users).To(HaveLen(1))
				Expect(cloudConfigSecretData.Users).To(HaveKeyWithValue(
					"bob.wilson",
					cloudinit.CloudConfigUserSecretData{Passwd: "password"}))
			})
			When("The user also references a hashed password", func() {
				BeforeEach(func() {
					cloudConfig.Users[0].HashedPasswd = &common.SecretKeySelector{
						Name: "my-bootstrap-data",
						Key:  "bob-wilsons-hashed-password",
					}
				})
				It("Should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(`cloud config specified both hashed_passwd and passwd for user "bob.wilson"`))
				})
			})
		})
	})

	When("CloudConfig has a file that references data from a string", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				WriteFiles: []vmopv1cloudinit.WriteFile{
					{
						Path: "/hello",
					},
				},
			}
		})
		When("The string is a single line", func() {
			BeforeEach(func() {
				cloudConfig.WriteFiles[0].Content = []byte("world")
			})
			It("Should return valid data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(cloudConfigSecretData.WriteFiles).To(HaveLen(1))
				Expect(cloudConfigSecretData.WriteFiles).To(HaveKeyWithValue("/hello", "world"))
			})
		})
		When("The string is a multi-line value", func() {
			BeforeEach(func() {
				cloudConfig.WriteFiles[0].Content = []byte("|\n  world\n  and the universe!")
			})
			It("Should return valid data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(cloudConfigSecretData.WriteFiles).To(HaveLen(1))
				Expect(cloudConfigSecretData.WriteFiles).To(HaveKeyWithValue("/hello", "world\nand the universe!"))
			})
		})
	})

	When("CloudConfig has a file that references data from a secret", func() {
		BeforeEach(func() {
			cloudConfig.WriteFiles[0].Content = []byte("name: \"my-bootstrap-data\"\nkey: \"file-hello\"")
		})
		When("The secret does not exist", func() {
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`secrets "my-bootstrap-data" not found`))
			})
		})
		When("The secret exists but the key is missing", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
					},
				}
			})
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`no data found for key "file-hello" for secret default/my-bootstrap-data`))
			})
		})
		When("The secret exists and they key is present but has no data", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
						Data: map[string][]byte{
							"file-hello": nil,
						},
					},
				}
			})
			It("Should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`no data found for key "file-hello" for secret default/my-bootstrap-data`))
			})
		})
		When("The secret exists and they key is present and has data", func() {
			BeforeEach(func() {
				initialObjects = []ctrlclient.Object{
					&corev1.Secret{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "v1",
							Kind:       "Secret",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      "my-bootstrap-data",
						},
						Data: map[string][]byte{
							"file-hello": []byte("world"),
						},
					},
				}
			})
			It("Should return valid data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(cloudConfigSecretData.WriteFiles).To(HaveLen(1))
				Expect(cloudConfigSecretData.WriteFiles).To(HaveKeyWithValue("/hello", "world"))
			})
		})
	})

	When("CloudConfig has a user and file that reference data from different secrets", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				Users: []vmopv1cloudinit.User{
					{
						Name: "bob.wilson",
						Passwd: &common.SecretKeySelector{
							Name: "my-bootstrap-credentials",
							Key:  "bob-wilsons-password",
						},
					},
				},
				WriteFiles: []vmopv1cloudinit.WriteFile{
					{
						Path:    "/hello",
						Content: []byte("name: \"my-bootstrap-files\"\nkey: \"file-hello\""),
					},
				},
			}

			initialObjects = []ctrlclient.Object{
				&corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: secretNamespace,
						Name:      "my-bootstrap-credentials",
					},
					Data: map[string][]byte{
						"bob-wilsons-password": []byte("password"),
					},
				},
				&corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: secretNamespace,
						Name:      "my-bootstrap-files",
					},
					Data: map[string][]byte{
						"file-hello": []byte("world"),
					},
				},
			}
		})
		It("Should return valid data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(cloudConfigSecretData.WriteFiles).To(HaveLen(1))
			Expect(cloudConfigSecretData.WriteFiles).To(HaveKeyWithValue("/hello", "world"))
			Expect(cloudConfigSecretData.Users).To(HaveLen(1))
			Expect(cloudConfigSecretData.Users).To(HaveKeyWithValue(
				"bob.wilson",
				cloudinit.CloudConfigUserSecretData{Passwd: "password"}))
		})
	})
})
