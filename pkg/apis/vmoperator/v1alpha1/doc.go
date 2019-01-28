/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Api versions allow the api contract for a resource to be changed while keeping
// backward compatibility by support multiple concurrent versions
// of the same resource

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=vmware.com/kubevsphere/pkg/apis/vmoperator
// +k8s:defaulter-gen=TypeMeta
// +groupName=vmoperator.vmware.com
package v1alpha1 // import "vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
