// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package encoding_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/vmware-tanzu/vm-operator/controllers/util/encoding"
)

var _ = Describe("DecodeYAML", func() {
	var (
		data    []byte
		chanErr <-chan error
		chanObj <-chan *unstructured.Unstructured
	)

	JustBeforeEach(func() {
		chanObj, chanErr = encoding.DecodeYAML(data)
	})

	AfterEach(func() {
		data = nil
		chanErr = nil
		chanObj = nil
	})

	Context("Cloud provider conf", func() {
		BeforeEach(func() {
			data = []byte(decodeYAMLTestPayload)
		})

		It("Should decode successfully", func() {
			Consistently(chanErr).ShouldNot(Receive())
		})

		It("Should receive nine objects", func() {
			getObjects := func() ([]*unstructured.Unstructured, error) {
				var objects []*unstructured.Unstructured
				for {
					select {
					case obj := <-chanObj:
						if obj == nil {
							return objects, nil
						}
						objects = append(objects, obj)
					case err := <-chanErr:
						if err == nil {
							return objects, nil
						}
						return nil, err
					}
				}
			}
			objects, err := getObjects()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(objects).To(HaveLen(9))

			Expect(objects[0].GetKind()).To(Equal("Namespace"))
			Expect(objects[0].GetName()).To(Equal("vmware-system-cloud-provider"))

			Expect(objects[1].GetKind()).To(Equal("ServiceAccount"))
			Expect(objects[1].GetName()).To(Equal("cloud-provider-svc-account"))

			Expect(objects[2].GetKind()).To(Equal("ClusterRole"))
			Expect(objects[2].GetName()).To(Equal("cloud-provider-cluster-role"))

			Expect(objects[3].GetKind()).To(Equal("ClusterRole"))
			Expect(objects[3].GetName()).To(Equal("cloud-provider-patch-cluster-role"))

			Expect(objects[4].GetKind()).To(Equal("ClusterRoleBinding"))
			Expect(objects[4].GetName()).To(Equal("cloud-provider-cluster-role-binding"))

			Expect(objects[5].GetKind()).To(Equal("ClusterRoleBinding"))
			Expect(objects[5].GetName()).To(Equal("cloud-provider-patch-cluster-role-binding"))

			Expect(objects[6].GetKind()).To(Equal("ConfigMap"))
			Expect(objects[6].GetName()).To(Equal("ccm-cloud-config"))

			Expect(objects[7].GetKind()).To(Equal("ConfigMap"))
			Expect(objects[7].GetName()).To(Equal("ccm-kubeconfig"))

			Expect(objects[8].GetKind()).To(Equal("Deployment"))
			Expect(objects[8].GetName()).To(Equal("guest-cluster-cloud-provider"))
		})
	})
})

const decodeYAMLTestPayload = `
apiVersion: v1
kind: Namespace
metadata:
  name: vmware-system-cloud-provider
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cloud-provider-svc-account
  namespace: vmware-system-cloud-provider
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cloud-provider-cluster-role
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  - services
  - nodes
  - endpoints
  verbs:
  - get
  - watch
  - list
- apiGroups:
  - "policy"
  resources:
  - podsecuritypolicies
  verbs:
  - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cloud-provider-patch-cluster-role
rules:
- apiGroups:
  - ""
  resources:
  - endpoints
  - events
  verbs:
  - create
  - update
  - replace
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cloud-provider-cluster-role-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cloud-provider-cluster-role
subjects:
- kind: ServiceAccount
  name: cloud-provider-svc-account
  namespace: vmware-system-cloud-provider
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cloud-provider-patch-cluster-role-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cloud-provider-patch-cluster-role
subjects:
- kind: ServiceAccount
  name: cloud-provider-svc-account
  namespace: vmware-system-cloud-provider
---
apiVersion: v1
data:
  cloud-config: data
kind: ConfigMap
metadata:
  name: ccm-cloud-config
  namespace: vmware-system-cloud-provider
---
apiVersion: v1
data:
  kubeconfig: data
kind: ConfigMap
metadata:
  name: ccm-kubeconfig
  namespace: vmware-system-cloud-provider
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: guest-cluster-cloud-provider
  namespace: vmware-system-cloud-provider
spec:
  selector:
    matchLabels:
      name: guest-cluster-cloud-provider
  template:
    metadata:
      labels:
        name: guest-cluster-cloud-provider
    spec:
      containers:
      - command:
        - /guest-cluster-cloud-provider
        - --controllers=service
        - --authentication-kubeconfig=/config/kubeconfig
        - --authorization-kubeconfig=/config/kubeconfig
        - --kubeconfig=/config/kubeconfig
        - --cloud-config=/config/cloud-config
        - --cluster-name={{ .ManagedClusterName }}
        image: guest-cluster-cloud-provider:latest
        imagePullPolicy: IfNotPresent
        name: guest-cluster-cloud-provider
        volumeMounts:
        - mountPath: /config
          name: ccm-config
          readOnly: true
      nodeSelector:
        node-role.kubernetes.io/master: ""
      serviceAccountName: cloud-provider-svc-account
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      volumes:
      - name: ccm-config
        projected:
          sources:
          - configMap:
              items:
              - key: kubeconfig
                path: kubeconfig
              name: ccm-kubeconfig
          - configMap:
              items:
              - key: cloud-config
                path: cloud-config
              name: ccm-cloud-config
`
