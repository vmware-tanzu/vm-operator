package testutils

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
)

const (
	ExecutedDuration   = 5 * time.Minute
	PollDuration       = 5 * time.Second
	StorageAppSelector = "gc-e2e-storage"
)

// AssertCreatePVC creates a PVC under a namespace with provided storageclass.
func AssertCreatePVC(client kubernetes.Interface, name, namespace, storageClassName string) {
	ctx := context.TODO()
	// Create the PVC
	pvc := ConstructPVC(name, storageClassName)

	Eventually(func() error {
		_, err := client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		return nil
	}, ExecutedDuration, PollDuration).Should(Succeed())

	// Validate the PVC is bound
	By("Validating the creation of pvc")

	timeouts := e2eframework.NewTimeoutContext()
	err := e2epv.WaitForPersistentVolumeClaimPhase(context.Background(), corev1.ClaimBound, client, namespace, name, 2*time.Second, timeouts.ClaimBound)
	e2eframework.ExpectNoError(err)
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
