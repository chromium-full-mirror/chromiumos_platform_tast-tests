// Copyright 2025 The ChromiumOS Authors
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

const (
	// Expected FIPS digest for RW_A
	fipsDigestA = "a4459b5bcbda8970"
	// Expected FIPS digest for RW_B
	fipsDigestB = "e2c63d3a2214ae8c"
)

// Console Regular expressions.
var (
	fipsDigestRE        = regexp.MustCompile("FIPS module digest ([0-9a-f]+)...")
	fipsUninitializedRE = regexp.MustCompile("FIPS mode not initialized")
	fipsErrorRE         = regexp.MustCompile("FIPS error code (0x[0-9a-fA-F]+), not-approved")
	fipsModeRE          = regexp.MustCompile("Running in FIPS 140-2 (not-)?approved mode")
	fipsCryptoRE        = regexp.MustCompile("FIPS crypto allowed: ([0-9])")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Cr50FIPS,
		Desc:    "Verify Cr50 FIPS output",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "power_on",
			Val:  ti50.GscResetFlagPowerOn,
		}, {
			Name: "hard_reset",
			Val:  ti50.GscResetFlagHard,
		}, {
			Name: "deep_sleep",
			Val:  ti50.GscResetFlagHibernate,
		}},
	})
}

// Cr50FIPS verify FIPS output after different types of reset.
func Cr50FIPS(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	resetType := s.Param().(int)

	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	switch resetType {
	case ti50.GscResetFlagPowerOn:
		s.Log("Already ran power-on reset")
	case ti50.GscResetFlagHard:
		th.MustSucceed(i.Reboot(ctx), "reboot failed")
	case ti50.GscResetFlagHibernate:
		// Get some state for debugging
		gpioget, err := i.Command(ctx, "gpioget")
		th.MustSucceed(err, "failed to get gpioget output")
		s.Logf("gpioget: %s", gpioget)
		sleepmask, err := i.Command(ctx, "sleepmask")
		th.MustSucceed(err, "failed to get sleepmask output")
		s.Logf("sleepmask: %s", sleepmask)

		th.MustSucceed(b.WaitUntilDeepSleep(ctx, i, ti50.WaitForSleepTimeout), "failed to enter deep sleep")
		// Wake GSC with the power button
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	default:
		s.Fatalf("Unsupported reset type: %x", resetType)
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	// Verify GSC did the correct reset
	sysinfo, err := i.Sysinfo(ctx)
	th.MustSucceed(err, "failed to get GSC sysinfo")
	s.Logf("sysinfo reset flags: %x", sysinfo.ResetFlags)
	if sysinfo.ResetFlags&uint32(resetType) == 0 {
		s.Fatalf("%x not found in sysinfo reset flags %x", resetType, sysinfo.ResetFlags)
	}
	s.Log("Ran GSC reset")

	version, err := i.VersionInfo(ctx)
	th.MustSucceed(err, "failed to get version")
	var expectedFIPSDigest string
	if version.RwA.Active {
		expectedFIPSDigest = fipsDigestA
	} else {
		expectedFIPSDigest = fipsDigestB
	}

	out, err := i.Command(ctx, "fips")
	th.MustSucceed(err, "failed to run fips command")
	s.Logf("fips: %s", out)

	match := fipsDigestRE.FindStringSubmatch(out)
	if match == nil {
		s.Error("Failed to find FIPS digest")
	} else if match[1] == expectedFIPSDigest {
		s.Logf("Valid digest: %s", match[1])
	} else {
		s.Errorf("Invalid digest: expected %s got %s", expectedFIPSDigest, match[1])
	}

	if fipsUninitializedRE.MatchString(out) {
		s.Error("FIPS is unitialized")
	}

	match = fipsErrorRE.FindStringSubmatch(out)
	if match == nil {
		s.Log("FIPS did not report an error")
	} else {
		s.Errorf("FIPS error: %s", match[0])
	}

	match = fipsModeRE.FindStringSubmatch(out)
	if match == nil {
		s.Error("Failed to find FIPS mode")
	} else if match[1] == "not-" {
		s.Error("FIPS is running in not approved mode")
	} else {
		s.Logf("FIPS is enabled: %s", match)
	}

	match = fipsCryptoRE.FindStringSubmatch(out)
	if match == nil {
		s.Error("Failed to find FIPS crypto status")
	} else if match[1] == "1" {
		s.Logf("Valid crypto status: %s", match[1])
	} else {
		s.Errorf("Invalid crypto status: %s", match[1])
	}

}
