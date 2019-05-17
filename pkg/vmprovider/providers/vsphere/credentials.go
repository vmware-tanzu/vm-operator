package vsphere

import (
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VSphereVmProviderCredentials wraps the data needed to login to vCenter.
type VSphereVmProviderCredentials struct {
	Username string
	Password string
}

func GetSecret(clientset *kubernetes.Clientset, secretNamespace string, secretName string) (*v1.Secret, error) {
	secret, err := clientset.CoreV1().Secrets(secretNamespace).Get(secretName, meta_v1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot find secret %s in namespace %s", secretName, secretNamespace)
	}

	return secret, nil
}

func GetProviderCredentials(clientset *kubernetes.Clientset, namespace string, secretName string) (*VSphereVmProviderCredentials, error) {
	secret, err := GetSecret(clientset, namespace, secretName)
	if err != nil {
		return nil, err
	}

	var credentials VSphereVmProviderCredentials
	credentials.Username = string(secret.Data["username"])
	credentials.Password = string(secret.Data["password"])

	if credentials.Username == "" || credentials.Password == "" {
		return nil, errors.New("vCenter username and password are missing")
	}

	return &credentials, nil
}
