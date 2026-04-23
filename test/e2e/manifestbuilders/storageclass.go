package manifestbuilders

import (
	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

func GetStorageQuotaYAML() ([]byte, error) {
	dir := "test/e2e/fixtures/yaml/vmoperator/storageclass"
	yaml := fixtures.ReadFileBytes(dir, "gc-storage-quota.yaml")

	return yaml, nil
}

func GetStorageClassYAML() ([]byte, error) {
	dir := "test/e2e/fixtures/yaml/vmoperator/storageclass"
	yaml := fixtures.ReadFileBytes(dir, "gc-storage-profile.yaml")

	return yaml, nil
}
