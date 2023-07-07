// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Sysinfo,
		Desc:    "Most basic test of the ti50 sysinfo command",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_ot_fpga_cw310", "gsc_he", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

var devIDRegexp = regexp.MustCompile(`DEV_ID: *0x([0-9a-fA-F]+) +0x([0-9a-fA-F]+)`)

func Ti50Sysinfo(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting ti50")
	b.GpioApplyStrap(ctx, ti50.TpmSpi)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Simulate the typing of "sysinfo" command on Ti50 console.
	output, err := i.Command(ctx, "sysinfo")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	// Rudimentary validation of output: find and print "DEV_ID:" line.
	match := devIDRegexp.FindStringSubmatch(output)
	if match == nil {
		s.Error("Did not find DEV_ID among sysinfo output: ", output)
	} else {
		s.Log("DEV_ID: ", match[1], ":", match[2])
	}
}
