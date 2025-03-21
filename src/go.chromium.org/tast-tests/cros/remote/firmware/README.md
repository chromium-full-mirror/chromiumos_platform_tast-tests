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

*   Enabled
*   Meets KPI
*   Stressed

> TODO: Link to the PE SW Gates document once public

### firmware_enabled

The enabled tests must pass 100% before before exiting the Enabled gate.

These tests can be run either using an SSH connection to the DUT (preferred), or
they can be run in `noSSH` mode. See the [FAFT for bringup](./bringup.md) for
details on running tests without SSH.

When adding the `firmware_enabled` attribute to any firmware test you must also
add both the `firmware_meet_kpi` and `firmware_stressed` attributes. There is a
pre-submit check that verifies this.

The enabled tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
# To run all tests
tast run $DUT_IP '("group:firmware" && firmware_enabled)'
# To run all tests except PD
tast run $DUT_IP '("group:firmware" && firmware_enabled && !firmware_pd)'
# EC only
tast run $DUT_IP '("group:firmware" && firmware_ec && firmware_enabled)'
# Bios only
tast run $DUT_IP '("group:firmware" && firmware_bios && firmware_enabled)'
# PD only
tast run $DUT_IP '("group:firmware" && firmware_pd && firmware_enabled)'

# EC enabled tests without SSH
tast run - '("group:firmware" && firmware_ec && firmware_enabled)'

```

### firmware_meet_kpi

The meets KPI tests include almost all the firmware tests, including all the
tests tagged with the `firmware_enabled` attribute.

These tests require an SSH connection to the DUT.

When adding the `firmware_meet_kpi` attribute to any test you must also add the
`firmware_stressed` attribute. There is a pre-submit check that verifies this.

The meets KPI tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
# To run all tests
tast run $DUT_IP '("group:firmware" && firmware_meets_kpi)'
# To run all tests except PD
tast run $DUT_IP '("group:firmware" && firmware_meets_kpi && !firmware_pd)'
# EC only
tast run $DUT_IP '("group:firmware" && firmware_ec && firmware_meets_kpi)'
# Bios only
tast run $DUT_IP '("group:firmware" && firmware_bios && firmware_meets_kpi)'
# PD only
tast run $DUT_IP '("group:firmware" && firmware_pd && firmware_meets_kpi)'
```

### firmware_stressed

The stressed tests include all the firmware tests.

These tests require an SSH connection to the DUT.

The stressed tests are run using:

```bash
(inside chroot)
DUT_IP=192.168.1.78 # Replace with actual IP of DUT
# To run all tests
tast run $DUT_IP '("group:firmware" && firmware_stressed)'
# To run all tests except PD
tast run $DUT_IP '("group:firmware" && firmware_stressed && !firmware_pd)'
# EC only
tast run $DUT_IP '("group:firmware" && firmware_ec && firmware_stressed)'
# Bios only
tast run $DUT_IP '("group:firmware" && firmware_bios && firmware_stressed)'
# PD only
tast run $DUT_IP '("group:firmware" && firmware_pd && firmware_stressed)'
```

## Firmware Qualification

The attributes for a firmware qualification are designed for modular runs of the
tests. Unfortunately there are still a tiny number of autotests that need to be
run for BIOS (AP) qualifications, so the steps are:

1.  If you are performing a AP BIOS qualification of any kind run the autotests

    ```bash
    (inside chroot)
    test_that --autotest_dir ~/chromiumos/src/third_party/autotest/files/ --board=$BOARD --model=$MODEL $DUT_IP --args "servo_host=$SERVO_HOST servo_port=$SERVO_PORT" suite:faft_bios_autotests
    ```

2.  Then run all the appropriate tast tests, that don't require a servo_micro or
    C2D2, removing any tags that don't apply to your qualification. For example,
    if everything is changing and you want to run all the tests:

    ```bash
    (inside chroot)
    tast run -var=servo=$SERVO_HOST:$SERVO_PORT $DUT_IP '("group:firmware" && !firmware_pd && (firmware_bios_ro || firmware_bios_rw || firmware_bios_pdc || firmware_ec_ro || firmware_ec_rw))'
    ```

    Or if you are only testing a new RW firmware for EC and AP with no new PDC
    firmware:

    ```bash
    (inside chroot)
    tast run -var=servo=$SERVO_HOST:$SERVO_PORT $DUT_IP '("group:firmware" && !firmware_pd && (firmware_bios_rw || firmware_ec_rw))'
    ```

3.  Perform a quick check to make sure that you need to run PD tests:

    ```bash
    (inside chroot)
    tast list $DUT_IP '("group:firmware" && firmware_pd && (firmware_bios_ro || firmware_bios_rw || firmware_bios_pdc || firmware_ec_ro || firmware_ec_rw))'
    ```

    if no tests are listed, then you are done, skip the rest of the steps.

4.  Then connect a servo_micro or C2D2 and run the PD tests on port 0

    ```bash
    (inside chroot)
    tast run -var=servo=$SERVO_HOST:$SERVO_PORT $DUT_IP '("group:firmware" && firmware_pd && (firmware_bios_ro || firmware_bios_rw || firmware_bios_pdc || firmware_ec_ro || firmware_ec_rw))'
    ```

    or:

    ```bash
    (inside chroot)
    tast run -var=servo=$SERVO_HOST:$SERVO_PORT $DUT_IP '("group:firmware" && firmware_pd && (firmware_bios_rw || firmware_ec_rw))'
    ```

5.  Then move the servo_v4p1 to the next USB-C port and run the previous step
    again.

### For test authors

When you write a new test add all the appropriate attributes to your test.

*   `group:firmware` - All firmware tests should have this attribute.
*   `firmware_ec` - If the test is verifying EC behavior and doesn't require the
    use of a servo_v4p1 + servo_micro (or C2D2).
*   `firmware_pd` - If the test should be run on every USB-C port and does
    require the use of a servo_v4p1 + servo_micro (or C2D2).
*   `firmware_bios` - If the test is verifying AP (BIOS) behavior.
*   `firmware_enabled` - If the test should be run at the Platform Enabled
    phase.
*   `firmware_meets_kpi` - If the test should be run at the Platform Meets KPI
    phase.
*   `firmware_stressed` - If the test should be run at the Platform Stressed
    phase.
*   `firmware_bios_ro` - If the test should be run for AP (BIOS) RO firmware
    qualifications.
*   `firmware_bios_rw` - If the test should be run for AP (BIOS) RW firmware
    qualifications.
*   `firmware_bios_pdc` - If the test should be run for AP (BIOS) RW firmware
    qualifications which bundle a new PDC firmware.
*   `firmware_ec_ro` - If the test should be run for EC RO firmware
    qualifications.
*   `firmware_ec_rw` - If the test should be run for EC RW firmware
    qualifications.

This means that a single test may have quite a lot of attributes. There are some
rules enforced, for example, a test in `firmware_ec_rw` must also be in
`firmware_ec_ro` and `firmware_ec`.
