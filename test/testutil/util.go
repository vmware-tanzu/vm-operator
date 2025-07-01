// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/onsi/ginkgo/v2" //nolint:depguard
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	klog "k8s.io/klog/v2"
)

// GetRootDir returns the root directory of this git repo.
func GetRootDir() (string, error) {
	_, s, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not determine Caller to get root path")
	}
	t := "test" + string(os.PathSeparator)
	i := strings.Index(s, t)
	if i < 0 {
		return "", fmt.Errorf("could not determine Caller to get root path")
	}
	return s[:i], nil
}

// GetRootDirOrDie returns the root directory of this git repo or dies.
func GetRootDirOrDie() string {
	rootDir, err := GetRootDir()
	if err != nil {
		klog.Fatal(err)
	}
	return rootDir
}

// FindModuleDir returns the on-disk directory for the provided Go module.
func FindModuleDir(module string) string {
	cmd := exec.Command("go", "mod", "download", "-json", module)
	out, err := cmd.Output()
	if err != nil {
		klog.Fatalf("Failed to run go mod to find module %q directory", module)
	}
	info := struct{ Dir string }{}
	if err := json.Unmarshal(out, &info); err != nil {
		klog.Fatalf("Failed to unmarshal output from go mod command: %v", err)
	} else if info.Dir == "" {
		klog.Fatalf("Failed to find go module %q directory, received %v", module, string(out))
	}
	return info.Dir
}

// WriteKubeConfig writes an existing *rest.Config out as the typical
// KubeConfig YAML data.
func WriteKubeConfig(config *rest.Config) ([]byte, error) {
	return clientcmd.Write(api.Config{
		Clusters: map[string]*api.Cluster{
			config.ServerName: {
				Server:                   config.Host,
				CertificateAuthorityData: config.CAData,
			},
		},
		Contexts: map[string]*api.Context{
			config.ServerName: {
				Cluster:  config.ServerName,
				AuthInfo: config.Username,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			config.Username: {
				ClientKeyData:         config.KeyData,
				ClientCertificateData: config.CertData,
			},
		},
		CurrentContext: config.ServerName,
	})
}

func ContainsError(err error, message string) bool {
	for {
		if err == nil {
			return false
		}
		if err.Error() == message {
			return true
		}

		var unwrap unwrappableError
		var unwrapSlice unwrappableErrorSlice
		switch {
		case errors.As(err, &unwrap):
			err = unwrap.Unwrap()
		case errors.As(err, &unwrapSlice):
			for _, uErr := range unwrapSlice.Unwrap() {
				if ContainsError(uErr, message) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

type unwrappableError interface {
	Unwrap() error
}

type unwrappableErrorSlice interface {
	Unwrap() []error
}

// GinkgoLogr returns a new logr.Logger that uses the GinkgoWriter with the
// specified verbosity.
func GinkgoLogr(v int) logr.Logger {
	return funcr.New(func(prefix, args string) {
		if prefix == "" {
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "%s\n", args)
		} else {
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "%s %s\n", prefix, args)
		}
	}, funcr.Options{
		Verbosity: v,
	})
}
