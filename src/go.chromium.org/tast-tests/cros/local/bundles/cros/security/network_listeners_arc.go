// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/netlisten"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkListenersARC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks TCP listeners on ARC systems",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"chrome"},
		Pre:          arc.Booted(),
		Timeout:      arc.BootTimeout,
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "container_r",
			ExtraSoftwareDeps: []string{"android_container_r"},
			// TODO(b/271846347): Move to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func NetworkListenersARC(ctx context.Context, s *testing.State) {
	ls := netlisten.Common(s.PreValue().(arc.PreData).Chrome)
	ls["127.0.0.1:5037"] = "/usr/bin/adb"
	// patchpaneld runs an ADB proxy server on port 5555 whenever ARC is running. The proxy end listens only when ADB sideloading or ADB debugging on dev mode is enabled.
	ls["*:5555"] = "/usr/bin/patchpaneld"
	// sslh is installed on ARC-capable systems to multiplex port 22 traffic between sshd and patchpaneld (for adb).
	ls["*:22"] = "/usr/sbin/sslh-fork"
	ls["*:2222"] = "/usr/sbin/sshd"
	// node_exporter runs on port 9090 to monitor DUT resources.
	ls["*:9090"] = "/usr/local/sbin/node_exporter"
	netlisten.CheckPorts(ctx, s, ls)
}
