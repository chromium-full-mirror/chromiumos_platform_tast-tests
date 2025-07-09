// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"github.com/google/go-tpm/tpm2"
	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testTPMCmd struct {
	bus      ti50.TpmBus
	cmd      string
	function func(context.Context, *testing.State, utils.DevboardHelper, *utils.TpmHelper)
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTPM,
		Desc:    "Test TPM functionality of ti50 in remote environment(Andreiboard connected to devboardsvc host)",
		Timeout: 60 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"aluo@chromium.org",     // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_fpga_cw310", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "spi_apro_boot",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "-B",
			},
		}, {
			Name: "i2c_apro_boot",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "-B",
			},
		}, {
			Name: "spi_get_time",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--get_time",
			},
		}, {
			Name: "i2c_get_time",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--get_time",
			},
		}, {
			Name: "spi_ccd_info",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--ccd_info",
			},
		}, {
			Name: "i2c_ccd_info",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--ccd_info",
			},
		}, {
			Name: "spi_board_id",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--board_id",
			},
		}, {
			Name: "i2c_board_id",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--board_id",
			},
		}, {
			Name: "spi_fwver",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--fwver",
			},
		}, {
			Name: "i2c_fwver",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--fwver",
			},
		}, {
			Name: "spi_metrics",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--metrics",
			},
		}, {
			Name: "i2c_metrics",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--metrics",
			},
		}, {
			Name: "spi_wp",
			Val: testTPMCmd{
				bus: ti50.TpmBusSpi,
				cmd: "--wp",
			},
		}, {
			Name: "i2c_wp",
			Val: testTPMCmd{
				bus: ti50.TpmBusI2c,
				cmd: "--wp",
			},
		}, {
			Name: "spi_nv_read",
			Val: testTPMCmd{
				bus:      ti50.TpmBusSpi,
				function: tpmNvRead,
			},
		}, {
			Name: "i2c_nv_read",
			Val: testTPMCmd{
				bus:      ti50.TpmBusI2c,
				function: tpmNvRead,
			},
		}, {
			Name: "spi_tpm_property_vendor_type",
			Val: testTPMCmd{
				bus:      ti50.TpmBusSpi,
				function: tpmProperty,
			},
		}},
	})
}

func tpmProperty(ctx context.Context, s *testing.State, b utils.DevboardHelper, tpm *utils.TpmHelper) {
	vendorTpmType, err := tpm.GetTPMProperty(tpm2.TPMPTVendorTPMType)
	if err != nil {
		s.Fatal("Could not get Vendor TPM Type property")
	}
	s.Log("Vendor TPM Type value: ", vendorTpmType)

	expectedTpmType := uint32(1)
	if b.GscProperties().ChipType() == ti50.GscOT {
		// NT devices should specify 2 as the vendor TPM type to differentiate
		// their version hash.
		expectedTpmType = 2
	}

	if vendorTpmType != expectedTpmType {
		s.Errorf("Vendor TPM Type incorrect: got %d want %d", vendorTpmType, expectedTpmType)
	}
}

func tpmNvRead(ctx context.Context, s *testing.State, b utils.DevboardHelper, tpm *utils.TpmHelper) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	// NV_Read first 0x101 bytes from EKcert
	// size: 0101
	// offset: 0000
	// auth 00000009400000090000000000 (size=13)
	//   size: 00000009
	//   session: TPM_RS_PW = 40000009
	//   nonce size: 0000
	//   sess attr: 00
	//   auth size: 0000
	// total cmd size: 10+8+13+4 = 35 = 0x23
	_, err := tpm.OpenTitanToolTpmCommand("execute-command", "--hexdata", "8002000000230000014e01c0000101c000010000000940000009000000000001010000")
	th.MustSucceed(err, "GSC NV_Read")
}

func GSCTPM(ctx context.Context, s *testing.State) {
	config := s.Param().(testTPMCmd)
	bus := config.bus
	cmd := config.cmd
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	var gpioMonitor utils.GpioMonitorSession
	if bus == ti50.TpmBusSpi {
		// Record everything that is transmitted by SPI bus lines CLK/CS/MISO/MOSI, for manual inspection later.
		gpioMonitor = b.GpioMonitorStart(
			ctx,
			ti50.Ti50SpiTpmCs,
			ti50.Ti50SpiTpmSck,
			ti50.Ti50SpiTpmMosi,
			ti50.Ti50SpiTpmMiso)
	} else {
		// Record everything that is transmitted by I2C bus lines SDA/SCL, for manual inspection later.
		gpioMonitor = b.GpioMonitorStart(
			ctx,
			ti50.GpioTi50DeviceI2cSda,
			ti50.GpioTi50DeviceI2cScl)
	}
	// Store transcript of TPM signal events events in .vcd format, to be reviewed in e.g. Pulseview.
	defer func(ctx context.Context) {
		events := b.GpioMonitorFinish(ctx, gpioMonitor)
		gpioMonitor.Save(ctx, events, "tpm.vcd")
	}(ctx)

	nctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	tpmHandle := b.ResetAndTpmStartupForBus(nctx, i, bus, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	// Read boot mode as a simple check of vendor command.
	bm, err := tpmHandle.TpmvGetBootMode()
	if err != nil {
		s.Error("boot mode error: ", err)
	}
	s.Logf("Read boot mode %d", bm)

	events := b.GpioMonitorRead(ctx, gpioMonitor)
	gpioMonitor.Save(ctx, events, "setup.vcd")

	if config.function != nil {
		config.function(ctx, s, b, tpmHandle)
	} else if config.cmd != "" {
		out, err := b.GSCToolCommandViaTPM(ctx, bus, "", cmd)
		if err != nil {
			s.Error("Could not get version via TPM: ", err)
		}
		s.Logf("GSCTool %s output: %s", cmd, out)
	} else {
		s.Error("Missing configuration parameter, either cmd or function")
	}

	events = b.GpioMonitorRead(ctx, gpioMonitor)
	gpioMonitor.Save(ctx, events, "cmd.vcd")
}
