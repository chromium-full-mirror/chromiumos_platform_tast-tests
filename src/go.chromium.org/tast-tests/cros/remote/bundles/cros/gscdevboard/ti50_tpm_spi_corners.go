// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50TpmSpiCorners,
		Desc:    "Test TPM SPI corner cases",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"ti50-core@google.com",
			"jbk@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50TpmSpiCorners(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	i := ti50.NewCrOSImage(b)
	tpmHandle := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdDisconnected, ti50.FfClamshell)

	// Perform irregular SPI TPM transaction, ask for content of status register, but never
	// read the bytes.
	_, err := b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write", "--hexdata", "C3D4001800")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Now read DIDVID register again.  This should cause previously enqueued status register
	// data to be discarded from the Dauntless SPI fifo.
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID after partial SPI transaction: ", didVid)
	}

	// Perform irregular SPI TPM transaction, truncated.
	_, err = b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write", "--hexdata", "C3D4")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Perform irregular SPI TPM transaction, read more bytes than the size of the register.
	_, err = b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write-read", "--hexdata", "C3D4001800", "--length", "16")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Check one final time that that Ti50 still respondes to DIDVID register, to make sure it
	// has not crashed or got the SPI driver into a funny state.
	didVid = tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

}
