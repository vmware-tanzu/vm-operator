# Cloud-Init schemas

## Overview

This directory contains schema files relates to Cloud-Init:

* [`schema-cloud-config-v1.json`](./schema-cloud-config-v1.json)

    * **`Copied`** `2023/12/14`
    * **`Source`** https://github.com/canonical/cloud-init/blob/main/cloudinit/config/schemas/schema-cloud-config-v1.json
    * **`--help`** The Cloud-Init CloudConfig schema that may be used to validate user and vendor data

## Generating the Go source code

Run `make generate-go`. If the local system has `npm`, it is used to install `quicktype`, which is then used to generate `cloudconfig.go` from the schema. Otherwise a container image is built from `Dockerfile.quicktype` which is then used to generate the Go source code.
