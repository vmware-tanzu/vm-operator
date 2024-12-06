# Netplan schemas

## Overview

This directory contains schema files relates to Netplan:

* [`schema.json`](./schema.json)

    * **`Copied`** `2024/11/112`
    * **`Source`** https://github.com/TobiasDeBruijn/netplan-types
    * **`--help`** Generate the schema with `make schema.json`

## Generating the Go source code

Run `make generate-go` to generate the Go source code. If the local system has `npm`, it is used, otherwise a container image is used with either Docker or Podman.
