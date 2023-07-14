# How to verify network traffic

`netexport` allows tast tests to verify the network calls made by the ChromeOS
device during a test run. For example, a policy test can verify that a specific
[Chrome Enterprise Policy](https://chromeenterprise.google/policies) can be used
to disable specific network calls. Network calls are identified based on their
[Network Traffic Annotation](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/docs/network_traffic_annotations.md).

There are two key parts to implementing this in a tast test:
1. Setup network logging
2. Verify network logs

## Setup: Code Examples
*Option 1: Browser (recommended)*
```go
// Start network logging using chrome://net-export and os://net-export.
netExport := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
if err != nil {
  s.Fatal("Failed to start net export: ", err)
}
defer netExport.Cleanup(cleanupCtx)
```

*Option 2: Enable netlog on startup*

Starting a net export session with `Start()` is done via the browser. This means
that all network traffic prior to this call will not be logged. To log network
traffic that happens on startup/login, Chrome needs to be started with network
logging enabled.

> **_NOTE:_** This option is NOT recommended for most policy tests because we
prefer to test all policy values in a single Chrome session, whenever possible.
The only way to reset the network log using this method is to restart Chrome.

```go
// Create Chrome opts with network logging enabled on startup.
opts = append(opts, netexport.CommandLineArgs(browser.TypeAsh)...)

// Start Chrome.
cr, err := chrome.New(ctx, opts...)
if err != nil {
  s.Fatal("Failed to start Chrome: ", err)
}
defer cr.Close(ctx)

// Get netexport session. This is used for log verification later.
netExport, err := netexport.FromCommandLineArg(browser.TypeAsh)
```

## Verification: Code Examples
*Single annotation*
```go
foundAnnotation, err := netExport.Find(useravatar.AnnotationHashCode)
if err != nil {
  s.Fatal("Failed to search net log file for annotation: ", err)
}
if foundAnnotation != param.ShouldFindAnnotation {
  s.Fatalf("Annotation mismatch = got %t, want: %t", foundAnnotation, param.ShouldFindAnnotation)
}
```

*Multiple annotations*
```go
foundAnnotations, err := netExport.FindAll()
if err != nil {
  s.Fatal("Unexpected error when verifying logs: ", err)
}
for _, annotationID := range extensioninstall.GetAnnotations() {
  if _, exists := foundAnnotations[annotationID]; exists != tc.ShouldFindAnnotation {
    s.Errorf("Unexpected status of annotation = %s, got %t, want %t", annotationID, exists, tc.ShouldFindAnnotation)
  }
}
```

> **_NOTE:_** Sometimes its better to use polling for checking annotations. This
is mainly useful for network traffic that occurs in the background without UI
notification.

*Single annotation w/ polling*
```go
foundAnnotation, err := netExport.FindUntil(ctx, passwordleakdetection.AnnotationHashCode,
  &testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second})
if err != nil {
  s.Fatal("Failed to check network logs: ", err)
}

if foundAnnotation != param.ShouldFindAnnotation {
  s.Fatalf("Annotation mismatch = got %t, want: %t", foundAnnotation, param.ShouldFindAnnotation)
}
```

*Multiple annotations w/ polling*
```go
foundAnnotations, err := netExport.FindMultipleAnnotationsUntil(ctx, hashCodes,
  &testing.PollOptions{Timeout: 80 * time.Second, Interval: 10 * time.Second})
if err != nil {
  s.Fatal("Failed to poll hashcode in log: ", err)
}

for _, annotationID := range hashCodes {
  if _, exists := foundAnnotations[annotationID]; exists {
    s.Error("Found unexpected annotation = ", foundAnnotations)
  }
}
```
