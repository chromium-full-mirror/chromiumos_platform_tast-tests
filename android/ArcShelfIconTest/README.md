// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

# ARCShelfIconTest
ARCShelfIconTest is an app used in Tast Shelf Icon tests.

# How to modify ARCShelfIconTest with Android Studio
1. Launch Android Studio, and click `File > New > Import Project`
2. Locate `chromiumos/src/platform/tast-tests/android/ArcShelfIconTest`, click the `build.gradle` to select it, and then click `OK` to import your project.
3. Click `Build > Make Project`.

# How to build ARCShelfIconTest.apk
## For development
1. Connect your DUT with ADB (ex: `$ adb connect ${DUT_IP}:22` )
2. Select `Run ARCShelfIconTest` from the `Run` menu.
3. `ARCShelfIconTest` should build and then appear on your DUT.

# How to test ARCShelfIconTest.apk with tast

1. Rebuild APK
```
cros_workon --board=${BOARD} start tast-local-apks-cros
emerge-${BOARD} tast-local-apks-cros
cros deploy --root=/usr/local ${DUT_IP} tast-local-apks-cros
```

2. Run your tast testcase in chroot by:
```
$ tast -verbose run ${DUT_IP} arc.ShelfIcons
```

# Reference Doc
[Android projects used by Tast tests]


[Android projects used by Tast tests]: https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/HEAD/android/README.md
