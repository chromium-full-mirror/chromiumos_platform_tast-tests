# Ensure battery within specified range

## Purpose

`PrepareBattery` function below is particularly useful when your work involves
the battery percentage. Battery percentage does not change linearly. When the
battery starts discharging from full, the battery percentage can remain at 100%
for a long period of time before it starts decreasing rapidly. Therefore you
cannot measure the power usage accurately when starting from full. In this
situation, you can discharge the battery to a range of 95% to 97% using
`PrepareBattery` function before you start measuring the power usage.

## Explanation

The [setup] package provides [PrepareBattery] function to help you charge or drain the DUT's
battery to a specified range. The function has a signature as shown below.

```go
PrepareBattery(ctx context.Context, minPercentage, maxPercentage float64, dischargeOnCompletion bool)
```

The function charges or discharges the battery to a range between
`minPercentage` and `maxPercentage`. Once the range is reached, it sets the
charging state to charge or discharge base on the argument
`dischargeOnCompletion`.

Note that the `battery_percent` metric is used to check if the battery is within
range, which is slightly different from the `battery_display_percentage`.
Therefore, the battery charge shown on the UI may indicate the DUT has not reach
the specified range but the function still finishes successfully. This is
expected and not a concern.

The expected time to drain the battery from 100% to 97% takes 20 to 30 minutes,
while draining from 100% to 80% takes about 1 hour and 20 minutes. Therefore,
this function may not be suitable for lab testing as it adds a significant
amount of precious lab time.

[setup]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/
[PrepareBattery]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/setup_battery.go?q=PrepareBattery