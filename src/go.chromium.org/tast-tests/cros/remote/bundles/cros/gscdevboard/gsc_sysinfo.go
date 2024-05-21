// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCSysinfo,
		Desc:    "Most basic test of the gsc sysinfo command",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"jbk@chromium.org",       // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310", "gsc_he",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCSysinfo(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Simulate the typing of "sysinfo" command on GSC console.
	sysinfo, err := i.GetSysinfo(ctx)
	if err != nil {
		s.Fatal("Error communicating with GSC: ", err)
	}

	// Rudimentary validation of output: find and print "DEV_ID:" line.
	s.Log("DEV_ID: ", sysinfo.Devid)

	version, err := i.GetVersionInfo(ctx)
	if err != nil {
		s.Fatal("Unable to get version output: ", err)
	}
	isReleaseBranch := false
	if version.Build.Branch == ti50.MP || version.Build.Branch == ti50.PrePvt {
		s.Log("Image was built from a release branch")
		isReleaseBranch = true
	}

	if sysinfo.ProdKeyladder {
		if version.RwA.Active {
			s.Logf("%+v is using the prod key ladder", version.RwA)
		} else if version.RwB.Active {
			s.Logf("%+v is using the prod key ladder", version.RwB)
		}

		if b.TestbedType == ti50.GscH1Shield && !isReleaseBranch {
			// Images from the H1 TOT branch should not use the prod keyladder.
			s.Fatalf("Found prod Key Ladder in a non-release H1 image: %+v", sysinfo)
		}
	} else if isReleaseBranch {
		s.Fatalf("Did not find prod Key Ladder in release image: %+v", sysinfo)
	}
}
