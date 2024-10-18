// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCDBGSysinfo,
		Desc:    "Verify sysinfo output is correct for a DBG image",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Fixture:      fixture.SystemDevboard,
	})
}

func GSCDBGSysinfo(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Simulate the typing of "sysinfo" command on GSC console.
	sysinfo, err := i.Sysinfo(ctx)
	if err != nil {
		s.Fatal("Error communicating with GSC: ", err)
	}

	// Rudimentary validation of output: find and print "DEV_ID:" line.
	s.Log("DEV_ID: ", sysinfo.Devid)

	version, err := i.VersionInfo(ctx)
	if err != nil {
		s.Fatal("Unable to get version output: ", err)
	}
	rwVersion := version.ActiveRw().Version
	s.Log("RW_VER: ", rwVersion)
	epoch := strings.Split(rwVersion, ".")[0]
	s.Log("epoch: ", epoch)

	if sysinfo.ProdKeyladder {
		s.Errorf("Found prod Key Ladder in a DBG image: %+v", sysinfo)
	}
	if epoch != "1" {
		s.Errorf("Epoch is not 1 in RW version %s", rwVersion)
	}

	// TODO(b/374809074): check that 128 rollback bits are blown in the image.
}
