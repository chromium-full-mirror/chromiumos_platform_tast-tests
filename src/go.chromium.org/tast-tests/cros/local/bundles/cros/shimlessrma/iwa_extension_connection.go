// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shimlessrma

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/shimlessrma/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IWAExtensionConnection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check if IWA(Isolated Web App) can send message to extension and get response",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Shimless RMA
		BugComponent: "b:1002147",
		// These attributes make the test suite to run on the shimless supported models.
		Attr:         []string{"group:shimless_rma", "shimless_rma_normal"},
		Fixture:      fixture.Install3pDiag,
		SoftwareDeps: []string{"chrome"},
		// The OEMs align with the setting in Chrome: chrome/common/chromeos/extensions/chromeos_system_extension_info.cc.
		HardwareDeps: hwdep.D(hwdep.OEM("ASUS", "Google", "HP")),
		Timeout:      5 * time.Minute,
	})
}

func IWAExtensionConnection(ctx context.Context, s *testing.State) {
	// Note: If you modify the fixture/data/iwa or fixture/data/extension,
	// you have to rebuild them by running build_iwa_ext.sh.
	v := s.FixtValue().(*fixture.Value)
	ui := uiauto.New(v.Tconn)

	checkExtensionButton := nodewith.NameContaining("Check extension").Role(role.Button)
	textAreaFinder := nodewith.HasClass("check-extension-textarea")
	if err := uiauto.Combine("send request to extension",
		ui.LeftClick(checkExtensionButton),
		ui.WaitUntilExists(textAreaFinder),
	)(ctx); err != nil {
		s.Fatal("Failed to click the button: ", err)
	}

	if node, err := ui.Info(ctx, textAreaFinder); err != nil {
		s.Fatal("Failed to get node info: ", err)
	} else if node.Value != "\"Extension sample message\"" {
		s.Fatal("Failed to get correct response from extension")
	}
}
