// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/userfiles"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/mountns"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UserFilesGuest,
		Desc: "Checks ownership and permissions of files for guest users",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
	})
}

func UserFilesGuest(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.GuestLogin())
	if err != nil {
		s.Fatal("Login failed: ", err)
	}
	defer cr.Close(ctx)

	if err := mountns.WithUserSessionMountNS(ctx, func(ctx context.Context) error {
		userfiles.Check(ctx, s, cr.NormalizedUser())
		return nil
	}); err != nil {
		s.Fatal("Failed to check user files: ", err)
	}
}
