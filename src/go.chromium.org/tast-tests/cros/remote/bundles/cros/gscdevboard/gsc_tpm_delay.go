// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const bootParams tpm2.TPMHandle = 0x013fff0a

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTPMDelay,
		Desc:    "Test a delay between TPM command and reading response",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_ot_shield", "gsc_image_ti50"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func GSCTPMDelay(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)

	nh, ds, err := nvReadPublic(tpm, bootParams)
	if err != nil {
		s.Fatal("Failed NVReadPublic: ", err)
	}
	readCmd := tpm2.NVRead{
		AuthHandle: ti50.RootPlatformHandle,
		NVIndex:    nh,
		Size:       ds,
	}
	// Test a 200ms delay between sending command and reading response.
	tpmw := &tpmWrapper{ctx, *tpm, 200 * time.Millisecond, true}
	readResp, err := readCmd.Execute(tpmw)
	if err != nil {
		s.Fatal("Failed NVRead: ", err)
	}
	d := readResp.Data.Buffer
	if len(d) != int(ds) {
		s.Fatalf("Wrong data size (got %d want %d)", len(d), ds)
	}
	s.Log("Read success with delay")

	// Send command but don't read response.
	tpmw.readResponse = false
	_, err = readCmd.Execute(tpmw)
	if err == nil {
		s.Fatal("Missing response didn't fail as expected: ", err)
	}

	// Send another command and read response.
	tpmw.readResponse = true
	readResp, err = readCmd.Execute(tpmw)
	d = readResp.Data.Buffer
	if len(d) != int(ds) {
		s.Fatalf("Wrong data size (got %d want %d)", len(d), ds)
	}
	s.Log("Read success with dropped response")
}

func nvReadPublic(tpm *utils.TpmHelper, h tpm2.TPMHandle) (tpm2.NamedHandle, uint16, error) {
	readPublicCmd := tpm2.NVReadPublic{NVIndex: h}
	readPublicResp, err := readPublicCmd.Execute(tpm)
	if err != nil {
		return tpm2.NamedHandle{}, 0, err
	}
	pub, err := readPublicResp.NVPublic.Contents()
	if err != nil {
		return tpm2.NamedHandle{}, 0, err
	}
	return tpm2.NamedHandle{
		Handle: h,
		Name:   readPublicResp.NVName,
	}, pub.DataSize, nil
}

type tpmStsValue uint32

const (
	stsBitDataAvail   tpmStsValue = 0x00000010
	stsBitGo          tpmStsValue = 0x00000020
	stsBitCmdReady    tpmStsValue = 0x00000040
	stsBitValid       tpmStsValue = 0x00000080
	stsMaskBurstCount tpmStsValue = 0x00ffff00
	// (SELF_TEST_DONE VALID BURST_COUNT=63 FAMILY=TPM_2_0)
	stsValueIdle tpmStsValue = 0x04003f84
	// (SELF_TEST_DONE COMMAND_READY VALID BURST_COUNT=63 FAMILY=TPM_2_0)
	stsValueCmdReady tpmStsValue = 0x04003fc4
)

type tpmWrapper struct {
	ctx context.Context
	utils.TpmHelper
	delayAfterCommand time.Duration
	readResponse      bool
}

func (t *tpmWrapper) Send(request []byte) ([]byte, error) {
	t.WriteSts(stsBitCmdReady)
	if s := t.ReadSts(); s != stsValueCmdReady {
		return nil, errors.Errorf("expected stsValueCmdReady, got %v", s)
	}
	t.TpmHelper.WriteRegister(ti50.TpmRegDataFifo, request)
	t.WriteSts(stsBitGo)
	// GoBigSleepLint sleep for delay required by test.
	testing.Sleep(t.ctx, t.delayAfterCommand)
	if !t.readResponse {
		return nil, nil
	}
	var resp []byte
	expectLength := 0
	for expectLength == 0 || len(resp) < expectLength {
		c := t.ReadDataAvail()
		if c == 0 {
			continue
		}
		if expectLength > 0 {
			c = min(c, expectLength-len(resp))
		}
		d := t.TpmHelper.ReadRegisterWithLength(ti50.TpmRegDataFifo, c)
		resp = append(resp, d...)
		expectLength = int(binary.BigEndian.Uint32(resp[2:6]))
	}
	return resp, nil
}

func (t *tpmWrapper) ReadSts() tpmStsValue {
	d := t.TpmHelper.ReadRegister(ti50.TpmRegSts)
	s := binary.LittleEndian.Uint32(d)
	return tpmStsValue(s)
}

func (t *tpmWrapper) WriteSts(s tpmStsValue) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(s))
	t.TpmHelper.WriteRegister(ti50.TpmRegSts, bytes)
}

func (t *tpmWrapper) ReadDataAvail() int {
	s := t.ReadSts()
	if s&stsBitValid == 0 || s&stsBitDataAvail == 0 {
		return 0
	}
	c := s & stsMaskBurstCount
	return int(c >> 8)
}
