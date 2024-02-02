// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type ti50PcrParam struct {
	shutdownType tpm2.TPMSU
	startupType  tpm2.TPMSU
	sleep        bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCPCRRestore,
		Desc:    "Test PCR restore behavior coming out of deep sleep",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"granaghan@google.com",     // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "state_state_sleep",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUState,
				startupType:  tpm2.TPMSUState,
				sleep:        true,
			},
		}, {
			Name: "state_state_pulse",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUState,
				startupType:  tpm2.TPMSUState,
				sleep:        false,
			},
		}, {
			Name: "clear_clear_sleep",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUClear,
				startupType:  tpm2.TPMSUClear,
				sleep:        true,
			},
		}, {
			Name: "clear_clear_pulse",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUClear,
				startupType:  tpm2.TPMSUClear,
				sleep:        false,
			},
		}, {
			Name: "state_clear_sleep",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUState,
				startupType:  tpm2.TPMSUClear,
				sleep:        true,
			},
		}, {
			Name: "state_clear_pulse",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUState,
				startupType:  tpm2.TPMSUClear,
				sleep:        false,
			},
		}, {
			Name: "clear_state_sleep",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUClear,
				startupType:  tpm2.TPMSUState,
				sleep:        true,
			},
		}, {
			Name: "clear_state_pulse",
			Val: ti50PcrParam{
				shutdownType: tpm2.TPMSUClear,
				startupType:  tpm2.TPMSUState,
				sleep:        false,
			},
		}},
	})
}

func GSCPCRRestore(ctx context.Context, s *testing.State) {
	zeroPcr := bytes.Repeat([]byte{0x00}, sha256.Size)
	testParams := s.Param().(ti50PcrParam)
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("Restarting ti50 with appropriate straps")
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CcdDisconnected, ti50.FfClamshell)

	pcrRead := tpm2.PCRRead{
		PCRSelectionIn: tpm2.TPMLPCRSelection{
			PCRSelections: []tpm2.TPMSPCRSelection{
				{
					Hash:      tpm2.TPMAlgSHA256,
					PCRSelect: []byte{0x01, 0x00, 0x00},
				},
			},
		},
	}
	read0, err := pcrRead.Execute(tpm)
	if err != nil {
		s.Fatal("PCRRead failed: ", err)
	}
	if bytes.Compare(read0.PCRValues.Digests[0].Buffer, zeroPcr) != 0 {
		s.Error("PCR not zero on reset: ", read0.PCRValues)
	}

	authHandle := tpm2.AuthHandle{
		Handle: tpm2.TPMHandle(0),
		Auth:   tpm2.PasswordAuth(nil),
	}
	pcrExtend := tpm2.PCRExtend{
		PCRHandle: authHandle,
		Digests: tpm2.TPMLDigestValues{
			Digests: []tpm2.TPMTHA{
				{
					HashAlg: tpm2.TPMAlgSHA256,
					Digest:  bytes.Repeat([]byte{0x01}, sha256.Size),
				},
			},
		},
	}
	if _, err := pcrExtend.Execute(tpm); err != nil {
		s.Fatal("PCRExtend failed: ", err)
	}

	read1, err := pcrRead.Execute(tpm)
	if err != nil {
		s.Fatal("PCRRead failed: ", err)
	}

	shutdown := tpm2.Shutdown{
		ShutdownType: testParams.shutdownType,
	}
	if _, err := shutdown.Execute(tpm); err != nil {
		s.Fatal("Shutdown failed: ", err)
	}

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	if testParams.sleep {
		s.Log("Waiting for Ti50 to go into deep sleep")
		th.MustSucceed(i.WaitUntilDeepSleep(ctx, 2*time.Minute), "Ti50 did not sleep when AP off")
	}
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	if testParams.sleep {
		if err := i.WaitUntilBooted(ctx); err != nil {
			s.Error("Ti50 did not wake up: ", err)
		}
	}

	startup := tpm2.Startup{
		StartupType: testParams.startupType,
	}
	_, err = startup.Execute(tpm)
	// Shutdown with clear and startup with state is expected to fail.
	if testParams.shutdownType == tpm2.TPMSUClear && testParams.startupType == tpm2.TPMSUState {
		if !errors.Is(err, tpm2.TPMRCValue) {
			s.Error("Unexpected error: ", err)
		}
		return
	} else if err != nil {
		s.Fatal("Startup failed: ", err)
	}

	read2, err := pcrRead.Execute(tpm)
	if err != nil {
		s.Fatal("PCRRead failed: ", err)
	}

	if testParams.startupType == tpm2.TPMSUState {
		if bytes.Compare(read1.PCRValues.Digests[0].Buffer, read2.PCRValues.Digests[0].Buffer) != 0 {
			s.Error("PCRs not reloaded properly: ", read1.PCRValues, read2.PCRValues)
		}
	} else {
		if bytes.Compare(read2.PCRValues.Digests[0].Buffer, zeroPcr) != 0 {
			s.Error("PCRs not cleared: ", read2.PCRValues)
		}
	}
}
