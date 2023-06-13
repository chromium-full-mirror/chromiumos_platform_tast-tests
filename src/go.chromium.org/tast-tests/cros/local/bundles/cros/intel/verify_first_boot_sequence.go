// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/versionutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyFirstBootSequence,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify first boot sequence",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Attr:         []string{"group:intel-nda"},
		Timeout:      10 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func VerifyFirstBootSequence(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Opening OS Settings app.
	if err := apps.Launch(ctx, tconn, apps.Settings.ID); err != nil {
		s.Fatal("Failed to launch Settings app: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatal("Settings app did not appear in shelf after launch: ", err)
	}

	v, err := versionutil.AshVersion(ctx)
	if err != nil {
		s.Error("Failed to get Chrome version: ", err)
	}
	version := v.String()

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)
	aboutCrOSTab := nodewith.NameContaining("About ChromeOS").Role(role.StaticText)
	if err := ui.DoDefault(aboutCrOSTab)(ctx); err != nil {
		s.Fatal("Failed to click About ChromeOS tab: ", err)
	}

	versionRegex := nodewith.NameRegex(regexp.MustCompile("Version *")).Role(role.StaticText)
	info, err := ui.Info(ctx, versionRegex)
	if err != nil {
		s.Fatal("Failed to get node info: ", err)
	}

	if !strings.Contains(info.Name, version) {
		s.Fatalf("Failed to verify CrOS version: got %q, want %q", info.Name, version)
	}

	ecVersion, err := testexec.CommandContext(ctx, "ectool", "version").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get ectool version: ", err)
	}
	s.Logf("EC Version: %s", ecVersion)

	cbVersion, err := testexec.CommandContext(ctx, "crossystem", "fwid").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get coreboot version: ", err)
	}
	s.Logf("CB Version: %s", cbVersion)

}
