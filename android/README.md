# Android projects used by Tast tests

This directory contains Android projects that are used by Tast tests. They are
built at the same time the ChromeOS test image is built, and not updated by
tast run command automatically.

## Using an APK under the directory in Tast

Take ArcGamepadTest.apk for example.

```go
p := s.PreValue().(arc.PreData)
a := p.ARC

if err := a.Install(ctx, arc.APKPath("ArcGamepadTest.apk")); err != nil {
  s.Fatal("Failed to install the APK: ", err)
}
```

## Updating APKs on the device

```bash
# one time setup
setup_board --board=${BOARD}

cros_workon --board=${BOARD} start tast-local-apks-cros
emerge-${BOARD} tast-local-apks-cros
cros deploy --root=/usr/local ${DUT_IP} tast-local-apks-cros
```

Then you can run normal tast run command.

## Finding the built APKs
In the chroot they can be found under
`/build/$BOARD/usr/libexec/tast/apks/local/cros`

On the DUT they can be found under
`/usr/local/libexec/tast/apks/local/cros/`

# Code Styles

Please follow
[AOSP Java style](https://source.android.com/docs/setup/contribute/code-style)
unless there's a specific reason.

Currently we have a limitation that we cannot use any external library for
building apks (b/217501318), including androidx support library, all code
should be written using standard android SDK only.
If androidx or some other dependency is needed, it’s still possible to put your
source code under ARC++ internal repository and use the prebuilt apk as
external data
([example](https://crsrc.org/o/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/arc/data/ArcCompanionLibDemo.apk.external)).
From maintenance perspective, this is not recommended.

# About OWNERS

Tast reviewers (tast-owners@google.com) are experts on Go language and Tast
tests, but not Java and Android. Thus, we have additional OWNERS for this
directory to review Android and Java code.
