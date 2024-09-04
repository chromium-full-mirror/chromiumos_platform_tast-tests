# mitmproxy in Tast

This directory contains mitmproxy implementations.

Usage (for Chrome in-session):
```
var opts []proxy.Option
mp, err := proxy.NewMitmProxy(ctx, opts...)
defer mp.Close(cleanupCtx)

reset, err := proxy.ConfigureChrome(ctx, mp, cr)
defer reset(cleanupCtx, cr)
```

Usage (for ARC++ in-session):

```
// The same as above to use a proxy in-session
mp, err := proxy.NewMitmProxy(ctx,
    proxy.CustomCA(true),
)
defer mp.Close(cleanupCtx)

reset, err := proxy.ConfigureChrome(ctx, mp, cr)
defer reset(cleanupCtx, cr)

// Insert the CA cert to ARC++ system.
a, err := arc.New(ctx, ...)
path, _, err := mp.RootCertificate(ctx)
a.AddCaCert(ctx, path, proxy.CaHashcodeVar.Value())
a.ResumeProvisioning(ctx)
```
See [this test](https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/src/go.chromium.org/tast-tests/cros/local/bundles/cros/testenv/verify_proxy_arc.go) as an example.

Usage (for signin flow):
```
mp, err := proxy.NewMitmProxy(ctx,
    proxy.CustomCA(true),
)
defer mp.Close(cleanupCtx)

crOpts := []chrome.Option{
    chrome.ProxyServer(mp.ProxyAddress()),
}
chrome.New(ctx, crOpts...)
```
