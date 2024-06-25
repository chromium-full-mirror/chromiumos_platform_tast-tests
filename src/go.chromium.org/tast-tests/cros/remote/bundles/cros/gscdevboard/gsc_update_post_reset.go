// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	reImageUpdated = regexp.MustCompile(`image updated`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUpdatePostReset,
		Desc:    "Verify GSC post reset does not turn on the update until PLT_RST_L is asserted or TurnUpdateOn is sent",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCUpdate,
		Params: []testing.Param{{
			Name: "i2c",
			Val:  ti50.TpmBusI2c,
		}, {
			Name: "spi",
			Val:  ti50.TpmBusSpi,
		}},
	})
}

// detectedReset returns True if GSC did a hard reset that triggered a pulse on EC_RST_L
func detectedReset(ctx context.Context, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession) bool {
	// GSC pulses EC_RST_L when it resets. Wait for a EC_RST_L pulse to detect the reset.
	events := b.GpioMonitorWait(ctx, gpioMonitor, 2*time.Second, 100*time.Millisecond)
	testing.ContextLog(ctx, "gpio events:", events.Sorted)
	return len(events.Sorted) != 0
}

// waitForUpdate waits for a GSC reset to pickup the update
func waitForUpdate(ctx context.Context, b utils.DevboardHelper, i *ti50.CrOSImage, gpioMonitor utils.GpioMonitorSession) error {
	if !detectedReset(ctx, b, gpioMonitor) {
		return errors.New("failed to detect EC_RST_L pulse")
	}
	err := i.WaitUntilBooted(ctx)
	if err != nil {
		return errors.Wrap(err, "GSC did not revive from reboot")
	}
	return nil
}

// GSCUpdatePostReset requires HW setup with SuzyQ cable from devboard to drone/workstation.
func GSCUpdatePostReset(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)
	testBus := s.Param().(ti50.TpmBus)

	// Valid image under test path.
	if f.ImagePath == "" {
		s.Fatal("Supply buildurl")
	}

	// Ti50 devices turn the update on when PLT_RST_L is asserted. Cr50 waits
	// until it sees the TurnUpdateOn vendor command
	turnUpdateOnWithVC := f.TestbedProperties.TestbedType == ti50.GscH1Shield

	imageUnderTest := f.ImagePath
	_, releaseVer, _, _, err := b.GSCToolBinVersion(ctx, imageUnderTest)
	th.MustSucceed(err, "failed to parse image under test version")

	s.Logf("Image under test %s: %s", releaseVer, imageUnderTest)

	// Connect Suzyq, so the test can update over ccd.
	s.Log("Simulating insertion of SuzyQ and resetting")
	b.ResetAndTpmStartupForBus(ctx, i, testBus, ti50.CcdSuzyQ, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.WaitUntilCCDConnected(ctx)

	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)

	// TODO(mruthven): use GSCToolPostReset after CL:5655336 lands.
	out, _ := b.GSCToolCommand(ctx, imageUnderTest, "--post_reset")
	// GSCTool responds with exit status 1 if the update passed. The error
	// isn't useful. Check for "image updated" in the output.
	if !reImageUpdated.Match(out) {
		s.Fatalf("Update failed: %s", out)
	}
	s.Logf("posted update: %s", out)

	if detectedReset(ctx, b, gpioMonitor) {
		s.Fatal("GSC turned on the update immediately with post reset")
	}

	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)

	if !turnUpdateOnWithVC {
		err = waitForUpdate(ctx, b, i, gpioMonitor)
		th.MustSucceed(err, "PLT_RST_L did not turn on update")
		s.Log("GSC turned on the update when PLT_RST_L was asserted")
		return
	}

	b.WaitUntilDeepSleep(ctx, i, ti50.WaitForSleepTimeout)
	s.Log("Entered deep sleep")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	if detectedReset(ctx, b, gpioMonitor) {
		s.Fatal("GSC reset before turning on the update")
	}

	sysinfo, err := i.Sysinfo(ctx)
	th.MustSucceed(err, "get sysinfo")
	s.Logf("sysinfo: %+v", sysinfo)

	if sysinfo.ResetFlags&ti50.GscResetFlagHibernate == 0 {
		s.Fatal("Did not detect deep sleep resume")
	}
	s.Log("Ran deep sleep reset")

	bus, err := i.BoardPropertiesTPMBus(ctx)
	th.MustSucceed(err, "get board properties")
	s.Logf("TPM bus after deep sleep: %+v", bus)

	if bus != testBus {
		s.Fatalf("TPM bus changed after deep sleep: expected %v got %v", testBus, bus)
	}
	tpm := b.Tpm(ctx, testBus)
	b.WaitForTpm(ctx, tpm)
	// The AP typically uses a 1000ms delay.
	tpm.TpmvTurnUpdateOn(1000)
	err = waitForUpdate(ctx, b, i, gpioMonitor)
	th.MustSucceed(err, "Turn on update vendor command did not turn on update")

	s.Log("Turned update on after deep sleep")
}
