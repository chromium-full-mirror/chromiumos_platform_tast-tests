// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenSaverPreview,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test previewing screen saver in the personalization hub app",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"cowmoo@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-972ece72-afe4-413f-a9b1-709c815c7f59",
		}},
		SoftwareDeps: []string{"chrome"},
		Timeout:      6 * time.Minute,
		Fixture:      "personalizationScreenSaverClamshell",
	})
}

func ScreenSaverPreview(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

	if err := uiauto.Combine("open ambient and start screen saver preview",
		ambient.OpenAmbientSubpage(ui),
		ambient.EnableAmbientMode(ui),
		ui.LeftClick(nodewith.Role(role.Button).HasClass("preview-button")),
		ui.WaitUntilExists(nodewith.HasClass("preview-button-disabled")),
		ui.WaitUntilExists(nodewith.ClassName("InSessionAmbientModeContainer").Role(role.Window)),
		ambient.CloseScreenSaverPreview(tconn, ui),
		personalization.ClosePersonalizationHub(ui))(ctx); err != nil {
		s.Fatal("Failed to start ambient preview: ", err)
	}
}
