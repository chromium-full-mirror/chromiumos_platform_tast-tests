// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeGAIA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome can make real GAIA logins",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
		},
		VarDeps: []string{
			"ui.gaiaPoolDefault",
		},
		Timeout: chrome.GAIALoginTimeout + time.Minute,
	})
}

func ChromeGAIA(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(
		ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
	)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)
}
