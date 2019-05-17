// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"testing"
	"time"

	"github.com/vmware-tanzu/vm-operator/test/integration"

	"github.com/onsi/gomega"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	ns := integration.DefaultNamespace
	name := "fooVmService"

	port := vmoperatorv1alpha1.VirtualMachineServicePort{
		Name:       "foo",
		Protocol:   "TCP",
		Port:       42,
		TargetPort: 42,
	}

	instance := &vmoperatorv1alpha1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
			Type:     "ClusterIP",
			Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{port},
			Selector: map[string]string{"foo": "bar"},
		},
	}

	expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}

	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the VirtualMachineService object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		_ = c.Delete(context.TODO(), instance)
	}()
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	/*
		deploy := &appsv1.Deployment{}
		g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
			Should(gomega.Succeed())

		// Delete the Deployment and expect Reconcile to be called for Deployment deletion
		g.Expect(c.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
		g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
			Should(gomega.Succeed())

		// Manually delete Deployment since GC isn't enabled in the test control plane
		g.Eventually(func() error { return c.Delete(context.TODO(), deploy) }, timeout).
			Should(gomega.MatchError("deployments.apps \"foo-deployment\" not found"))
	*/
}
