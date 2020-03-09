// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var c client.Client

const timeout = time.Second * 5

var _ = Describe("InfraClusterProvider controller", func() {
	ns := integration.DefaultNamespace
	var (
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)

		err = vsphere.InstallVSphereVmProviderConfig(clientSet,
			ns,
			integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
			integration.SecretName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/updating WCP Cluster Config ConfigMap", func() {

		It("invoke the reconcile method while creating/updating WCP CLuster Config ConfigMap", func() {
			wcpNamespacedName := types.NamespacedName{
				Name:      vsphere.WcpClusterConfigMapName,
				Namespace: vsphere.WcpClusterConfigMapNamespace,
			}

			ctrlContext := &controllerContext.ControllerManagerContext{
				VmProvider: vmprovider.GetService().GetRegisteredVmProviderOrDie(),
			}

			expectedRequest := reconcile.Request{NamespacedName: wcpNamespacedName}
			recFn, requests, _, _ := integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
			Expect(add(mgr, recFn)).To(Succeed())

			// Create the WCP Cluster ConfigMap object and expect the Reconcile to update VMOP ConfigMap
			pnid := "pnid-01"
			wcpConfigMap := BuildNewWcpClusterConfigMap(pnid)
			err = c.Create(context.TODO(), &wcpConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Validate if PNID changed
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Update WCP Cluster ConfigMap
			pnid = "pnid-02"
			wcpConfigMapUpdated := BuildNewWcpClusterConfigMap(pnid)
			err = c.Update(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).NotTo(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			providerConfig, err = vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Update ConfigMap with the same PNID
			wcpConfigMapUpdated = BuildNewWcpClusterConfigMap(pnid)
			err = c.Update(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).NotTo(HaveOccurred())
			Eventually(requests, timeout).ShouldNot(Receive(Equal(expectedRequest)))
			providerConfig, err = vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Delete WCP ConfigMap
			err = c.Delete(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("VM operator Service Account secret rotation", func() {
		var (
			correctUserName   string
			correctPassword   string
			incorrectUserName string
			incorrectPassword string
		)

		BeforeEach(func() {
			correctUserName = "correctUsername"
			correctPassword = "correctPassword"
			incorrectUserName = "incorrectUserName"
			incorrectPassword = "incorrectPassword"

			providerCreds := vsphere.VSphereVmProviderCredentials{
				Username: correctUserName,
				Password: correctPassword,
			}
			Expect(vsphere.InstallVSphereVmProviderSecret(clientSet, ns, &providerCreds, vsphere.VmOpSecretName)).To(Succeed())

			vcSim.Server.URL.User = url.UserPassword(correctUserName, correctPassword)
			vcSim.VcSim.Service.Listen = vcSim.Server.URL
		})

		Context("without rotation", func() {
			It("should not fail", func() {
				providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = vsphere.NewClient(context.TODO(), providerConfig)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when a secret is updated to an invalid cred", func() {
			It("should fail to initialize the new session", func() {
				providerCreds := vsphere.VSphereVmProviderCredentials{
					Username: incorrectUserName,
					Password: incorrectPassword,
				}

				Expect(vsphere.InstallVSphereVmProviderSecret(clientSet, ns, &providerCreds, vsphere.VmOpSecretName)).To(Succeed())
				providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = vsphere.NewClient(context.TODO(), providerConfig)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("login failed for url"))
			})
		})

		Context("when secret is rotated to a valid cred", func() {
			It("should re-initialize all the sessions", func() {
				By("update vcsim to use a different cred")
				newUserName := "newUserName"
				newPassword := "newPassword"

				vcSim.Server.URL.User = url.UserPassword(newUserName, newPassword)
				vcSim.VcSim.Service.Listen = vcSim.Server.URL

				providerCreds := vsphere.VSphereVmProviderCredentials{
					Username: newUserName,
					Password: newPassword,
				}

				Expect(vsphere.InstallVSphereVmProviderSecret(clientSet, ns, &providerCreds, vsphere.VmOpSecretName)).To(Succeed())
				providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, ns)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = vsphere.NewClient(context.TODO(), providerConfig)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

func BuildNewWcpClusterConfigMap(pnid string) v1.ConfigMap {
	wcpClusterConfig := &vsphere.WcpClusterConfig{
		VcPNID: pnid,
		VcPort: vsphere.DefaultVCPort,
	}

	configMap, err := vsphere.BuildNewWcpClusterConfigMap(wcpClusterConfig)
	Expect(err).NotTo(HaveOccurred())

	return configMap
}
