// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const fileID tpm2.TPMHandle = 0x100100F

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCNVCreateDelete,
		Desc:    "Creates, deletes, and recreates NVs on the GSC",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"granaghan@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCNVCreateDelete(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)
	err := tpm.TpmvCommitNvmem()
	if err != nil {
		s.Fatal("Failed to enable Nvmem writes: ", err)
	}

	err = testNvSpace(b, i, tpm, 40, 0x11)
	if err != nil {
		s.Fatal("testNvSpace failed: ", err)
	}

	err = testNvSpace(b, i, tpm, 13, 0x22)
	if err != nil {
		s.Fatal("testNvSpace failed: ", err)
	}

	err = testNvSpace(b, i, tpm, 51, 0x33)
	if err != nil {
		s.Fatal("testNvSpace failed: ", err)
	}
}

// testNvSpace creates an NV space with the given size, fills it with fill, checks that it can be
// read, and checks that it can be deleted.
func testNvSpace(b utils.DevboardHelper, i *ti50.CrOSImage, tpm *utils.TpmHelper, size uint16, fill byte) error {
	attr := tpm2.TPMSNVPublic{
		NVIndex: fileID,
		NameAlg: tpm2.TPMAlgSHA1,
		Attributes: tpm2.TPMANV{
			PlatformCreate: true,
			AuthRead:       true,
			PPRead:         true,
			WriteSTClear:   true,
			PPWrite:        true,
			NT:             tpm2.TPMNTOrdinary,
		},
		DataSize: size,
	}

	// Attempt to clean up state in case the NV was already defined.
	tpm.NvUndefineSpace(attr)

	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	if _, err := def.Execute(tpm); err != nil {
		return errors.Join(errors.New("NVDefineSpace failed: "), err)
	}

	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		return errors.Join(errors.New("failed to get NV name: "), err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: fileID,
		Name:   *nvName,
	}

	read := tpm2.NVRead{
		AuthHandle: ti50.RootPlatformHandle,
		NVIndex:    nvHandle,
		Size:       size,
	}
	_, err = read.Execute(tpm)
	if err != tpm2.TPMRCNVUninitialized {
		return errors.Join(errors.New("unexpected NVRead response: "), err)
	}

	data := make([]byte, size)
	for i := range data {
		data[i] = fill
	}
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: data,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		return errors.Join(errors.New("NVWrite failed: "), err)
	}

	resp, err := read.Execute(tpm)
	if err != nil {
		return errors.Join(errors.New("NVRead failed: "), err)
	}
	if !bytes.Equal(resp.Data.Buffer, data) {
		return errors.Errorf("Read mismatch: %v", resp.Data.Buffer)
	}

	if err := tpm.NvUndefineSpace(attr); err != nil {
		return errors.Join(errors.New("NVUndefineSpace failed: "), err)
	}

	return nil
}
