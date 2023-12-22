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
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
)

var _ = Describe("CloudConfig MarshalYAML", func() {
	var (
		err                   error
		data                  string
		cloudConfig           vmopv1cloudinit.CloudConfig
		cloudConfigSecretData cloudinit.CloudConfigSecretData
	)

	BeforeEach(func() {
		err = nil
		data = ""
		cloudConfig = vmopv1cloudinit.CloudConfig{}
		cloudConfigSecretData = cloudinit.CloudConfigSecretData{}
	})

	JustBeforeEach(func() {
		data, err = cloudinit.MarshalYAML(cloudConfig, cloudConfigSecretData)
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
			Expect(data).To(Equal("## template: jinja\n#cloud-config\n\nusers:\n  - name: bob.wilson\n"))
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
				cloudConfig.DefaultUserEnabled = true
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
				RunCmd: []byte(`["ls /",["ls","-a","-l","/"],["echo","hello, world."]]`),
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

	When("CloudConfig has all possible fields set", func() {
		BeforeEach(func() {
			cloudConfig = vmopv1cloudinit.CloudConfig{
				Users: []vmopv1cloudinit.User{
					{
						Name:         "bob.wilson",
						CreateGroups: addrOf(false),
						ExpireDate:   addrOf("9999-99-99"),
						Gecos:        addrOf("gecos"),
						Groups:       []string{"group1", "group2"},
						HashedPasswd: &common.SecretKeySelector{
							Name: "my-bootstrap-data",
							Key:  "cloud-init-user-bob.wilson-hashed_passwd",
						},
						Homedir:           addrOf("/home/bob.wilson"),
						Inactive:          addrOf(int32(1)),
						LockPasswd:        addrOf(false),
						NoCreateHome:      addrOf(false),
						NoLogInit:         addrOf(false),
						PrimaryGroup:      addrOf("group1"),
						SELinuxUser:       addrOf("bob.wilson"),
						Shell:             addrOf("/bin/bash"),
						SnapUser:          addrOf("bob.wilson"),
						SSHAuthorizedKeys: []string{"key1", "key2"},
						SSHImportID:       []string{"id1", "id2"},
						SSHRedirectUser:   addrOf(false),
						Sudo:              addrOf("sudoyou?"),
						System:            addrOf(false),
						UID:               addrOf(int64(123)),
					},
					{
						Name:         "rob.wilson",
						CreateGroups: addrOf(true),
						ExpireDate:   addrOf("9999-99-99"),
						Gecos:        addrOf("gecos"),
						Groups:       []string{"group1", "group2"},
						Homedir:      addrOf("/home/rob.wilson"),
						Inactive:     addrOf(int32(10)),
						LockPasswd:   addrOf(true),
						NoCreateHome: addrOf(true),
						NoLogInit:    addrOf(true),
						Passwd: &common.SecretKeySelector{
							Name: "my-bootstrap-data",
							Key:  "cloud-init-user-rob.wilson-passwd",
						},
						PrimaryGroup:      addrOf("group1"),
						SELinuxUser:       addrOf("rob.wilson"),
						Shell:             addrOf("/bin/bash"),
						SnapUser:          addrOf("rob.wilson"),
						SSHAuthorizedKeys: []string{"key1", "key2"},
						SSHImportID:       []string{"id1", "id2"},
						SSHRedirectUser:   addrOf(true),
						Sudo:              addrOf("sudoyou?"),
						System:            addrOf(true),
						UID:               addrOf(int64(123)),
					},
				},
				RunCmd: []byte(`["ls /",["ls","-a","-l","/"],["echo","hello, world."]]`),
				WriteFiles: []vmopv1cloudinit.WriteFile{
					{
						Path:        "/hello",
						Content:     []byte(`"world"`),
						Append:      true,
						Defer:       true,
						Encoding:    vmopv1cloudinit.WriteFileEncodingTextPlain,
						Owner:       "bob.wilson:bob.wilson",
						Permissions: "0644",
					},
					{
						Path:        "/hi",
						Content:     []byte(`{"name":"my-bootstrap-data","key":"/hi"}`),
						Append:      false,
						Defer:       false,
						Encoding:    vmopv1cloudinit.WriteFileEncodingTextPlain,
						Owner:       "rob.wilson:rob.wilson",
						Permissions: "0755",
					},
					{
						Path:        "/doc",
						Content:     []byte(`"a multi-line\ndocument"`),
						Append:      true,
						Defer:       true,
						Encoding:    vmopv1cloudinit.WriteFileEncodingTextPlain,
						Owner:       "bob.wilson:bob.wilson",
						Permissions: "0644",
					},
				},
			}
			cloudConfigSecretData = cloudinit.CloudConfigSecretData{
				Users: map[string]cloudinit.CloudConfigUserSecretData{
					"bob.wilson": {
						HashPasswd: "0123456789",
					},
					"rob.wilson": {
						Passwd: "password",
					},
				},
				WriteFiles: map[string]string{
					"/hi": "there",
				},
			}
		})
		It("Should return user data", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(Equal(cloudConfigWithAllPossibleValues))
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

const cloudConfigWithNoDefaultUser = `## template: jinja
#cloud-config

users:
  - hashed_passwd: "0123456789"
    name: bob.wilson
write_files:
  - content: world
    path: /hello
  - content: there
    path: /hi
`

const cloudConfigWithDefaultUser = `## template: jinja
#cloud-config

users:
  - default
  - hashed_passwd: "0123456789"
    name: bob.wilson
write_files:
  - content: world
    path: /hello
  - content: there
    path: /hi
`

const cloudConfigWithWithRunCmds = `## template: jinja
#cloud-config

users:
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

const cloudConfigWithAllPossibleValues = `## template: jinja
#cloud-config

users:
  - create_groups: false
    expiredate: 9999-99-99
    gecos: gecos
    groups:
      - group1
      - group2
    hashed_passwd: "0123456789"
    homedir: /home/bob.wilson
    inactive: "1"
    lock_passwd: false
    name: bob.wilson
    no_create_home: false
    no_log_init: false
    primary_group: group1
    selinux_user: bob.wilson
    shell: /bin/bash
    snapuser: bob.wilson
    ssh_authorized_keys:
      - key1
      - key2
    ssh_import_id:
      - id1
      - id2
    ssh_redirect_user: false
    sudo: sudoyou?
    system: false
    uid: 123
  - create_groups: true
    expiredate: 9999-99-99
    gecos: gecos
    groups:
      - group1
      - group2
    homedir: /home/rob.wilson
    inactive: "10"
    lock_passwd: true
    name: rob.wilson
    no_create_home: true
    no_log_init: true
    passwd: password
    primary_group: group1
    selinux_user: rob.wilson
    shell: /bin/bash
    snapuser: rob.wilson
    ssh_authorized_keys:
      - key1
      - key2
    ssh_import_id:
      - id1
      - id2
    ssh_redirect_user: true
    sudo: sudoyou?
    system: true
    uid: 123
runcmd:
  - ls /
  - - ls
    - -a
    - -l
    - /
  - - echo
    - hello, world.
write_files:
  - append: true
    content: world
    defer: true
    encoding: text/plain
    owner: bob.wilson:bob.wilson
    path: /hello
    permissions: "0644"
  - content: there
    encoding: text/plain
    owner: rob.wilson:rob.wilson
    path: /hi
    permissions: "0755"
  - append: true
    content: |-
      a multi-line
      document
    defer: true
    encoding: text/plain
    owner: bob.wilson:bob.wilson
    path: /doc
    permissions: "0644"
`
