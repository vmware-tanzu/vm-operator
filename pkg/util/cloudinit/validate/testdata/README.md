# Cloud-Init test data for validation

## Overview

This directory contains test data files relates to Cloud-Init:

## Valid test data

* [`valid-cloud-config-1.yaml`](./valid-cloud-config-1.yaml)

    * **`Copied`** `2023/12/14`
    * **`Source`** https://cloudinit.readthedocs.io/en/latest/reference/examples.html#including-users-and-groups
    * **`--help`** An example, valid CloudConfig that includes several users and groups

## Invalid test data

* [`invalid-cloud-config-1.yaml`](./valid-cloud-config-1.yaml)

    * **`--help`** A copy of `valid-cloud-config-1.yaml`, where the first, non-default user has a name that is an array rather than a string.

