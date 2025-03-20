# FAFT for firmware testing

[TOC]

## Overview

The firmware tests implemented in FAFT are run throughout a platform enablement
and they are also run as part of firmware qualification.

Each firmware FAFT test is tagged with multiple attributes, allowing one to
tailor the list of tests run to the specific platform enablement gate or
firmware qualification type.

## Platform Enablement Gates

Every Platform Enablement defines these software readiness gates (PE SW Gates)
that require passing a specific list of firmware tests to successfully exit the
gate.

* Enabled
* Meets KPI
* Stressed

> TODO: Link to the PE SW Gates document once public

### firmware_enabled

The enabled tests must pass 100% before before exiting the Enabled gate.

These tests can be run either using an SSH connection to the DUT (preferred), or
they can be run in `noSSH` mode.  See the [FAFT for bringup](./bringup.md) for
details on running tests without SSH.

When adding the `firmware_enabled` attribute to any test you must also add both
the `firmware_meet_kpi` and `firmware_stressed` attributes.  There is a pre-submit check
that verifies this.

The enabled tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
tast run $DUT_IP '("group:firmware" && firmware_enabled)'
```

### firmware_meet_kpi

The meets KPI tests include almost all the firmware tests, including all
the tests tagged with the `firmware_enabled` attribute.

These tests require an SSH connection to the DUT.

When adding the `firmware_meet_kpi` attribute to any test you must also add the
`firmware_stressed` attribute.  There is a pre-submit check that verifies this.

The meets KPI tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
tast run $DUT_IP '("group:firmware" && firmware_meets_kpi)'
```

### firmware_stressed

The stressed tests include all the firmware tests, with the exception of
tests tagged with the `firmware_experimental` attribute.

These tests require an SSH connection to the DUT.

The stressed tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
tast run $DUT_IP '("group:firmware" && firmware_stressed)'
```

## Firmware Qualification

> TODO: add attributes for FW qualification.
