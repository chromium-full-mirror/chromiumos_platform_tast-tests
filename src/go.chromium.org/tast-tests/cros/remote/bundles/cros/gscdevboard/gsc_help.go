// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	cr50HelpPrePvt = "cr50.help.prepvt.txt"
	cr50HelpMP     = "cr50.help.mp.txt"
	ti50Help       = "ti50.help.txt"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCHelp,
		Desc:    "Verify GSC help output",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310", "gsc_he",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Data:    []string{cr50HelpPrePvt, cr50HelpMP, ti50Help},
	})
}

// GSCHelp verifies GSC help output
func GSCHelp(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get GSC version")
	activeRw := version.ActiveRw()

	var filename string
	if f.TestbedProperties.TestbedType == ti50.GscH1Shield {
		if activeRw.Branch == ti50.MP {
			filename = cr50HelpMP
		} else {
			filename = cr50HelpPrePvt
		}
	} else {
		filename = ti50Help
	}
	expectedOutput, err := os.ReadFile(s.DataPath(filename))
	th.MustSucceed(err, "failed to read expected help output")

	// Set capabilities into their default states
	th.MustSucceed(i.CCDOpen(ctx), "failed to open CCD")
	th.MustSucceed(i.CCDReset(ctx), "failed to reset CCD")
	th.MustSucceed(i.CCDLock(ctx), "failed to lock CCD")

	expectedCommands := strings.Fields(string(expectedOutput))
	s.Logf("Expected commands: %s", expectedCommands)

	helpOutput, err := i.Help(ctx)
	th.MustSucceed(err, "failed to get help output")
	actualCommands := helpOutput.Commands
	s.Logf("actual commands: %s", actualCommands)
	extra, missing := commandDifference(expectedCommands, actualCommands)
	if len(missing) != 0 {
		s.Errorf("missing commands: %s", missing)
	}
	if len(extra) != 0 {
		s.Errorf("extra commands: %s", extra)
	}
}

func commandDifference(expectedCommands, commands []string) (extra, missing []string) {
	expectedCommandMap := make(map[string]bool, len(expectedCommands))
	commandMap := make(map[string]bool, len(commands))
	for _, command := range expectedCommands {
		expectedCommandMap[command] = true
	}
	for _, command := range commands {
		commandMap[command] = true
	}

	// If a command is in the actual output and not the expected output,
	// then it's extra
	for _, command := range commands {
		if _, ok := expectedCommandMap[command]; !ok {
			extra = append(extra, command)
		}
	}
	// If a command is in the expected output and not the actual output,
	// then it's missing
	for _, command := range expectedCommands {
		if _, ok := commandMap[command]; !ok {
			missing = append(missing, command)
		}
	}
	return
}
