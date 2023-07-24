# README: Local Tast Power Tests

> This document assumes that you are familiar with [general Tast documentation].

[general Tast documentation]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/

[TOC]

## Introduction
Local Tast power test sets up a device for power measurement and collects power
metrics while running a workload.

## Ensure battery within specified range

The `setup` package provides a function to help you charge or drain the DUT's
battery to a specified range. The function has a signature as shown below.

```
PrepareBattery(ctx context.Context, minPercentage, maxPercentage float64, dischargeOnCompletion bool)
```

The function charges or discharges the battery to a range between `minPercentage`
and `maxPercentage`. Once the range is reached, it sets the charging state to charge
or discharge base on the argument `dischargeOnCompletion`.

Note that the `battery_percent` metric is used to check if the battery is within
range, which is slightly different from the `battery_display_percentage`. Therefore,
the battery charge shown on the UI may indicate the DUT has not reach the specified
range but the function still finishes successfully. This is expected and not a cause
for concern.

This function is particularly useful when your work involves the battery
percentage as it does not change linearly. When the battery starts discharging from
full, the battery percentage can remain at 100% for a long period of time
before it starts decreasing rapidly. Therefore you cannot measure the power usage
when starting from full. In this situation, you can discharge the battery to a range
of 95% to 97% using this function before you start measuring the power usage.

The expected time to drain the battery from 100% to 97% takes 20 to 30 minutes, while
draining from 100% to 80% takes about 1 hour and 20 minutes. Therefore, this
function may not be suitable for lab testing as it adds a significant amount of
precious lab time.

## Results
### Look at the collected power metrics
To understand the metrics that are collected, see [metrics.md].

[metrics.md]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/docs/metrics.md

### Local visualization
A local html data visualization of the collected power metrics is at
`tests/<TEST NAME>/power_log.html`.

### Power dashboard
The previously mentioned `tests/<TEST NAME>/power_log.html` contains a link to
the visualization of the test run in the power dashboard.

Otherwise, access directly at http://go/power-dashboard-view and filter the
criteria to view data visualization.

### Crosbolt dashboard
http://go/crosbolt and filter the criteria to view.

To monitor regression through crosbolt, see http://go/power-test-regression.

## Feedback

If you have any feature requests / bugs / feedback please feel free to share with us:
* [Power Buganizer List] (go/cros-power-buganizer)
* [ChromeOS Power Q&A Chat Room] (go/cros-power-q&a)

[ChromeOS Power Q&A Chat Room]: go/cros-power-q&a
[Power Buganizer List]: go/cros-power-buganizer