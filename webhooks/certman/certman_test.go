// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman_test

import (
	"bytes"
	"strconv"
	"sync"
	"time"

	//nolint
	. "github.com/onsi/ginkgo"
	//nolint
	. "github.com/onsi/gomega"
	admissionregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/webhooks/certman"
)

var _ = Describe("Rotating certificates", func() {
	var (
		done                     chan struct{}
		expectedRotationCountMin int
		expectedRotationCountMax int
		numCertificateManagers   int
		options                  certman.Options
		rotationInterval         time.Duration
		secretKey                client.ObjectKey
		webhookConfigKey         client.ObjectKey
		totalRotationDuration    time.Duration
		waitForCertMansToStop    sync.WaitGroup
	)
	BeforeEach(func() {
		done = make(chan struct{})
		expectedRotationCountMin = 3
		expectedRotationCountMax = 5
		numCertificateManagers = 1
		rotationInterval = time.Second * 5
		totalRotationDuration = time.Second * 20
		waitForCertMansToStop = sync.WaitGroup{}
	})
	JustBeforeEach(func() {
		By("installing the secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "default",
					GenerateName: "test-",
					Annotations: map[string]string{
						"certman.io/rotation-interval": rotationInterval.String(),
					},
				},
			}
			Expect(k8sClient.Create(suiteCtx, secret)).To(Succeed())
			secretKey = client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
		})
		By("installing the webhook", func() {
			failurePolicy := admissionregv1.Ignore
			webhookURL := "https://not.a.real.address.local:9443/validate-secrets"
			webhookConfig := &admissionregv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "default",
					GenerateName: "test-",
					Labels: map[string]string{
						"certman.io": "true",
					},
				},
				Webhooks: []admissionregv1.ValidatingWebhook{
					{
						Name: "validate.configmaps.io",
						ClientConfig: admissionregv1.WebhookClientConfig{
							CABundle: []byte{'\n'},
							URL:      &webhookURL,
						},
						FailurePolicy: &failurePolicy,
						Rules:         []admissionregv1.RuleWithOperations{},
					},
				},
			}
			Expect(k8sClient.Create(suiteCtx, webhookConfig)).To(Succeed())
			webhookConfigKey = client.ObjectKey{Namespace: webhookConfig.Namespace, Name: webhookConfig.Name}
		})

		// Define the certificate manager options.
		options = certman.Options{
			Client:                        k8sClient,
			Logger:                        logf.Log,
			WebhookConfigLabelKey:         "certman.io",
			NextRotationAnnotationKey:     "certman.io/next-rotation",
			RotationCountAnnotationKey:    "certman.io/rotation-count",
			RotationIntervalAnnotationKey: "certman.io/rotation-interval",
			SecretNamespace:               secretKey.Namespace,
			SecretName:                    secretKey.Name,
			ServiceNamespace:              "default",
			ServiceName:                   "webhook-service",
		}

		// Launch multiple certificate managers in parallel.
		for i := 0; i < numCertificateManagers; i++ {
			waitForCertMansToStop.Add(1)
			cm, err := certman.New(options)
			Expect(err).ShouldNot(HaveOccurred())
			go func() {
				defer GinkgoRecover()
				defer waitForCertMansToStop.Done()
				Expect(cm.Start(done)).To(Succeed())
			}()
		}

		// Background cancel the done channel after some time.
		time.AfterFunc(totalRotationDuration, func() { close(done) })
	})
	JustAfterEach(func() {
		// Wait for the controller managers to stop.
		waitForCertMansToStop.Wait()

		// Get the secret.
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(suiteCtx, secretKey, secret)).To(Succeed())

		// ASSERT that the number of rotations is equal to or within one of
		// the expected count. Because the manager is cancelled before we're
		// sure it started a given cycle, and due to load, there's a plus/minus
		// of one room-for-error.
		Expect(secret.Annotations).ToNot(BeNil())
		Expect(secret.Annotations[options.RotationCountAnnotationKey]).ToNot(BeEmpty())
		actualRotationCount, err := strconv.Atoi(secret.Annotations[options.RotationCountAnnotationKey])
		Expect(err).ShouldNot(HaveOccurred())
		Expect(actualRotationCount).To(BeNumerically(">=", expectedRotationCountMin))
		Expect(actualRotationCount).To(BeNumerically("<=", expectedRotationCountMax))

		// Get the webhook config.
		webhookConfig := &admissionregv1.ValidatingWebhookConfiguration{}
		Expect(k8sClient.Get(suiteCtx, webhookConfigKey, webhookConfig)).To(Succeed())

		// ASSERT that the webhook config was updated.
		Expect(webhookConfig.Webhooks).To(HaveLen(1))
		Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).ToNot(BeNil())
		Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).To(Equal(secret.Data[certman.CACertName]))
	})
	AfterEach(func() {
		done = nil
		expectedRotationCountMin = 0
		expectedRotationCountMax = 0
		numCertificateManagers = 0
		rotationInterval = 0
		totalRotationDuration = 0
		options = certman.Options{}
		secretKey = client.ObjectKey{}
		webhookConfigKey = client.ObjectKey{}
		waitForCertMansToStop = sync.WaitGroup{}
	})
	When("there is a single certificate manager", func() {
		It("should rotate the certificates three-to-five times in 20s", func() {
			// ASSERT happens in the JustAfterEach above
		})
	})
	When("there are three certificate managers", func() {
		BeforeEach(func() {
			numCertificateManagers = 3
		})
		It("should rotate the certificates three-to-five times in 20s", func() {
			// ASSERT happens in the JustAfterEach above
		})
	})
	When("there are seven certificate managers", func() {
		BeforeEach(func() {
			numCertificateManagers = 7
		})
		It("should rotate the certificates three-to-five times in 20s", func() {
			// ASSERT happens in the JustAfterEach above
		})
	})
	When("the webhook YAML is reapplied and a new certificate manager starts", func() {
		BeforeEach(func() {
			rotationInterval = time.Minute * 1
			expectedRotationCountMin = 1
			expectedRotationCountMax = 1
			totalRotationDuration = time.Second * 10
		})
		It("should update the webhook configurations outside the normal rotation interval", func() {
			// Wait for the secret's initial rotation.
			Eventually(func() (bool, error) {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(suiteCtx, secretKey, secret); err != nil {
					return false, err
				}
				if len(secret.Data[certman.CACertName]) == 0 {
					return false, nil
				}
				webhookConfig := &admissionregv1.ValidatingWebhookConfiguration{}
				if err := k8sClient.Get(suiteCtx, webhookConfigKey, webhookConfig); err != nil {
					return false, err
				}
				if len(webhookConfig.Webhooks) == 0 {
					return false, nil
				}
				return bytes.Equal(webhookConfig.Webhooks[0].ClientConfig.CABundle, secret.Data[certman.CACertName]), nil
			}, time.Second*30, time.Second*1).Should(BeTrue(), "webhook config CABundle should be set to CA from secret")

			// GET the webhook config.
			webhookConfig := &admissionregv1.ValidatingWebhookConfiguration{}
			Expect(k8sClient.Get(suiteCtx, webhookConfigKey, webhookConfig)).To(Succeed())

			// UPDATE the webhook config's CABundle back to a newline char '\n'
			webhookConfig.Webhooks[0].ClientConfig.CABundle = []byte{'\n'}
			Expect(k8sClient.Update(suiteCtx, webhookConfig)).To(Succeed())

			// ASSERT the webhook config is eventually a newline char.
			Eventually(func() (bool, error) {
				webhookConfig := &admissionregv1.ValidatingWebhookConfiguration{}
				if err := k8sClient.Get(suiteCtx, webhookConfigKey, webhookConfig); err != nil {
					return false, err
				}
				if len(webhookConfig.Webhooks) == 0 {
					return false, nil
				}
				return bytes.Equal(webhookConfig.Webhooks[0].ClientConfig.CABundle, []byte{'\n'}), nil
			}, time.Second*30, time.Second*1).Should(BeTrue(), "webhook config CABundle should be set to a newline char")

			// START a new certificate manager.
			waitForCertMansToStop.Add(1)
			cm, err := certman.New(options)
			Expect(err).ToNot(HaveOccurred())
			go func() {
				defer GinkgoRecover()
				defer waitForCertMansToStop.Done()
				Expect(cm.Start(done)).To(Succeed())
			}()
		})
	})
})
