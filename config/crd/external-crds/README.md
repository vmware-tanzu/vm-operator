# External CRDs

## Overview

This directory contains external CRD manifests consumed by VM Operator. Some are included in the Supervisor install kustomization (`config/default`) and therefore deployed in production; others are used only in integration tests.

## Production CRDs

These CRDs are referenced by `config/default/kustomization.yaml` and are installed alongside VM Operator on Supervisor:

* `encryption.vmware.com_encryptionclasses.yaml` — EncryptionClass API (bring-your-own-key support)
* `vsphere.policy.vmware.com_computepolicies.yaml` — vSphere compute policy API
* `vsphere.policy.vmware.com_policyevaluations.yaml` — vSphere policy evaluation API
* `vsphere.policy.vmware.com_tagpolicies.yaml` — vSphere tag policy API
* `infra.vmware.com_storagepolicies.yaml` — Infrastructure storage policy API
* `vim.vmware.com_configtargets.yaml` — VIM config target API
* `vim.vmware.com_virtualmachineconfigoptions.yaml` — VIM VM config options API
* `vim.vmware.com_virtualmachineconfigpolicies.yaml` — VIM VM config policy API
* `vim.vmware.com_virtualmachineguestoptions.yaml` — VIM VM guest options API

## Integration Test CRDs

These CRDs are referenced only in local dev (`config/local`) or test suite setup files and are not installed on Supervisor:

* `cnsnodevmattachment-crd.yaml` is used by volume controller for the integration tests
* `cnsnodevmbatchattachments.yaml` is used by batch volume controller for the integration tests
* `cnsregistervolumes.yaml` is used by both volume controllers for the integration tests
* `topology.tanzu.vmware.com_availabilityzones.yaml` is used by the VM Operator integration tests
* `imageregistry.vmware.com_contentlibraries.yaml` is used by virtualmachinepublishrequest_controller_suite_test.go for the integration tests
* `imageregistry.vmware.com_clustercontentlibraryitems.yaml` is used by the clustercontentlibraryitem_controller_suite_test.go for the integration tests
* `imageregistry.vmware.com_contentlibraryitems.yaml` is used by the contentlibraryitem_controller_suite_test.go for the integration tests
