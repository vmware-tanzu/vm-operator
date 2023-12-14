// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudinit_test

import (
	"context"
	"encoding/json"

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

var _ = Describe("CloudConfig MarshalYAML", func() {
	var (
		err                   error
		data                  string
		ctx                   context.Context
		cloudConfig           vmopv1cloudinit.CloudConfig
		cloudConfigSecretData cloudinit.CloudConfigSecretData
	)

	BeforeEach(func() {
		err = nil
		data = ""
		ctx = context.Background()
		cloudConfig = vmopv1cloudinit.CloudConfig{}
		cloudConfigSecretData = cloudinit.CloudConfigSecretData{}
	})

	JustBeforeEach(func() {
		data, err = cloudinit.MarshalYAML(ctx, cloudConfig, cloudConfigSecretData)
	})

	When("CloudConfig and CloudConfigSecretData are both empty", func() {
		It("Should return nil data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(BeEmpty())
		})
	})

	When("CloudConfig is not empty but CloudConfigSecretData is", func() {
		BeforeEach(func() {
			cloudConfig.Users = []vmopv1cloudinit.User{
				{
					Name: "bob.wilson",
				},
			}
		})
		It("Should return user data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(Equal("users:\n  - name: bob.wilson\n"))
		})
	})

	When("CloudConfig is empty but CloudConfigSecretData is not", func() {
		BeforeEach(func() {
			cloudConfigSecretData.Users = map[string]cloudinit.CloudConfigUserSecretData{
				"bob.wilson": {
					Passwd: "password",
				},
			}
		})
		It("Should return nil data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(BeEmpty())
		})
	})

	When("CloudConfig and CloudConfigSecretData both have data", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				Users: []vmopv1cloudinit.User{
					{
						Name: "bob.wilson",
						HashedPasswd: &common.SecretKeySelector{
							Name: "my-bootstrap-data",
							Key:  "cloud-init-user-bob.wilson-hashed_passwd",
						},
					},
				},
				WriteFiles: []vmopv1cloudinit.WriteFile{
					{
						Path:    "/hello",
						Content: []byte("world"),
					},
					{
						Path:    "/hi",
						Content: []byte("name: \"my-bootstrap-data\"\nkey: \"cloud-init-files-hi\""),
					},
				},
			}
			cloudConfigSecretData = cloudinit.CloudConfigSecretData{
				Users: map[string]cloudinit.CloudConfigUserSecretData{
					"bob.wilson": {
						HashPasswd: "0123456789",
					},
				},
				WriteFiles: map[string]string{
					"/hi":    "there",
					"/hello": "world",
				},
			}
		})
		When("There is no default user", func() {
			It("Should return user data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal(cloudConfigWithNoDefaultUser))
			})
		})
		When("There is a default user", func() {
			BeforeEach(func() {
				cloudConfig.AlwaysDefaultUser = true
			})
			It("Should return user data", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal(cloudConfigWithDefaultUser))
			})
		})
	})

	When("CloudConfig has RunCmds", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				Users: []vmopv1cloudinit.User{
					{
						Name: "bob.wilson",
						HashedPasswd: &common.SecretKeySelector{
							Name: "my-bootstrap-data",
							Key:  "cloud-init-user-bob.wilson-hashed_passwd",
						},
					},
				},
				RunCmd: []json.RawMessage{
					[]byte("ls /"),
					[]byte(`[ "ls", "-a", "-l", "/" ]`),
					[]byte("- echo\n- \"hello, world.\""),
				},
			}
			cloudConfigSecretData = cloudinit.CloudConfigSecretData{
				Users: map[string]cloudinit.CloudConfigUserSecretData{
					"bob.wilson": {
						HashPasswd: "0123456789",
					},
				},
			}
		})
		It("Should return user data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(Equal(cloudConfigWithWithRunCmds))
		})
	})

})

