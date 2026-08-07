// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shimlessrma

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/shimlessrma/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IWARunRoutines,
		Desc: "Run healthd routines from IWA(Isolated Web App)",
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
		HardwareDeps: hwdep.D(hwdep.OEM("ASUS", "Google", "HP"), hwdep.MinStorage(16)),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "audio_driver",
			Val:  "audio_driver",
		}, {
			Name: "cpu_floating_point_accuracy",
			Val:  "cpu_floating_point_accuracy",
		}},
	})
}

func IWARunRoutines(ctx context.Context, s *testing.State) {
	// Note: If you modify the fixture/data/iwa or fixture/data/extension,
	// you have to rebuild them by running build_iwa_ext.sh.
	v := s.FixtValue().(*fixture.Value)
	ui := uiauto.New(v.Tconn)
	routine := s.Param().(string)

	routineButton := nodewith.HasClass("diagnostics-" + routine + "-button")
	textAreaFinder := nodewith.HasClass("diagnostics-" + routine + "-textarea")
	if err := uiauto.Combine("send request to healthd",
		ui.LeftClick(routineButton),
		ui.WaitUntilExists(textAreaFinder),
	)(ctx); err != nil {
		s.Fatal("Failed to click the button: ", err)
	}

	if node, err := ui.Info(ctx, textAreaFinder); err != nil {
		s.Fatal("Failed to get node info: ", err)
	} else if !strings.Contains(node.Value, "\"status\": \"passed\"") {
		s.Fatal("Routine result is not passed, result: ", node.Value)
	}
}
