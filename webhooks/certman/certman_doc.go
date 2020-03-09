// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package certman provides a manager that generates and rotates self-signed
// certificates for a controller-manager's webhook server.
//
// The self-signed certificates are generated into a configured Secret
// resource that must be mounted to the container running the webhook
// server.
//
// The certificate manager updates the CA bundle in any webhook configurations
// that match a configurable label selector.
//
// Controller-runtime v0.2+ automatically monitors the configured certificate
// directory and restarts the webhook server if the certificates are changed.
package certman
