// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/selinux"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SELinuxProcesses,
		Desc: "Checks that processes are running in correct SELinux domain",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"selinux_current"},
		Attr:         []string{"group:mainline"},
	})
}

func SELinuxProcesses(ctx context.Context, s *testing.State) {
	selinux.ProcessesTestInternal(ctx, s)
}
