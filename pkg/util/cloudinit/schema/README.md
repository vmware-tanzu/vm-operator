# Cloud-Init schemas

## Overview

This directory contains schema files relates to Cloud-Init:

* [`schema-cloud-config-v1.json`](./schema-cloud-config-v1.json)

    * **`Copied`** `2023/12/14`
    * **`Source`** https://github.com/canonical/cloud-init/blob/main/cloudinit/config/schemas/schema-cloud-config-v1.json
    * **`--help`** The Cloud-Init CloudConfig schema that may be used to validate user and vendor data

## Generating the Go source code

Run `make generate-go` to generate the Go source code. If the local system has `npm`, it is used, otherwise a container image is used with either Docker or Podman.

This relies on `npm ci quicktype`. If you need to use a private npm registry which might require authentication,
you can set `NPM_TOKEN` and `NPM_REGISTRY` variables.`
