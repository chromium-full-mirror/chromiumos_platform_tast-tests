// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

import (
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/vm"
)

func TestToolkitParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name:      "gtk3_wayland",
			ExtraData: []string{"toolkit_gtk3_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_gtk3_demo.py",
				Command: []string{"env", "GDK_BACKEND=wayland", "python3", "toolkit_gtk3_demo.py"},
			}`,
			UseFixture: true,
		}, {
			Name:      "gtk3_x11",
			ExtraData: []string{"toolkit_gtk3_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_gtk3_demo.py",
				Command: []string{"env", "GDK_BACKEND=x11", "python3", "toolkit_gtk3_demo.py"},
			}`,
			UseFixture: true,
		}, {
			Name:      "gtk4_wayland",
			ExtraData: []string{"toolkit_gtk4_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_gtk4_demo.py",
				Command: []string{"env", "GDK_BACKEND=wayland", "python3", "toolkit_gtk4_demo.py"},
			}`,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBookworm,
		}, {
			Name:      "gtk4_x11",
			ExtraData: []string{"toolkit_gtk4_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_gtk4_demo.py",
				Command: []string{"env", "GDK_BACKEND=x11", "python3", "toolkit_gtk4_demo.py"},
			}`,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBookworm,
		}, {
			Name:      "qt5_x11",
			ExtraData: []string{"toolkit_qt5_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_qt5_demo.py",
				Command: []string{"python3", "toolkit_qt5_demo.py"},
			}`,
			UseFixture: true,
		}, {
			// This was not previously tested in bullseye.
			Name:      "qt5_wayland",
			ExtraAttr: []string{"informational"},
			ExtraData: []string{"toolkit_qt5_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_qt5_demo.py",
				Command: []string{"python3", "toolkit_qt5_demo.py"},
			}`,
			UseFixture: true,
		}, {
			Name:      "qt6_x11",
			ExtraData: []string{"toolkit_qt6_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_qt6_demo.py",
				Command: []string{"python3", "toolkit_qt6_demo.py"},
			}`,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBookworm,
		}, {
			Name:      "qt6_wayland",
			ExtraData: []string{"toolkit_qt6_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_qt6_demo.py",
				Command: []string{"python3", "toolkit_qt6_demo.py"},
			}`,
			UseFixture:              true,
			MinimumContainerVersion: vm.DebianBookworm,
		}, {
			Name:      "tkinter",
			ExtraData: []string{"toolkit_tkinter_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_tkinter_demo.py",
				Command: []string{"python3", "toolkit_tkinter_demo.py"},
			}`,
			UseFixture: true,
		}})
	genparams.Ensure(t, "toolkit.go", params)
}
