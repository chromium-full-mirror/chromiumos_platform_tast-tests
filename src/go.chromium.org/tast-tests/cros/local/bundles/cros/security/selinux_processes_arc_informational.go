// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/selinux"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SELinuxProcessesARCInformational,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that processes are running in correct SELinux domain (new and flaky tests) after ARC boots",
		Contacts: []string{
			"chromeos-hardening@google.com",
			"niwa@chromium.org",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"selinux", "chrome"},
		Pre:          arc.Booted(),
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func SELinuxProcessesARCInformational(ctx context.Context, s *testing.State) {
	selinux.ProcessesTestInternal(ctx, s, []selinux.ProcessTestCaseSelector{selinux.Unstable})
}