var _ = Describe("CloudConfig GetSecretResources", func() {
	var (
		err             error
		ctx             context.Context
		k8sClient       ctrlclient.Client
		initialObjects  []ctrlclient.Object
		secretNamespace string
		cloudConfig     vmopv1cloudinit.CloudConfig
		secretResources []ctrlclient.Object
	)

	BeforeEach(func() {
		err = nil
		initialObjects = nil
		ctx = context.Background()
		secretNamespace = "default"
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().WithObjects(initialObjects...).Build()
		secretResources, err = cloudinit.GetSecretResources(
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
		When("The secret exists", func() {
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
				Expect(secretResources).To(HaveLen(1))
				Expect(secretResources[0].GetNamespace()).To(Equal(secretNamespace))
				Expect(secretResources[0].GetName()).To(Equal("my-bootstrap-data"))
				Expect(secretResources[0].(*corev1.Secret).Data).To(HaveKeyWithValue(
					"bob-wilsons-password", []byte("password")))
			})

			When("There are also files", func() {
				BeforeEach(func() {
					cloudConfig.WriteFiles = []vmopv1cloudinit.WriteFile{
						{
							Path: "/hello",
						},
					}
				})
				When("The files does not reference a secret", func() {
					BeforeEach(func() {
						cloudConfig.WriteFiles[0].Content = []byte("world")
					})
					It("Should return valid data", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(secretResources).To(HaveLen(1))
						Expect(secretResources[0].GetNamespace()).To(Equal(secretNamespace))
						Expect(secretResources[0].GetName()).To(Equal("my-bootstrap-data"))
						Expect(secretResources[0].(*corev1.Secret).Data).To(HaveLen(1))
						Expect(secretResources[0].(*corev1.Secret).Data).To(HaveKeyWithValue(
							"bob-wilsons-password", []byte("password")))
					})
				})
				When("The files reference secrets", func() {
					When("The file references an existing secret", func() {
						BeforeEach(func() {
							cloudConfig.WriteFiles[0].Content = []byte("name: \"my-bootstrap-data\"\nkey: \"/hello\"")
							initialObjects[0].(*corev1.Secret).Data["/hello"] = []byte("world")
						})
						It("Should return valid data", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(secretResources).To(HaveLen(1))
							Expect(secretResources[0].GetNamespace()).To(Equal(secretNamespace))
							Expect(secretResources[0].GetName()).To(Equal("my-bootstrap-data"))
							Expect(secretResources[0].(*corev1.Secret).Data).To(HaveLen(2))
							Expect(secretResources[0].(*corev1.Secret).Data).To(HaveKeyWithValue(
								"bob-wilsons-password", []byte("password")))
							Expect(secretResources[0].(*corev1.Secret).Data).To(HaveKeyWithValue(
								"/hello", []byte("world")))
						})
					})
					When("The file references an different secret", func() {
						BeforeEach(func() {
							cloudConfig.WriteFiles[0].Content = []byte("name: \"my-file-data\"\nkey: \"/hello\"")
							initialObjects = append(initialObjects,
								&corev1.Secret{
									TypeMeta: metav1.TypeMeta{
										APIVersion: "v1",
										Kind:       "Secret",
									},
									ObjectMeta: metav1.ObjectMeta{
										Namespace: secretNamespace,
										Name:      "my-file-data",
									},
									Data: map[string][]byte{
										"/hello": []byte("world"),
									},
								},
							)
						})
						It("Should return valid data", func() {
							Expect(err).ToNot(HaveOccurred())
							Expect(secretResources).To(HaveLen(2))
							Expect(secretResources[0].GetNamespace()).To(Equal(secretNamespace))
							Expect(secretResources[0].GetName()).To(Equal("my-bootstrap-data"))
							Expect(secretResources[0].(*corev1.Secret).Data).To(HaveLen(1))
							Expect(secretResources[0].(*corev1.Secret).Data).To(HaveKeyWithValue(
								"bob-wilsons-password", []byte("password")))
							Expect(secretResources[1].GetNamespace()).To(Equal(secretNamespace))
							Expect(secretResources[1].GetName()).To(Equal("my-file-data"))
							Expect(secretResources[1].(*corev1.Secret).Data).To(HaveLen(1))
							Expect(secretResources[1].(*corev1.Secret).Data).To(HaveKeyWithValue(
								"/hello", []byte("world")))
						})
					})
				})
			})
		})
	})
})

const cloudConfigWithNoDefaultUser = `users:
  - hashed_passwd: "0123456789"
    name: bob.wilson
write_files:
  - content: world
    path: /hello
  - content: there
    path: /hi
`

const cloudConfigWithDefaultUser = `users:
  - default
  - hashed_passwd: "0123456789"
    name: bob.wilson
write_files:
  - content: world
    path: /hello
  - content: there
    path: /hi
`

const cloudConfigWithWithRunCmds = `users:
  - hashed_passwd: "0123456789"
    name: bob.wilson
runcmd:
  - ls /
  - - ls
    - -a
    - -l
    - /
  - - echo
    - hello, world.
`
