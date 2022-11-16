// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeGAIA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome can make real GAIA logins",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"tast-owners@google.com",
			"cros-oac@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:mainline",
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
