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

func TestSecureCopyPasteParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name:      "copy_wayland",
			ExtraData: []string{"secure_copy.py"},
			Val: `secureCopyPasteConfig{
				backend: "wayland",
				app:     "secure_copy.py",
				action:  copying,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		}, {
			Name:      "copy_x11",
			ExtraData: []string{"secure_copy.py"},
			Val: `secureCopyPasteConfig{
				backend: "x11",
				app:     "secure_copy.py",
				action:  copying,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		}, {
			Name:      "paste_wayland",
			ExtraData: []string{"secure_paste.py"},
			Val: `secureCopyPasteConfig{
				backend: "wayland",
				app:     "secure_paste.py",
				action:  pasting,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		}, {
			Name:      "paste_x11",
			ExtraData: []string{"secure_paste.py"},
			Val: `secureCopyPasteConfig{
				backend: "x11",
				app:     "secure_paste.py",
				action:  pasting,
			}`,
			UseFixture:       true,
			OnlyStableBoards: true,
		}})
	genparams.Ensure(t, "secure_copy_paste.go", params)
}
