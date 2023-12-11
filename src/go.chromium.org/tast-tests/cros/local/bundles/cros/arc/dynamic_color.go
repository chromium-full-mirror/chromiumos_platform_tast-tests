// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"encoding/json"
	"time"

	arcpkg "go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DynamicColor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that ArcSystemUIService changes Settings.Secure",
		Contacts: []string{
			"arc-app-dev@google.com",
		},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Timeout:      chrome.GAIALoginTimeout + arcpkg.BootTimeout + 2*time.Minute,
	})
}

func DynamicColor(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arcpkg.PreData).Chrome
	arc := s.FixtValue().(*arcpkg.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	var ret bool
	const overlayStyle = "Expressive"
	if err := tconn.Call(ctx, &ret, "tast.promisify(chrome.autotestPrivate.sendArcOverlayColor)", 50, overlayStyle); err != nil {
		s.Fatal("Failed to call sendArcOverlayColor: ", err)
	}
	if !ret {
		s.Fatal("Failed to parse return value of sendArcOverlayColor")
	}

	output, err := arc.Command(ctx, "settings", "get", "secure", "theme_customization_overlay_packages").Output()
	if err != nil {
		s.Fatal("Failed to get secure settings: ", err)
	}

	var themeOverlay map[string]interface{}
	if err := json.Unmarshal([]byte(string(output)), &themeOverlay); err != nil {
		s.Fatal("Failed to unmarshal output: ", err)
	}
	const testPaletteInHex = "000032"
	if palette := themeOverlay["android.theme.customization.system_palette"].(string); palette != testPaletteInHex {
		s.Errorf("Invalid system_palette got: %s, want: %s", palette, testPaletteInHex)
	}
	const testStyle = "EXPRESSIVE"
	if style := themeOverlay["android.theme.customization.theme_style"].(string); style != testStyle {
		s.Errorf("Invalid theme_style got: %s want: %s", style, testStyle)
	}
}
