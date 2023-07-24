# Run a Manual Power Test

## Purpose
Before fully committing to writing a new power test, measure power while running
the feature manually, to get an estimate of the feature's power consumption.

This should give the feature team a means to measure power consumption in early
development, and help feature team decide whether and how much to invest in
power consumption measurement and improvement.

## Steps

You can run a manual UI test from [manual_ui.go].

Currently the test supports `ash/lacros` browsers, and `ARC/GAIA` variants.

Refer to the available test names in `ManualUI` for specifics.

```
$ tast run <DUT_IP> power.ManualUI.<testname>
```

Inside the Tast command, you can pass specific variables by leading with the
-var flag. To set the test duration (between 1-1440 minutes i.e. 24 hours
maximum), use the flag:

```
$ tast run -var "power.test_duration=<minutes>" <DUT_IP> ...
```

[manual_ui.go]: (https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/power/manual_ui.go)
