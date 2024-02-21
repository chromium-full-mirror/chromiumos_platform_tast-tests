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

func TestCopyPasteParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name: "wayland_to_wayland",
			Val: `guestos.CopyPasteConfig{
				Copy:  guestos.WaylandCopyConfig,
				Paste: guestos.WaylandPasteConfig,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		},
		{
			Name: "wayland_to_x11",
			Val: `guestos.CopyPasteConfig{
				Copy:  guestos.WaylandCopyConfig,
				Paste: guestos.X11PasteConfig,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		},
		{
			Name: "x11_to_wayland",
			Val: `guestos.CopyPasteConfig{
				Copy:  guestos.X11CopyConfig,
				Paste: guestos.WaylandPasteConfig,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		},
		{
			Name: "x11_to_x11",
			Val: `guestos.CopyPasteConfig{
				Copy:  guestos.X11CopyConfig,
				Paste: guestos.X11PasteConfig,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		}})
	genparams.Ensure(t, "copy_paste.go", params)
}
