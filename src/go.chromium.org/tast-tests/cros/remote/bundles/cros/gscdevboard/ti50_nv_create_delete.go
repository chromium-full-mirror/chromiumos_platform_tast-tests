// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/google/go-tpm/tpmutil"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const fileID tpmutil.Handle = 0x100100F

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50NvCreateDelete,
		Desc:    "Creates, deletes, and recreates NVs on the GSC",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"granaghan@google.com",     // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Ti50NvCreateDelete(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)
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
	// Clear NV space if it exists to start from a clean place. Ignore the error in case it was
	// already clean.
	tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, fileID)

	if err := tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		fileID,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		size,
	); err != nil {
		return errors.Join(errors.New("NVDefineSpace failed: "), err)
	}

	resp, err := tpm2.NVReadEx(tpm,
		fileID,
		ti50.RootPlatformHandle,
		ti50.EmptyPassword,
		0,
	)
	// tpm2/legacy doesn't wrap errors, so errors.Is() does not work. We'll have to use this
	// workaround until migrating to the new tpm2.
	if !strings.Contains(err.Error(), "NV Index is used before being initialized") {
		return errors.Join(errors.New("NVRead returned unexpected error: "), err)
	}

	data := make([]byte, size)
	for i := range data {
		data[i] = fill
	}
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		fileID,
		ti50.EmptyPassword,
		data,
		0,
	); err != nil {
		return errors.Join(errors.New("NVWrite failed: "), err)
	}

	resp, err = tpm2.NVReadEx(tpm,
		fileID,
		ti50.RootPlatformHandle,
		ti50.EmptyPassword,
		0,
	)
	if err != nil {
		return errors.Join(errors.New("NVRead failed: "), err)
	}
	if !bytes.Equal(resp, data) {
		return errors.Errorf("Read mismatch: %v", resp)
	}

	if err := tpm2.NVUndefineSpace(tpm,
		ti50.EmptyPassword,
		ti50.RootPlatformHandle,
		fileID,
	); err != nil {
		return errors.Join(errors.New("Undefine failed: "), err)
	}

	return nil
}
