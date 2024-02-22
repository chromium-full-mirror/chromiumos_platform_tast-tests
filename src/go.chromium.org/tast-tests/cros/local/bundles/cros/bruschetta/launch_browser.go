// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchBrowser,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Opens a browser window on the host from the guest, using several common approaches (/etc/alternatives, $BROWSER, and xdg-open)",
		Contacts:     []string{"clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline", "group:bruschetta_cq"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: bruschetta.BruschettaFixture,
				Val:     browser.TypeAsh,
			},
			{
				Name:    "lacros",
				Val:     browser.TypeLacros,
				Fixture: bruschetta.BruschettaFixtureWithLacros,
			},
		},
	})
}
func LaunchBrowser(ctx context.Context, s *testing.State) {
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	bt := s.Param().(browser.Type)

	s.Run(ctx, "testing_sh_c ", func(ctx context.Context, s *testing.State) {
		if err := guestos.LaunchBrowser(ctx, tconn, bt, bru, "browser-env", []string{"sh", "-c", "${BROWSER} http://browser-env.test/"}); err != nil {
			s.Fatal("Launching browser failed: ", err)
		}
	})
	s.Run(ctx, "testing_x_www_browser", func(ctx context.Context, s *testing.State) {
		if err := guestos.LaunchBrowser(ctx, tconn, bt, bru, "x-www-browser", []string{"/etc/alternatives/x-www-browser", "http://x-www-browser.test/"}); err != nil {
			s.Fatal("Launching browser failed: ", err)
		}
	})
	s.Run(ctx, "testing_xdg_open", func(ctx context.Context, s *testing.State) {
		if err := guestos.LaunchBrowser(ctx, tconn, bt, bru, "xdg-open", []string{"xdg-open", "http://xdg-open.test/"}); err != nil {
			s.Fatal("Launching browser failed: ", err)
		}
	})
}
