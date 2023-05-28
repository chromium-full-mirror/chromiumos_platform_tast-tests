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
		Func: SELinuxFilesSystemInformational,
		Desc: "Checks that SELinux file labels are set correctly for system files (new testcases, flaky testcases)",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		Attr:         []string{},
		SoftwareDeps: []string{"selinux"},
	})
}

func SELinuxFilesSystemInformational(ctx context.Context, s *testing.State) {
	type rwFilter int
	const (
		readonly rwFilter = iota
		writable
	)

	// Files to be tested.
	var testArgs []selinux.FileTestCase

	selinux.FilesTestInternal(ctx, s, testArgs)
}
