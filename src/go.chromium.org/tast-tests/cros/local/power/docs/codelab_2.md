# Tast Power Tests Codelab #2: Charge or Drain the Battery Before Test

## Introduction

The power tast library provides functions to charge or drain a DUT's battery
to a specific range. For example, you can use it to charge or drain the battery
to a range of 75% to 80% before starting a test.

## Why do we need this?

The primary reason that we need to charge or drain the battery before some tests
is because the battery percentage does not change linearly. When the battery
starts discharging from full, the battery percentage can remain at 100% for a
long period of time before it starts decreasing rapidly. As a result, if you
start your test when the battery is fully charged, you may not be able to
accurately measure the battery usage by computing the difference in battery
percentage before and after the test.

## When should you use this?

You should consider charging or draining the battery if you are interested in
how much battery was used during a test, or you simply need it to test the
charging and discharging mechanism itself. However, you should only use it
during manual testing because it may take up to 1.5 hours to drain the battery
from 100% to 80%. This process is too lengthy for lab testing as the lab resource
is limited.

## How to use PrepareBattery for Test?

The [setup] package provides [PrepareBattery] function to help you charge or
drain the DUT's battery to a specified range. The function has a signature as
shown below, where [ChargeParams] defines parameters used for a charge test:

```go
PrepareBattery(ctx context.Context, cp power.ChargeParams)
```

The function charges or drains the battery to a range between `cp.MinChargePercentage`
and `cp.MaxChargePercentage`. Once the range is reached, it sets the charging
state to charge or discharge base on the argument `cp.DischargeOnCompletion`.

A `chargeParams`` currently has the following fields to control a charge test
behavior:
- `MinChargePercentage` and `MaxChargePercentage`: two floats that indicate the
battery lower and upper bound.
- `DischargeOnCompletion`: if set to true then AC power will be forced to be
temporarily disconnected to prevent battery from being charged after the test.
This is done via `ectool`, not through a physical disconnection.
- `IsCustomized`: if set to true, then a customized charge range should be
provided by the test run. How to enforce the customization behavior depends on
your use case. In [ChargeDischargeBattery], we enforce the customization
behavior by throwing an error if either min or max is not provided. To run the test,
we do `tast run -var="min_charge_percent=50" -var="max_charge_percent=65" <DUT>`
`power.ChargeDischargeBattery.customization_prep`.
- `IsPowerQual`: if set to true, it indicates the charge test is part of the power qual
tests, which is used to measure the charging speed of a device. In this case, the screen
will be set to default screen brightness instead of 0 for speed charging.

Note that the `battery_percent` metric is used to check if the battery is within
range, which is slightly different from the `battery_display_percentage`.
Therefore, the battery charge shown on the UI may indicate the DUT has not reached
the specified range but the function still finishes successfully. This is
expected and not a concern.

## Examples

### [ExampleNoUIDischarge]

Assuming we want the battery to be around 75% charge and the DUT is fully
charged to begin with, we can usually expect about 60 minutes to drain the DUT
to 75% charge. Thus, we need to set the test time accordingly when we add the
test. You should choose a sensible timeout when you are working on your own test.

```go
func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleNoUIDischarge,
        ...
		Fixture: setup.PowerNoUINoWiFi,
		Timeout: 60 * time.Minute,
	})
}
```
Usually we want to have 1 to 2 percent of margin when preparing the battery
due to the accuracy of reading battery percentage. Therefore, if we want the battery
to be around 75%, we want to set the minimum and maximum battery charge to be 74%
and 76%. As the battery is already discharging as a result of using the `PowerNoUINoWiFi`
fixture, we want to make sure the battery continues to discharge after we prepared
the battery. Thus we can define a charge params struct like this:

```go
var exampleChargeParam = power.ChargeParams{
	MinChargePercentage:   74.0,
	MaxChargePercentage:   76.0,
	DischargeOnCompletion: true,
	IsCustomized:          false,
	IsPowerQual:           false,
}
```
In the test body, you can use the `PrepareBattery` function to
charge or drain the battery to the expected range.

```go
	if err := setup.PrepareBattery(ctx, exampleChargeParam); err != nil {
		s.Fatal("Failed to prepare battery: ", err)
	}
```

When you want to prepare battery for **measuring the power usage** of your
feature, you should consider whether to place the `PrepareBattery` function
before or after you set up the DUT in the test body. A genenral rule is that,
if setting up the DUT consumes a consistent amount of power (i.e. uses roughly
the same amount of power every time), then it might be a good idea to prepare
the battery first and then set up the DUT. This is because `PrepareBattery`
stresses the CPU to drain the battery quickly, and if your feature is set up and
running before preparing the battery, the high CPU usage may affect or even
break your feature. As long as the setup process consumes a consistent amount of
power, the starting state of your power recording should remain consistent.

Once the battery charge is within the specified range, we can start recording
the power metrics.

### [BatteryChargeQual]

For battery charge qual test, the purpose is to measure how long it takes to charge
the battery to full from a very low percentage. `DischargeOnCompletion` can be set to
either false or true and this shouldn't affect subsequent tests setup as batteries will
be charged/drained again for subsequent tests as needed. So we define the charge
param qualChargeParam like below:

```go
var qualChargeParam = power.ChargeParams{
	MinChargePercentage:   99.0,
	MaxChargePercentage:   100.0,
	DischargeOnCompletion: false,
	IsCustomized:          false,
	IsPowerQual:           true,
}
```
In the test body, we simply call the [PrepareBattery] function to charge the battery
to full.

```go
if err := setup.PrepareBattery(ctx, qualChargeParam); err != nil {
s.Fatal("Failed to charge DUT: ", err)
}
```

[setup]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/
[PrepareBattery]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/setup_battery.go?q=PrepareBattery
[ChargeParams]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/params.go;l=17
[ChargeDischargeBattery]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/charge_discharge_battery.go
[ExampleNoUIDischarge]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_no_ui_discharge.go
[BatteryChargeQual]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/battery_charge_qual.go