#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

PKG=github.com/vmware-tanzu/vm-operator

apiregister-gen --verify-only --input-dirs $PKG/pkg/apis/... --input-dirs $PKG/pkg/controller/...

conversion-gen --verify-only --input-dirs $PKG/pkg/apis/vmoperator/v1alpha1 --input-dirs $PKG/pkg/apis/vmoperator --go-header-file boilerplate.go.txt \
	-O zz_generated.conversion --extra-peer-dirs k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/conversion,k8s.io/apimachinery/pkg/runtime 

defaulter-gen --verify-only --input-dirs $PKG/pkg/apis/vmoperator/v1alpha1 --input-dirs $PKG/pkg/apis/vmoperator --go-header-file boilerplate.go.txt \
	-O zz_generated.defaults --extra-peer-dirs= k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/conversion,k8s.io/apimachinery/pkg/runtime

client-gen --verify-only --go-header-file boilerplate.go.txt --input-base $PKG/pkg/apis --input vmoperator/v1alpha1 \
	--clientset-path $PKG/pkg/client/clientset_generated --clientset-name clientset

lister-gen --verify-only --input-dirs $PKG/pkg/apis/vmoperator/v1alpha1 --go-header-file boilerplate.go.txt --output-package $PKG/pkg/client/listers_generated

informer-gen --verify-only --input-dirs $PKG/pkg/apis/vmoperator/v1alpha1 --go-header-file boilerplate.go.txt --output-package $PKG/pkg/client/informers_generated \
	--listers-package $PKG/pkg/client/listers_generated --versioned-clientset-package $PKG/pkg/client/clientset_generated/clientset
