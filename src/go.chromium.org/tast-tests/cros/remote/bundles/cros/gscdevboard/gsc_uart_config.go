// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUARTConfig,
		Desc:    "Tests changing EC UART settings",
		Timeout: 90 * time.Second,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"ecgh@chromium.org",      // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_fpga_cw310", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCUARTConfig(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("(Re)starting ti50")
	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnectedAndUARTTXEnabled(ctx)

	testECUART(ctx, s, b, i, th, 9600)
	testECUART(ctx, s, b, i, th, 57600)
	testECUART(ctx, s, b, i, th, 115200)
}

func testECUART(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper, baud int) {
	// Use the bitbang command to set the baud.
	_, err := i.Command(ctx, fmt.Sprintf("bitbang 2 %d none", baud))
	th.MustSucceed(err, "Command error")

	uart := b.PhysicalUartWithBaud(ti50.UartEC, baud)
	ccd := b.CcdSerialInterfaceWithBaud(ti50.UartEC, time.Second, baud)
	th.MustSucceed(ccd.Open(ctx), "Failed to open ccd")
	defer ccd.Close(ctx)
	th.MustSucceed(uart.Open(ctx), "Failed to open uart")
	defer uart.Close(ctx)

	// Flush out any data.
	uart.WriteSerial(ctx, []byte("AB\r\n"))
	_, _, err = ccd.ReadSerialSubmatch(ctx, regexp.MustCompile(`AB\r\n`))
	th.MustSucceed(err, "Error clearing buffer")

	// Send data to UART, expecting to read it out of the USB interface.
	databuf := []byte(fmt.Sprintf("Baud rate %d. UART TX USB RX. The quick red fox jumps over the lazy brown dog", baud))

	th.MustSucceed(uart.WriteSerial(ctx, databuf), "Write error")
	byt, err := ccd.ReadSerialBytes(ctx, len(databuf))
	if err != nil {
		s.Errorf("Data sent to UART did not come out of USB: %s", err)
	} else if !bytes.Equal(byt, databuf) {
		s.Error("Data sent to UART came out of USB corrupted")
		s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
	}

	th.MustSucceed(uart.ClearInput(ctx), "Error clearing buffer")

	// Send data to USB interface, expecting to read it out of the UART.
	databuf = []byte(fmt.Sprintf("Baud rate %d. USB TX UART RX. The quick red fox jumps over the lazy brown dog", baud))

	th.MustSucceed(ccd.WriteSerial(ctx, databuf), "Write error")
	byt, err = uart.ReadSerialBytes(ctx, len(databuf))
	if err != nil {
		s.Errorf("Data sent to USB did not come out of UART: %s", err)
	} else if !bytes.Equal(byt, databuf) {
		s.Error("Data sent to USB came out of UART corrupted")
		s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
	}
}
