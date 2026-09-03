package testutils

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	executedDuration   = 5 * time.Minute
	pollDuration       = 5 * time.Second
	StorageAppSelector = "gc-e2e-storage"
)

// AssertCreatePVC creates a PVC under a namespace with provided storageclass.
func AssertCreatePVC(client ctrlclient.Client, name, namespace, storageClassName string) {
	ctx := context.TODO()

	// Create the PVC.
	Eventually(func() error {
		pvc := ConstructPVC(name, storageClassName)
		pvc.Namespace = namespace
		return client.Create(ctx, pvc)
	}, executedDuration, pollDuration).Should(Succeed())

	// Validate the PVC is bound
	By("Validating the creation of pvc")

	timeouts := e2eframework.NewTimeoutContext()
	Eventually(func(g Gomega) {
		got := &corev1.PersistentVolumeClaim{}
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, got)).To(Succeed())
		g.Expect(got.Status.Phase).To(Equal(corev1.ClaimBound))
	}, timeouts.ClaimBound, 2*time.Second).Should(Succeed())
}

// ConstructPVC returns a PVC object with user-specified pvcname and storage class.
func ConstructPVC(pvcName, storageClassName string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.ObjectMeta.Name = pvcName
	pvc.ObjectMeta.Annotations = map[string]string{
		"volume.beta.kubernetes.io/storage-class": storageClassName,
	}
	pvc.ObjectMeta.Labels = map[string]string{
		"app":  StorageAppSelector,
		"type": "pvc",
	}
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}
	pvc.Spec.Resources = corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{
			"storage": resource.MustParse("100Mi"),
		},
	}
	pvc.Spec.StorageClassName = &storageClassName

	return pvc
}
