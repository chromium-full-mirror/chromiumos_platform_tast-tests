// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testGSCFWMPResetECRecDev struct {
	bidType     ti50.BIDField
	expectReset bool
}

const (
	// This is a non-zero BID. At this point no GSCs have the EC_RST_L
	// pulse enabled by default. You can add a flag to start using the code
	// and this test BID.
	// If the code is ever enabled for a BID, you can add a valid test BID
	// here
	testTypeResetEC  = ti50.BIDField(0x12345678)
	testFlagsResetEC = ti50.BIDField(0xfffff)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFWMPResetECRecDev,
		Desc:    "Verify GSC rec+dev ecrst behavior",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Params: []testing.Param{{
			Name: "bid_enabled",
			// Support for this is not compiled in by default.
			// You can run this test case with a special image.
			Val: testGSCFWMPResetECRecDev{
				bidType:     testTypeResetEC,
				expectReset: true,
			},
		}, {
			Name: "bid_unknown",
			Val: testGSCFWMPResetECRecDev{
				bidType:     testTypeResetEC,
				expectReset: false,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			Name: "bid_erased",
			Val: testGSCFWMPResetECRecDev{
				bidType:     ti50.UnsetBID,
				expectReset: false,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}},
	})
}

// GSCFWMPResetECRecDev verifies GSC blocks rec+dev mode with a EC_RST_L pulse
// on devices with the correct board id when the FWMP block dev mode flag is
// set. It verifies GSC proceeds normally if the device doesn't have one of the
// allowlisted board ids.
func GSCFWMPResetECRecDev(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	config := s.Param().(testGSCFWMPResetECRecDev)
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	if config.bidType != ti50.UnsetBID {
		err := tpm.TpmvSetBoardID(config.bidType, testFlagsResetEC)
		th.MustSucceed(err, "failed to set board id type")

		bid, err := i.ChipBID(ctx)
		th.MustSucceed(err, "failed to get board id")
		s.Logf("Full BID: %+v", bid)
		if bid.TypeIsErased || bid.Type != config.bidType {
			s.Fatal("Unable to set board id type")
		}
		if bid.FlagsAreErased || bid.Flags != testFlagsResetEC {
			s.Fatal("Flags were not set correctly")
		}
	}
	setFWMPCheckPCR0ECResetBehavior(ctx, s, b, i, "0 fwmp", false, true, 0)
	setFWMPCheckPCR0ECResetBehavior(ctx, s, b, i, "Fwmp disable dev mode", config.expectReset, true, utils.FWMPDisableDevMode)
	setFWMPCheckPCR0ECResetBehavior(ctx, s, b, i, "Fwmp disable unlock", false, true, utils.FWMPDisableUnlock)
	setFWMPCheckPCR0ECResetBehavior(ctx, s, b, i, "No fwmp", false, false, 0)
}

