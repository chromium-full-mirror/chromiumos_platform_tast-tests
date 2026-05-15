// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     UnicornPlaystoreOn,
		Desc:     "Checks if PlayStore is Enabled for Unicorn Acccount",
		Contacts: []string{"arc-commercial@google.com", "cros-arc-te@google.com", "jinrongwu@google.com", "mattlui@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:release-health", "release-health_arc"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
				ExtraAttr:         []string{"informational", "group:release-health"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Fixture: "familyLinkUnicornArcLogin",
	})
}

func UnicornPlaystoreOn(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	st, err := arc.GetState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get ARC state: ", err)
	}
	if st.Provisioned {
		s.Log("ARC is already provisioned. Skipping the Play Store setup")
	} else {
		if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
			s.Fatal("Failed to optin to Play Store and Close: ", err)
		}
	}
}
