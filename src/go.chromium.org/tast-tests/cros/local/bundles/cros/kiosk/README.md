# Kiosk Tast tests

This document includes some common things test authors should pay attention to
when writing Tast tests for Kiosk.

## Kiosk test strategy

The Chrome OS Kiosk team has guidelines for the Tast tests we own. Please refer
to http://go/kiosk-test-strategy before writing new tests if you intend to add
the Kiosk team as a contact in the tests.

For questions, reach out to chromeos-kiosk-eng@google.com.

## Check `smoke.go` for best practices

`smoke.go` is the smoke test suite of Kiosk, we rely on this test to verify the
basis of many of our Kiosk features. This is the test we refer other teams to
run when verifying their changes work in Kiosk.

For this to work reliably it's important that the smoke tests stay green.

When changing Kiosk related code, please remember to at least verify the smoke
tests work with `tast run <DUT> "kiosk.Smoke.*"`. This includes changes to the
`kioskmode` package here in Tast code.

## Handle `kiosk.Close()` properly

Calls to `kioskmode.New()` should be paired with a call to `kiosk.Close()` to
clean up any resources taken up by the Kiosk session. Most importantly, this
resets Kiosk policies before the next test starts.

Lingering Kiosk policies can be especially problematic because of auto launch.

`kiosk.Close()` returns an error that must be checked, often in a `defer`
statement. An error in `kiosk.Close()` implies the library could not properly
clean up after the test and Kiosk policies may have remained. Tests should fail
if `kiosk.Close()` fails.

## Reserve context time for cleanup

Kiosk tests need a long setup and cleanup time. Due to this longer cleanup,
tests should take extra care and reserve time for `kioskmode.PolicyUpdateTime`.
This can be done with some boilerplate, as in:

```go
func MyKioskTest(ctx context.Context, s *testing.State) {
	// Keep the original ctx in cleanupCtx.
	cleanupCtx := ctx
	// Shorten ctx to reserve this time for cleanupCtx.
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.PolicyUpdateTime)
	defer cancel()

	// Start Kiosk and check for errors.
	kiosk, cr, err := kioskmode.New( ... )
	if err != nil { ... }
	// Defer the kiosk.Close() call using cleanupCtx.
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			// Fail the test if kiosk.Close() fails.
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	// Use ctx on your test as usual.
	...
}
```

## Test timeout

Adjust your test timeout based on the discussions above. Make sure it has enough
time for your test to setup Kiosk with its policies, and to later clear them up,
as many times as your test may need to do it.

You can base your test timeout on the constants `kioskmode.SetupDuration`,
`kioskmode.LaunchDuration`, and `kioskmode.CleanupDuration`, as in:

```go
func init() {
	testing.AddTest(&testing.Test{
		Func: MyKioskTest,
		...
		// Calls kioskmode.New once, waits for kiosk launch, calls kiosk.Close once,
		// plus time for this test.
		Timeout: kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + 1*time.Minute,
	})
}
```
