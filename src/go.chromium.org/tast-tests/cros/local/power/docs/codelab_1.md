# Tast Power Tests Codelab #1: power.ExampleUI

This codelab outlines the structure of a simple local Tast test
[power.ExampleUI] to set up a device for power measurement and collect power
metrics while the device is idling. By following this codelab, we can quickly
get started with evaluating the power impact of a particular use case or a new
feature.

[power.ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go

[TOC]

## Set up the device to run a power test

For device power qualification and local testing, the recommendation is to run
power tests without AC, ethernet and peripheral connection, to eliminate
unnecessary power consumption. For regression monitoring and lab testing, they
can be run with AC and/or ethernet connection, but be sure to take into account
their impact when calculating battery life estimations.

### Power fixtures (Recommended)

The following fixtures are recommended:
- powerNoUINoWiFi (can only run with ethernet / in lab)
- powerNoUIWiFi
- powerAsh*
- powerLacros*

Refer to [power/setup/fixture.go] to see their recommended scenarios. Fixtures
set up the devices for testing, including stopping services that might have
power impact, adjusting backlights, etc.

For examples on how to use the power fixtures, please refer to test [ExampleUI]
and [ExampleNoUIManualMetrics]. Set the fixture in the test metadata.

```go
		Params: []testing.Param{{
			Name:      "ash",
			Fixture:   setup.PowerAsh,
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			Val:       exampleUI5minTimeParams,
			Timeout:   6*time.Minute + power.RecorderTimeout,
		}}
```

If your test already uses fixtures and is unable to inherit from a power
fixture, take a look at how to use [PowerTestSetup] directly.

[power/setup/fixture.go]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/fixture.go
[ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go
[ExampleNoUIManualMetrics]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_no_ui_manual_metrics.go
[PowerTestSetup]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/setup/setup.go?q=PowerTestSetup

### Test specific setup

Additional setup procedures can be done in the test main body. The main test
body should reserve some time for cleaning up at the end.

```go
func ExampleUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
```

`powerAsh*` and `powerLacros*` fixtures provide the browser type as `Bt`, and
the Chrome instance as `Cr`.

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
	defer conn.CloseTarget(cleanupCtx)

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

## Cool down the test device and start collecting power metrics

Cooling down the device is necessary, as the setup process can result in higher
power consumption than the average power consumption of the actual workload.
Without cooling down, the test results might be skewed, and thus the test loses
its purpose.

Depending on the device spec, test specific setup and workload, the cool down
process could be adjusted by adjusting threshold and time.

`power.TimeParams` determines measurement intervals and total test duration.

```go
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total
```

- `power.TimeParams.Interval`: `time.Duration` type, and describes the interval
between two metric snapshots.
- `power.TimeParams.Total`: `time.Duration` type, and describes the duration of
the main test body.

Pick a measurement interval that works well with the total test duration. See
recommendation:

| Total test duration | Measurement interval |
|---------------------|----------------------|
| <5  min             | 5  sec               |
| <20 min             | 10 sec               |
| Otherwise           | 20 sec               |

High measurement frequency (with interval shorter than 5 sec) is not recommended
for the following reasons:

- Additional power consumption from sampling overhead
  http://b/288496804#comment10.
- Reading error http://b/276789695#comment35.

```go
	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
```

## Run the test workload

In [ExampleUI], the workload is to idle and maintain it for a while for accurate
measurement. For other tests, replace this with the workload which the test is
collecting power metrics on.

If the workload is bursty, `sleep` for a while to allow time to measure power.
Workload such as video / audio does not need to `sleep`, as the video frame /
audio sampling already implies `sleep`.

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

[ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go

## Finish power metrics collection

Stop collecting power metrics, post-process data, upload to dashboards and
create data visualizations.

```go
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
```

For instructions on how to view the test results, see [README].

[README]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/power/README.md#results

## See the full test

[power.ExampleUI] collects power metrics while the device is idling on a blank
page.

[power.ExampleUI]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/example_ui.go
