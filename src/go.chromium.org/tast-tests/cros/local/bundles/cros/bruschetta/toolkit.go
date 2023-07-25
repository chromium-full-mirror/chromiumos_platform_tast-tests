// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Toolkit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the behaviour of GUI apps based on various toolkits",
		Contacts:     []string{"clumptini+oncall@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
		Timeout:      7 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "gtk3_wayland",
				ExtraData: []string{"toolkit_gtk3_demo.py"},
				Val: guestos.ToolkitConfig{
					Data:    "toolkit_gtk3_demo.py",
					Command: []string{"env", "GDK_BACKEND=wayland", "python3", "toolkit_gtk3_demo.py"},
				},
			}, {
				Name:      "gtk3_x11",
				ExtraData: []string{"toolkit_gtk3_demo.py"},
				Val: guestos.ToolkitConfig{
					Data:    "toolkit_gtk3_demo.py",
					Command: []string{"env", "GDK_BACKEND=x11", "python3", "toolkit_gtk3_demo.py"},
				},
			}, {
				Name:      "qt5_x11",
				ExtraData: []string{"toolkit_qt5_demo.py"},
				Val: guestos.ToolkitConfig{
					Data:    "toolkit_qt5_demo.py",
					Command: []string{"env", "QT_QPA_PLATFORM=xcb", "python3", "toolkit_qt5_demo.py"},
				},
			}, {
				Name:      "tkinter",
				ExtraData: []string{"toolkit_tkinter_demo.py"},
				Val: guestos.ToolkitConfig{
					Data:    "toolkit_tkinter_demo.py",
					Command: []string{"python3", "toolkit_tkinter_demo.py"},
				},
			},
		},
	})
}

func Toolkit(ctx context.Context, s *testing.State) {
	conf := s.Param().(guestos.ToolkitConfig)
	cr := s.FixtValue().(bruschetta.FixtureData).Chrome
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	if err := guestos.Toolkit(ctx, s.DataPath(conf.Data), s.OutDir(), &conf, cr, tconn, bru); err != nil {
		s.Fatal("Toolkit test failed: ", err)
	}
}
