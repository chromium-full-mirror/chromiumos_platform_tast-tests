// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	// These packages register their tests via init functions.
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/apps"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/arc"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/audio"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/autoupdate"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/bisector"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/camera"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/cellular"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/certprovisioning"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/crash"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/croshealthd/fixture"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/example"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/factory"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/featured"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/feedback"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/filemanager"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/floatingworkspace"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/hardware"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/hotspot"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/hps"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/inputs"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/intel"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/kernel"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/kunit"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/labqual"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/lacros"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/meet"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/meta"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/nearbyshare"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/network"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/network/allowlist"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/networkui"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/osinstall"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/platform"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/policy"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/power"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/remotecommands"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/rollback"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/secagentd"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/security"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/shimlessrma"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/statefulmigration"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/storage"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/syzcorpus"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/syzkaller"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/typec"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/ui"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/usb"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/uwb"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/vm"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb"

	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/factory/fixture"
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/oobe/fixture"
	_ "go.chromium.org/tast-tests/cros/remote/camera"           // import fixture for camera tests
	_ "go.chromium.org/tast-tests/cros/remote/camera/camerabox" // import fixture for camerabox tests
	_ "go.chromium.org/tast-tests/cros/remote/camera/cca"       // import fixture for cca tests
	_ "go.chromium.org/tast-tests/cros/remote/cellular"         // import fixture for local cellular tests
	_ "go.chromium.org/tast-tests/cros/remote/cryptohome"       // import fixture for cryptohome tests
	_ "go.chromium.org/tast-tests/cros/remote/demomode"         // import fixture for local-only demo mode tests
	_ "go.chromium.org/tast-tests/cros/remote/graphics"         // import fixture for graphics tests
	_ "go.chromium.org/tast-tests/cros/remote/hooks"            // import fixture for Tast root fixture
	_ "go.chromium.org/tast-tests/cros/remote/meta"             // import fixture for meta tests
	_ "go.chromium.org/tast-tests/cros/remote/tape"
	_ "go.chromium.org/tast-tests/cros/remote/uwb" // import fixture for local uwb tests
)
