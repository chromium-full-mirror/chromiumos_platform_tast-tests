// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50TpmI2cCorners,
		Desc:    "Test TPM I2C corner cases",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"ti50-core@google.com",
			"jbk@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50TpmI2cCorners(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	i := ti50.NewCrOSImage(b)
	tpmHandle := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusI2c, ti50.CcdDisconnected, ti50.FfClamshell)

	// Below the low level OpenTitanToolCommand() is used to send I2C transactions in various
	// ways, which do not form valid TPM commands.  If we find that other tests need to
	// perform similar actions, then we should consider how to create proper helper methods,
	// to avoid having to explicitly mention I2C busses on each invocation.

	// Perform irregular I2C transaction, ask for content of status register, but never read
	// the bytes.
	_, err := b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", "80", "raw-write", "--hexdata", "01")
	if err != nil {
		s.Fatal("i2c error: ", err)
	}

	// Now read DIDVID register again.  This should cause previously enqueued status register
	// data to be discarded from the Dauntless I2C fifo.
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID after partial I2C transaction: ", didVid)
	}

	// Perform irregular I2C transaction, ask for other I2C addresses.
	for addr := 0; addr <= 127; addr++ {
		if addr == 0x50 {
			continue
		}
		_, err = b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", strconv.Itoa(addr), "raw-write", "--hexdata", "01")
		if err == nil {
			// No error from opentitantool means that Ti50 ack'ed the address.
			s.Error("Ti50 responded to other address: ", addr)
		}
	}

	// Perform irregular I2C transaction, read more bytes than the size of the register.
	_, err = b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", "80", "raw-write-read", "--hexdata", "01", "--length", "16")
	if err != nil {
		s.Fatal("i2c error: ", err)
	}

	// Check one final time that that Ti50 still respondes to DIDVID register, to make sure it
	// has not crashed or got the I2C driver into a funny state.
	didVid = tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

}