// setFWMPCheckPCR0ECResetBehavior verifies the GSC EC_RST_L behavior after
// trying to extend PCR0 to all supported boot modes
func setFWMPCheckPCR0ECResetBehavior(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, desc string, expectReset, setFwmp bool, fwmp uint32) {
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)
	tpm.NvUndefineSpace(ti50.FwmpAttr())
	// Wipe the TPM to completely remove the fwmp.
	err := i.WipeTpmWithCCDOpen(ctx)
	if err != nil {
		s.Fatalf("%s: unable to wipe the TPM: %+v", desc, err)
	}

	tpm = b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)

	if setFwmp {
		attr := ti50.FwmpAttr()
		nvName, err := tpm2.NVName(&attr)
		if err != nil {
			s.Errorf("%s: failed to get NV name: %+v", desc, err)
		}
		nvHandle := tpm2.NamedHandle{
			Handle: attr.NVIndex,
			Name:   *nvName,
		}

		s.Logf("Write FWMP %x", fwmp)
		fwmpFile := utils.MakeFWMPFile(fwmp)
		// Define space in NV storage and clean up afterwards
		def := tpm2.NVDefineSpace{
			AuthHandle: tpm2.TPMRHPlatform,
			Auth:       ti50.EmptyPassword(),
			PublicInfo: tpm2.New2B(attr),
		}
		if _, err := def.Execute(tpm); err != nil {
			s.Fatalf("%s: NVDefineSpace failed: %+v", desc, err)
		}
		defer tpm.NvUndefineSpace(attr)
		// Write the fwmp file data to new space.
		write := tpm2.NVWrite{
			AuthHandle: tpm2.TPMRHPlatform,
			NVIndex:    nvHandle,
			Data: tpm2.TPM2BMaxNVBuffer{
				Buffer: fwmpFile,
			},
			Offset: 0,
		}
		if _, err := write.Execute(tpm); err != nil {
			s.Fatalf("%s: NVWrite failed: %+v", desc, err)
		}
	}

	// The ccd command prints the fwmp state.
	ccd, err := i.Command(ctx, "ccd")
	if err == nil {
		s.Logf("end ccd: %s", ccd)
	}

	err = extendPCR0CheckECReset(ctx, b, i, false, ti50.ExtendNormalBoot, ti50.DigestNormalBoot)
	if err != nil {
		s.Errorf("%s: unexpected normal mode behavior: %+v", desc, err)
	}
	err = extendPCR0CheckECReset(ctx, b, i, false, ti50.ExtendDevBoot, ti50.DigestDevBoot)
	if err != nil {
		s.Errorf("%s: unexpected dev mode behavior: %+v", desc, err)
	}
	err = extendPCR0CheckECReset(ctx, b, i, expectReset, ti50.ExtendRecDevBoot, ti50.DigestRecDevBoot)
	if err != nil {
		s.Errorf("%s: unexpected rec+dev mode behavior: %+v", desc, err)
	}
	err = extendPCR0CheckECReset(ctx, b, i, false, ti50.ExtendRecBoot, ti50.DigestRecBoot)
	if err != nil {
		s.Errorf("%s: unexpected rec mode behavior: %+v", desc, err)
	}
	err = extendPCR0CheckECReset(ctx, b, i, false, ti50.ExtendUnknownBoot, ti50.DigestUnknownBoot)
	if err != nil {
		s.Errorf("%s: unexpected non-standard PCR0 behavior: %+v", desc, err)
	}
	tpm.NvUndefineSpace(ti50.FwmpAttr())
}

// extendPCR0CheckECReset verifies the GSC behavior after trying to extend PCR0
func extendPCR0CheckECReset(ctx context.Context, b utils.DevboardHelper, i *ti50.CrOSImage, expectReset bool, extend, expectedDigest string) error {
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)

	i.Command(ctx, "ccd")

	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)

	// Read from the gpio monitor to clear existing events.
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	testing.ContextLog(ctx, "Cleared Events: ", events)

	tpm.PCRExtendCheckDigest(0, extend, expectedDigest)

	// Read new events from the GPIO monitor
	events = b.GpioMonitorFinish(ctx, gpioMonitor)
	testing.ContextLog(ctx, "EC_RST_L Events: ", events)

	ccdstate, err := i.CCDStateInfo(ctx)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "ccdstate: %+v", ccdstate)

	if !strings.Contains(strings.ToLower(ccdstate.PCR0), strings.ToLower(expectedDigest)) {
		return errors.Errorf("Digest not found in ccdstate output: %+v", ccdstate)
	}

	if expectReset {
		if len(events.Sorted) == 0 {
			return errors.Errorf("%s did not trigger EC_RST_L pulse", extend)
		}
		testing.ContextLogf(ctx, "%s triggered a EC_RST_L pulse", extend)
	} else if len(events.Sorted) != 0 {
		return errors.Errorf("%s triggered a EC_RST_L pulse", extend)
	}

	return nil
}
