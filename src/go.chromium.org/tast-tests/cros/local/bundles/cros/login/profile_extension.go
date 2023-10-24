// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProfileExtension,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check private signin profile extension loads",
		Contacts: []string{
			"cros-lurs@google.com",
			"antrim@google.com",
			"cros-oac@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func ProfileExtension(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	if _, err := cr.SigninProfileTestAPIConn(ctx); err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
}
