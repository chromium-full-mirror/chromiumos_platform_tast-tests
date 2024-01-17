# Checkpoints

## Purpose

Sometimes a power test might run multiple smaller tasks in sequence. In addition
to the overall power statistics, we also would like to view statistics for each
individual task. Checkpoints are introduced to provide this information. They
help with:

- Understanding the power consumption of each individual task.
- Debugging individual tasks and locating any bugs.
- Easier automated bisection based on the metrics from a part of a test.

## Example tests

The local [`Browsing`] test is an example of using checkpoints in a test. It
measures power metrics for each loop of the test and each website that it opens.

[`Browsing`]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/browsing.go

## Example usage

Checkpoints are only enabled in local power tests now, as a part of power
[`Recorder`].

For example, if a power test runs several loops of the same workload, and the
workload contains Google website, checkpoints can be constructed in the
following way. Checkpoints can overlap with each other.

```go
r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
defer r.Close(cleanupCtx)
if err := r.Cooldown(ctx); err != nil {
  s.Error("Cooldown failed: ", err)
}
if err := r.Start(ctx); err != nil {
  s.Fatal("Cannot start collecting power metrics: ", err)
}

for loop := 0; loop < loopCount; loop++ {
  loopName := fmt.Sprintf("loop%02d", loop)
  loopCheckpoint := r.StartCheckpoint(loopName)

  # Navigate to Google website.

  siteCheckpoint := r.StartCheckpoint("google")

  # Idle at Google website.

  r.EndCheckpoint(siteCheckpoint)

  # Navigate to other websites...

  r.EndCheckpoint(loopCheckpoint)
}

if err := r.Finish(ctx, configValues); err != nil {
  s.Error("Cannot finish collecting power metrics: ", err)
}
```

[`Recorder`]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/recorder.go?q=StartCheckpoint


## Behind the scenes

Checkpoints data are processed and output alongside power metrics, and show
up in the following files:

### `checkpoint_log.json`

A list of checkpoint timestamps.

Example file format:
```
{
    "google": [
        [
            1705406426.61482,
            1705406486.390573
        ]
    ],
    "loop00": [
        [
            1705406426.390353, // start Unix timestamp for checkpoint "loop00"
            1705406546.390887  // end Unix timestamp for checkpoint "loop00"
        ]
    ]
}
```

### `power_log.json`

Each power metric data point is tagged with the corresponding checkpoints.

Example file format:
```
"checkpoint": [
  [
    "loop00", // checkpoints for the 1st data point
    "google"
  ],
  [
    "loop00", // checkpoints for the 2nd data point
    "google"
  ],
  [
    "loop00"  // checkpoints for the 3rd data point
  ]
]
```

### `results-chart.json`

Power metrics are filtered by checkpoints.

Example file format:
```
"system": {
  "summary": {
    "units": "W",
    "improvement_direction": "down",
    "type": "list_of_scalar_values",
    "interval": "t",
    "values": [
      6.101564,
      6.349024,
      6.582808,
    ]
  }
},
"system.google": {
  "summary": {
    "units": "W",
    "improvement_direction": "down",
    "type": "list_of_scalar_values",
    "interval": "t.google",
    "values": [
      6.101564,
      6.349024
    ]
  }
},
"system.loop00": {
  "summary": {
    "units": "W",
    "improvement_direction": "down",
    "type": "list_of_scalar_values",
    "interval": "t.loop00",
    "values": [
      6.101564,
      6.349024,
      6.582808,
    ]
  }
}
```

### `debug_summary.csv`

Statistics for the data in results-chart.json.

Example file format:
```
name,mean,stddev,min,max,# data
system.google,6.225294,0.123730,6.101564,6.349024,2
```

## View the results

### Visually

When viewing detailed measurements over one test run on the [power dashboard],
data points will be tagged with the names of the tasks that they are collected
from.

> Note that this only shows up on the [power dashboard], not in
`power_log.html`.

[power dashboard]: http://go/power-dashboard-view

### Crosbolt dashboard and bisection

Statistics for each metric and for each checkpoint will be added to
results-chart.json and the [Crosbolt dashboard]. They can then be used for
[ChromeOS Bisector].

[Crosbolt dashboard]: http://go/crosbolt
[ChromeOS Bisector]: http://go/crosbisect
