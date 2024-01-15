# Tast Remote Power Tests Codelab: Example Remote Power Test With UI

This is the location for tast remote power test codelabs. For local power tests,
see [Tast Local Power Tests].

[Tast Local Power Tests]: https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/power/docs/

In this codelab, an example remote power test with UI is used to demonstrate how
to interact with Chrome to start a WebGL rendering workload and measure the
power usage of a DUT remotely.

## Example Test With UI

See [ExampleRemoteUI] for the full snippet.

[ExampleRemoteUI]:https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/power/example_remote_ui.go

In the example test with UI, we will attempt to record the power usage of a
WebGL 3D rendering workload called Aquarium.

```go
// A WebGL rendering workload used for stressing GPU.
const aquariumURL = "https://webglsamples.org/aquarium/aquarium.html"
```

We need to add some service dependencies to the test. The `DeviceSetupService`
and `RecorderService` are remote power test services for measuring power. The
`ChromeService` is used to sign into ChromeOS remotely, whereas the
`ConnService` is used to interact with the Chrome browser to open and close
websites.

```go
ServiceDeps: []string{
	"tast.cros.power.DeviceSetupService",
	"tast.cros.power.RecorderService",
	"tast.cros.browser.ChromeService",
	"tast.cros.ui.ConnService",
}
```

As you may also expect, we will need Chrome so we must include it as part of the
software dependencies.

```go
SoftwareDeps: []string{"chrome"}
```

We begin the test by establishing a connection with the DUT first and defer
closing of the connection to the end of the test.

```go
// Connecting to the DUT.
cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
if err != nil {
	s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
}
defer cl.Close(ctx)
```

Let’s also create all the service clients early on so they don’t scatter
everywhere in the test.

```go
crs := ui.NewChromeServiceClient(cl.Conn)
cns := ui.NewConnServiceClient(cl.Conn)
ds := ps.NewDeviceSetupServiceClient(cl.Conn)
rs := ps.NewRecorderServiceClient(cl.Conn)
```

We need to login to the DUT using the Chrome Service’s `New` function so that we
can interact with the browser later. Here, we use the fake login mode (the
default login mode) but other modes are also available such as using GAIA
credentials. Remember to `Close` the chrome service at the end of the test to
release resources.

```go
// Fake login on the DUT through the chrome service.
if _, err := crs.New(ctx, &ui.NewRequest{}, grpc.WaitForReady(true)); err != nil {
	s.Fatal("Failed to login on the DUT: ", err)
}
defer crs.Close(ctx, &empty.Empty{})
```

We also need to close all tabs (including websites) at the end of the test as we
don’t want the workload to be running long after the test is finished.

```go
defer crs.ResetState(ctx, &empty.Empty{})
```

That should conclude all of the test setup for now. We can now proceed to set up
the power test environment with the `Setup` call. For this test, we want the UI
to remain unchanged and set the screen brightness to default. The `Cleanup` call
is deferred to the end of the test to ensure the DUT will return to its original
state.

```go
// Setting up a DUT remotely according to the setup request.
if _, err = ds.Setup(ctx, &ps.DeviceSetupRequest{
	Ui:               ps.UIMode_DO_NOT_CHANGE_UI,
	ScreenBrightness: ps.ScreenMode_DEFAULT_SCREEN_BRIGHTNESS,
}); err != nil {
	s.Fatal("Failed to setup DUT: ", err)
}
// Restoring the DUT to its original state once the test finishes.
defer ds.Cleanup(ctx, &empty.Empty{})
```

A recorder with a sampling interval of 5 seconds is created using the recorder’s
`Create` call and `Close` the recorder at the end of the test.

```go
// Create a recorder that takes 1 sample every 5 second.
if _, err = rs.Create(ctx, &ps.RecorderRequest{
	IntervalSec: 5,
}); err != nil {
	s.Fatal("Failed to create a recorder: ", err)
}
// Close the recorder once the test finishes.
defer rs.Close(ctx, &empty.Empty{})
```

Before starting the workload, we should cooldown the device to ensure there is
no power intensive workload running in the background.

```go
// Cooldown device.
if _, err = rs.Cooldown(ctx, &empty.Empty{}); err != nil {
	s.Fatal("Failed to cool down device: ", err)
}
```

Open the Aquarium workload in the browser. This must be done after the cooldown,
because the workload would keep the CPU utilisation high and cause cooldown to
fail.

```go
// Open a WebGL load in the browser.
connInfo, err := cns.NewConn(ctx, &ui.NewConnRequest{Url: aquariumURL})
if err != nil {
	s.Fatal("Failed to open page: ", err)
}
defer cns.Close(ctx, &ui.CloseRequest{Id: connInfo.Id})
```

Now that everything is set-up, we can start recording power metrics.

```go
// Start recording metrics.
if _, err = rs.Start(ctx, &empty.Empty{}); err != nil {
	s.Fatal("Failed to start recording metrics: ", err)
}
```

Let the DUT run the WebGL workload for 30 seconds.

```go
if err := testing.Sleep(ctx, 30*time.Second); err != nil {
	s.Fatal("Failed to sleep: ", err)
}
```

After sleeping, we can stop the recording by invoking the recorder’s `Stop` call
and obtain a copy of the recorded metrics. We also convert the `perf.Values
Proto` to `perf.Values` such that we can manipulate the data more easily.

```go
// Stop recording metrics.
rRes, err := rs.Stop(ctx, &empty.Empty{})
if err != nil {
	s.Fatal("Failed to stop recording metrics: ", err)
}
perfVals := perf.NewValuesFromProto(rRes.GetPerfMetrics())
```

As discussed in the previous section about data processing, we start by saving
and uploading the power log to the power dashboard. As a result, you should see
the recorded metrics on the power dashboard, and in the `power_log.json` and
`power_log.html` in the Host’s (typically this is your workstation) test output
directory.

```go
if _, err := cp.CreateSaveUploadPowerLog(
	ctx,
	s.OutDir(),
	s.TestName(),
	"",
	perfVals,
	nil,
	rRes.GetDeviceInfo(),
	rRes.GetOneTimeMetrics(),
); err != nil {
	s.Fatal("Failed to save and upload power log: ", err)
}
```

After that, you are free to modify or insert additional metrics into the
`perf.Values`. In this case, we insert a new metric called `custom_metric` where
its unit is `unit` and it has a value of 10.

```go
perfVals.Set(perf.Metric{
	Name: "custom_metric",
	Unit: "unit",
}, 10.0)
```

Finally, we save the recorded metrics with custom metrics (if any) to
`result-chart.json` in the Host’s test output directory, which would be picked
up by Crosbolt. Just keep in mind that the custom metric would not show up in
the power dashboard and `power_log` files but will show up in the Crosbolt
dashboard and `result-chart.json`.

```go
if err := perfVals.Save(s.OutDir()); err != nil {
	s.Fatal("Failed to save perf data for crosbolt: ", err)
}
```
