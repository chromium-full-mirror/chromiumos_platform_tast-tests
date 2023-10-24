// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/graphics/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

/* TODO(b/307460167): Update this test to use modern Crostini fixtures.
import (
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
)

func TestGlBenchParams(t *testing.T) {
	// This test suite has some non-crostini test parameters, so add them here.
	params := `{
			Name:      "",
			Val:       config{config: &glbench.CrosConfig{}},
			Timeout:   3 * time.Hour,
			ExtraAttr: []string{"group:graphics", "graphics_nightly"},
			Fixture:   "graphicsNoChrome",
		}, {
			Name:      "hasty",
			Val:       config{config: &glbench.CrosConfig{Hasty: true}},
			ExtraAttr: []string{"group:mainline", "informational"},
			Timeout:   5 * time.Minute,
			Fixture:   "graphicsNoChrome",
		},`
	params += crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name:              "crostini_hasty",
			Timeout:           5 * time.Minute,
			Val:               `config{config: &glbench.CrostiniConfig{Hasty: true}}`,
			ExtraSoftwareDeps: []string{"chrome", "crosvm_gpu", "vm_host"},
			ExtraAttr:         []string{"group:graphics", "graphics_weekly"},
			MinimalSet:        true,
			OnlyStableBoards:  true,
		}})

	genparams.Ensure(t, "glbench.go", params)
} */
