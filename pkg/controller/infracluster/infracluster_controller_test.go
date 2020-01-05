// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infracluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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

			// Install VMOP ConfigMap
			clientSet := kubernetes.NewForConfigOrDie(cfg)
			err = vsphere.InstallVSphereVmProviderConfig(kubernetes.NewForConfigOrDie(cfg),
				integration.DefaultNamespace,
				integration.NewIntegrationVmOperatorConfig(vcSim.IP, vcSim.Port, integration.GetContentSourceID()),
				integration.SecretName)
			Expect(err).NotTo(HaveOccurred())

			expectedRequest := reconcile.Request{NamespacedName: wcpNamespacedName}
			recFn, requests, _ := integration.SetupTestReconcile(newReconciler(mgr))
			Expect(add(mgr, recFn)).To(Succeed())

			// Create the WCP Cluster ConfigMap object and expect the Reconcile to update VMOP ConfigMap
			pnid := "pnid-01"
			wcpConfigMap := BuildNewWcpClusterConfigMap(pnid)
			err = c.Create(context.TODO(), &wcpConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Validate if PNID changed
			providerConfig, err := vsphere.GetProviderConfigFromConfigMap(clientSet, integration.DefaultNamespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Update WCP Cluster ConfigMap
			pnid = "pnid-02"
			wcpConfigMapUpdated := BuildNewWcpClusterConfigMap(pnid)
			err = c.Update(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).NotTo(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			providerConfig, err = vsphere.GetProviderConfigFromConfigMap(clientSet, integration.DefaultNamespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Update ConfigMap with the same PNID
			wcpConfigMapUpdated = BuildNewWcpClusterConfigMap(pnid)
			err = c.Update(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).NotTo(HaveOccurred())
			Eventually(requests, timeout).ShouldNot(Receive(Equal(expectedRequest)))
			providerConfig, err = vsphere.GetProviderConfigFromConfigMap(clientSet, integration.DefaultNamespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pnid).Should(Equal(providerConfig.VcPNID))

			// Delete WCP ConfigMap
			err = c.Delete(context.TODO(), &wcpConfigMapUpdated)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

func BuildNewWcpClusterConfigMap(pnid string) v1.ConfigMap {
	wcpClusterConfig := &vsphere.WcpClusterConfig{
		VCHost: pnid,
		VCPort: vsphere.DefaultVCPort,
	}

	configMap, err := vsphere.BuildNewWcpClusterConfigMap(wcpClusterConfig)
	Expect(err).NotTo(HaveOccurred())

	return configMap
}
