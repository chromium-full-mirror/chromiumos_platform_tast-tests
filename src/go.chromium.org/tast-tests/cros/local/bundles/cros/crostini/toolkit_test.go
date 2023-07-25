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
			Name:      "qt5",
			ExtraData: []string{"toolkit_qt5_demo.py"},
			Val: `guestos.ToolkitConfig{
				Data:    "toolkit_qt5_demo.py",
				Command: []string{"python3", "toolkit_qt5_demo.py"},
			}`,
			UseFixture: true,
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
