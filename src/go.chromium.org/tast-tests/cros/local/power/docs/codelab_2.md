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

TODO: Update `PrepareBattery` function signature and description.

The [setup] package provides [PrepareBattery] function to help you charge or
drain the DUT's battery to a specified range. The function has a signature as
shown below.

```go
PrepareBattery(ctx context.Context, minPercentage, maxPercentage float64,
	dischargeOnCompletion bool)
```

The function charges or drains the battery to a range between
`minPercentage` and `maxPercentage`. Once the range is reached, it sets the
charging state to charge or discharge base on the argument
`dischargeOnCompletion`.

Note that the `battery_percent` metric is used to check if the battery is within
range, which is slightly different from the `battery_display_percentage`.
Therefore, the battery charge shown on the UI may indicate the DUT has not reached
the specified range but the function still finishes successfully. This is
expected and not a concern.

## Example

TODO: Update `PrepareBattery` function signature, explanation and usage.

This example is based on the [ExampleNoUIDischarge] test.

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

In the test body, you can use the `PrepareBattery` function to
charge or drain the battery to the expected range. Usually we want to have 1 to
2 percent of uncertainty when preparing the battery due to the accuracy of reading
battery percentage. Therefore, if we want the battery to be around 75%, we want
to set the minimum and maximum battery charge to be 74% and 76%.

```go
	if err := setup.PrepareBattery(ctx, 74.0, 76.0, true, false); err != nil {
		s.Fatal("Failed to prepare battery: ", err)
	}
```

Note that the battery is already discharging before the `PrepareBattery` call
because of the `PowerNoUINoWiFi` fixture configures the DUT to discharge during
test. Thus, we want to make sure the battery continues to discharge after we
prepared the battery.

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

TODO: Add a second charging test.

[setup]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/
[PrepareBattery]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/setup_battery.go?q=PrepareBattery
[This example is based on the [ExampleNoUIDischarge] test.
]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_no_ui_discarge.go