# Tast Remote Power Tests Codelab: meta.PowerServodWrapper

## Introduction
This codelab runs through the usage of [meta.PowerServodWrapper] in Tast, which
allows the measurement of intrusive power metrics from sense-resistors or
onboard PAC chips via. an attached Servo device.

PowerServodWrapper can start an existing local or remote test (i.e. the subtest)
within the wrapper test as a goroutine. It captures power metrics while the
subtest runs to completion.

This test is suitable for measuring a feature manually.

Results are saved to your tast result directory after the test run, and are also
 published to [go/power-dashboard-view].

[meta.PowerServodWrapper]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/power_servod_wrapper.go

### Pre-requisites
1. DUT with power-measuring hardware, that can communicate via. `servod`.
Each hardware can measure:
    - CPD*: the battery sense resistor and PAC chip on the CPD
    - Sweetberry*: sense resistors on the DUT, INA chips on the Sweetberry
    - PACDebugger, Servo: sense resistors and PAC/INA chips that are on the DUT

    > Note: \* requires hardware reworking

    > Note: INA chips (without built-in accumulators) should set the test param
    `useAccum=false`. The wrapper will query from `power_rails` instead of
    `avg_power_rails`.

2. A running instance of `servod`, with its host and port provided.

3. The workstation (or where the Tast test is run from) should be in same
timezone with the DUT.

If you are looking to get a CPD/Sweetberry reworked onto a device, or more info about
your hardware setup, e.g. `servod` config, please reach out to your local hardware
team for advice.

## Setup
```
                               Sense resistors
                                 |
                                 |
        Host---------Servo*-----DUT
 (running servod)      |         |
                       |         |
                       +-----PAC chips*
```
> \*PAC chips may be on the DUT or the Servo. Some devices like Sweetberry may
not need to connect to additional Servo device to communicate via. `servod`.
Please refer to your power measuring hardware guide for specific setup.


### Start `servod`
Start a servod instance to talk to the Servo or Sweetberry that is attached
to the DUT.

- The default host:port is `localhost:9999`.

To specify a different configuration when running the Tast test, add the
following variable:

```
tast run -var "servo=<servo-host>:<servo-port> ..."
```
For more configuration options, see [go/tast-running#Running-tests-with-Servo]

[go/tast-running#Running-tests-with-Servo]: go/tast-running#Running-tests-with-Servo

## Running the Test

### Setting the servo measurement interval

- The default measurement interval is **4 seconds**.

If your subtest (the test that you are wrapping) is using a Power recorder, it
is recommended to use the same interval or shorter than the power recorder
interval.

Note: **Do not set the interval too short**.
Depending on the number of power rails you are querying, there may be a delay
in communication between Tast and servod.
If the time between measurements is too short the test will fail.
You should experiment with finding an interval that works for your case.

To explicitly set the measurement interval, use the following variable:

```
tast run -var "meta.PowerServodWrapper.interval=<seconds> ..."
```


### Manual Tests
If you are running with a CPD device, the CPD rails come with
a `ft4232h_generic.` prefix that is handled specially.
Run your test with the following scenario:

```
tast run -var "subtest=<subtest>" $DUT_IP meta.PowerServodWrapper.cpd_manual
```

Otherwise, use the generic scenario:

```
tast run -var "subtest=<subtest>" $DUT_IP meta.PowerServodWrapper.manual
```
An example power test you can use is `subtest=power.ExampleUI.ash` .


## View results
Results from the wrapper test are trimmed to match the duration of the inner test.
- E.g. `PowerServodWrapper` starts measuring from timestamp 0.
If the inner test completes from timestamp 2 to timestamp 5 seconds,
`PowerServodWrapper` data points are trimmed to report measurements at/between
timestamp 2 to timestamp 5 seconds.

The wrapper test results are saved separately from the subtest.
In your results directory, you should find:
- (PowerServodWrapper) `power_log.html/json`
- (PowerServodWrapper) `results-chart.json`
- `subtests_results/` directory, with the usual results of the inner subtest.

You can also view the results of `meta.PowerServodWrapper` on [go/power-dashboard-view].
- `PowerServodWrapper` results are published **to the name of your
inner subtest** , at a separate timestamp.
- Use caution if you stack the results of the wrapper and subtest, as the start
timestamps may not be the same.

[go/power-dashboard-view]: go/power-dashboard-view
