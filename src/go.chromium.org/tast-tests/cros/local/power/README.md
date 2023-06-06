# Local Tast Power Tests: the Beginner's Guide

> This document assumes that you are familiar with [general Tast documentation].

This instruction outlines the structure of a simple local Tast test to set up a
device for power measurement and collect power metrics while running a workload.
By following this instruction, we can quickly get started with evaluating the
power impact of a particular use case or a new feature.

[TOC]

[general Tast documentation]: https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/

## Set up the device for running a power test

For device power qualification and local testing, the recommendation is to run
power tests without AC and ethernet connection, to eliminate unnecessary power
consumption. For regression monitoring and lab testing, they can be run with AC
and/or ethernet connection, but be sure to take into account their impact when
calculating battery life estimations.

### Standard setup: power fixtures

The following fixtures are recommended:
- powerNoUINoWiFi (can only run with ethernet / in lab)
- powerNoUIWiFi
- powerAshKbbl
- powerAsh
- powerLacrosKbbl
- powerLacros

Refer to [power/setup/fixture.go] to see their recommended scenarios. Fixtures
set up the devices for testing, including stopping services that might have
power impact, adjusting backlights, etc.

For examples on how to use the power fixtures, please refer to test [ExampleUI]
and [ExampleNoUIManualMetrics].

If your test already uses fixtures and is unable to inherit from a power
fixture, take a look at how to use `PowerTestSetup` directly.
TODO: b/283738206 - add a link to `PowerTestSetup`.

[power/setup/fixture.go]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/fixture.go
[ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go
[ExampleNoUIManualMetrics]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_no_ui_manual_metrics.go

### Additional functionality / test specific setup

Additional setup procedures can be done in the test main body, using [ExampleUI]
test as an example.

The main test body should reserve some time for cleaning up at the end.
```go
func ExampleUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
```
`powerAsh*` and `powerLacros*` fixtures provide the browser type as `bt`, and
the Chrome instance as `cr`.
```go
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
```
The test then performs any test specific setup. In the case of [ExampleUI] test,
it opens the browser with a blank page and maximizes the browser window.
```go
	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}
```

[ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go

## Start power metrics collection

Cool down the test device and start collecting power metrics.

* `sampleInterval` provided as a `time.Duration` type, describes the interval
between two metric snapshots. Consider dividing your total test time into an
appropriate measurement interval. For example, if your total test is length
`300 * time.Second`, the `sampleInterval` could be `5 * time.Second`.

Note: Depending on the test specific setup and workload, cool down process might
 need to be adjusted.

Cooling down the device is necessary, as the setup process can result in higher
power consumption than the average power consumption of the actual workload.

Without cooling down, the test results can be skewed, and thus loses its purpose
 to measure accurate power consumption and estimate battery life.
```go
	r := power.NewRecorder(ctx, sampleInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
```

## Run the test workload

Replace with the workload which the test is collecting power metrics on.
```go
	// Start of main test body. Idle for `total` seconds while reading power
	// metrics every `interval` seconds. Device setup is handled in fixture.
	// This test both serves as an example for future power tests and as a light
	// weight test to test the device setup. Replace this chunk of code with
	// functionality code for future power tests.
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.
```

## Finish power metrics collection

Stop collecting power metrics, post-process data, upload to dashboards and
create data visualizations.
```go
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
```
### Adding custom perf metrics
If you have custom `perf` values to report, you can call `Finish` and provide one or more custom values to the recorder.

All collected power metrics are located at `tests/<TEST NAME>/power_log.json`.

```go
	if err := r.Finish(ctx, p1, p2, etc. ); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
```


## Look at the collected power metrics
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

To monitor regression through crosbolt, see go/power-test-regression.
