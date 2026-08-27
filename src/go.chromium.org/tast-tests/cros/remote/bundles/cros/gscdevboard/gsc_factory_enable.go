// Copyright 2026 The ChromiumOS Authors
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

type testFactoryEnableConfig struct {
	bus                      ti50.TpmBus
	chassisOpenDuringEnable  bool
	chassisOpenDuringDisable bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFactoryEnable,
		Desc:    "Verifies factory enable works when chassis_open is asserted",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310", "gsc_he",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "i2c",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusI2c,
				chassisOpenDuringEnable:  true,
				chassisOpenDuringDisable: false,
			},
		}, {
			Name: "i2c_chassis_closed",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusI2c,
				chassisOpenDuringEnable:  false,
				chassisOpenDuringDisable: false,
			},
		}, {
			Name: "i2c_chassis_open",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusI2c,
				chassisOpenDuringEnable:  true,
				chassisOpenDuringDisable: true,
			},
		}, {
			Name: "spi",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusSpi,
				chassisOpenDuringEnable:  true,
				chassisOpenDuringDisable: false,
			},
		}, {
			Name: "spi_chassis_closed",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusSpi,
				chassisOpenDuringEnable:  false,
				chassisOpenDuringDisable: false,
			},
		}, {
			Name: "spi_chassis_open",
			Val: testFactoryEnableConfig{
				bus:                      ti50.TpmBusSpi,
				chassisOpenDuringEnable:  true,
				chassisOpenDuringDisable: true,
			},
		}},
	})
}

// GSCFactoryEnable verifies factory enable works when chassis_open is asserted
func GSCFactoryEnable(ctx context.Context, s *testing.State) {
	config := s.Param().(testFactoryEnableConfig)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(i.CCDOpen(ctx), "failed to open CCD")
	th.MustSucceed(i.CCDReset(ctx), "failed to reset CCD")
	caps, err := i.CCDCapabilities(ctx)
	th.MustSucceed(err, "failed to get caps")
	s.Logf("caps: %+v", caps)
	if !caps.IsReset || caps.IsFactoryReset {
		s.Fatalf("Caps not set to Default: %+v", caps)
	}

	tpm := b.ResetAndTpmStartupForBus(ctx, i, config.bus, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, config.chassisOpenDuringEnable)
	bp, err := i.Command(ctx, "bpforce")
	th.MustSucceed(err, "failed to get bpforce")
	s.Logf("batt state before factory enable: %s", bp)

	out, err := b.GSCToolCommandViaTPM(ctx, tpm.Bus, "", "--factory", "enable")
	s.Logf("gsc factory enable: %s", out)

	testing.Sleep(ctx, 5*time.Second) // GoBigSleepLint: delay gsc to wipe the tpm and set capabilities

	caps, capErr := i.CCDCapabilities(ctx)
	th.MustSucceed(capErr, "failed to get caps")
	s.Logf("caps: %+v", caps)

	if config.chassisOpenDuringEnable {
		th.MustSucceed(err, "failed factory enable")
		if !caps.IsFactoryReset {
			s.Errorf("caps not in factory mode: %+v", caps)
		}
	} else if err == nil {
		s.Error("Factory enable succeeded with CHASSIS_OPEN deasserted")
	} else if !caps.IsReset {
		s.Errorf("caps should still be reset: %+v", caps)
	}
	inFactoryMode := caps.IsFactoryReset

	tpm = b.ResetAndTpmStartupForBus(ctx, i, tpm.Bus, ti50.CCDModeOn, ti50.FfClamshell)

	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, config.chassisOpenDuringDisable)
	bp, err = i.Command(ctx, "bpforce")
	th.MustSucceed(err, "failed to get bpforce")
	s.Logf("batt state before factory disable: %s", bp)

	out, err = b.GSCToolCommandViaTPM(ctx, tpm.Bus, "", "--factory", "disable")
	if !inFactoryMode && !config.chassisOpenDuringDisable {
		if err == nil {
			s.Fatal("factory disable succeeded with CHASSIS_OPEN deasserted")
		}
	} else {
		th.MustSucceed(err, "failed factory disable")
	}
	s.Logf("gsc factory disable: %s", out)
	caps, err = i.CCDCapabilities(ctx)
	th.MustSucceed(err, "failed to get caps")
	s.Logf("caps: %+v", caps)
	if !caps.IsReset || caps.IsFactoryReset {
		s.Fatalf("caps not reset: %+v", caps)
	}
}
