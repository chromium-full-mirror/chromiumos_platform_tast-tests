// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/userfiles"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserFilesLoggedIn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks ownership and permissions of user files for logged-in users",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Fixture:      "chromeLoggedIn",
	})
}

func UserFilesLoggedIn(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	userfiles.Check(ctx, s, cr.NormalizedUser())
}
