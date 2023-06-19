// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppFirefoxNoArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         `Open Firefox, check rendering by looking for a rendered browser tab with the title "Welcome to Firefox" or "New Tab" via ACUITI`,
		Contacts:     []string{"clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "vm_host"},
		BugComponent: "b:1122570",
		Params: []testing.Param{
			{
				Name:              "bullseye_clamshell_unstable_no_arc",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBullseyeLargeContainerClamshellWithoutArc",
				Timeout:           15 * time.Minute,
			},
		},
	})
}

func AppFirefoxNoArc(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(crostini.FixtureData).Tconn
	keyboard := s.FixtValue().(crostini.FixtureData).KB
	cr := s.FixtValue().(crostini.FixtureData).Chrome

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	ud := uidetection.NewDefault(tconn)
	ui := uiauto.New(tconn)
	firefoxWindow := nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()

	const startupTimeout = 2 * time.Minute // slower devices could take up to two minutes to start Firefox
	if err := uiauto.Combine("verify Firefox",
		launcher.SearchAndLaunchWithQuery(tconn, keyboard, "f", "Firefox ESR"),
		ui.WithTimeout(startupTimeout).WaitUntilExists(firefoxWindow),
		uiauto.IfFailThen(
			ud.WithTimeout(startupTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Welcome", "to", "Firefox"}).WithinA11yNode(firefoxWindow).First()),
			ud.WithTimeout(startupTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Get", "started"}).WithinA11yNode(firefoxWindow).First()),
		),
		ui.WithInterval(time.Second).RetryUntil(
			keyboard.AccelAction("ctrl+w"),
			ui.WithTimeout(3*time.Second).WaitUntilGone(firefoxWindow),
		),
	)(ctx); err != nil {
		s.Fatal("Failed to verify Firefox: ", err)
	}
}
