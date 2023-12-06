// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

var consoleUpdateTooSoonRegexp = regexp.MustCompile("Attempted update too soon")
var gsctoolUpdateTooSoonRegexp = regexp.MustCompile(`Error: status 0x9`)
var gsctoolUpdateSuccessRegexp = regexp.MustCompile(`image updated`)
var activeTi50Version = `\* ([0-9.]+/ti50_common[^:]*:v\S*)`
var inactiveTi50Version = `  ([0-9.]+/ti50_common[^:]*:v\S*)`
var versionRwARegexp = regexp.MustCompile(`RW_A:  ` + activeTi50Version)
var versionRwBRegexp = regexp.MustCompile(`RW_B:  ` + activeTi50Version)
var versionRwAInactiveRegexp = regexp.MustCompile(`RW_A:  ` + inactiveTi50Version)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50CCDUpdate,
		Desc:    "Ti50 firmware update over CCD using gsctool",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ecgh@chromium.org",
			"ti50-core@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

// Ti50CCDUpdate requires HW setup with SuzyQ cable from Andreib to drone/workstation.
func Ti50CCDUpdate(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)

	ccdImage, err := prepareCcdImageFile(ctx, s, f.ImagePath)
	if err != nil {
		s.Fatal("Prepare file '", f.ImagePath, "': ", err)
	}

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// Wait for reboot output to finish before reading version.
	s.Log("Simulating insertion of SuzyQ and resetting")
	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	outStr, err := i.Command(ctx, "version")
	if err != nil {
		s.Fatal("Console version: ", err)
	}
	s.Log("Version before 1st gsctool update: ")
	s.Log(outStr)

	if !versionRwARegexp.MatchString(outStr) {
		s.Fatal("Before 1st update, should be running RW_A")
	}

	// Wait one more second to ensure that USB is connected before running gsctool.
	testing.Sleep(ctx, 1*time.Second) // GoBigSleepLint: TODO(b/278113474) fix this

	out, err := b.GSCToolCommand(ctx, "", "--fwver")
	if err != nil {
		// Report the current usb connection state on failure.
		usbOut, err2 := i.Command(ctx, "usb")
		if err2 != nil {
			s.Fatal("Getting usb state: ", err2)
		}
		s.Log("USB state: ", usbOut)
		s.Fatal("Failed to read version: ", err, string(out))
	}

	// Update should succeed within 60 seconds of Power on Reset.
	out, _ = b.GSCToolCommand(ctx, ccdImage)
	if !gsctoolUpdateSuccessRegexp.Match(out) {
		s.Fatalf("Wrong gsctool output for 1st update: %s", out)
	}

	// Wait for reboot output to finish before reading version.
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	outStr, err = i.Command(ctx, "version")
	if err != nil {
		s.Fatal("Console version after 1st update: ", err)
	}
	s.Log("Version after 1st gsctool update: ", outStr)

	if !versionRwBRegexp.MatchString(outStr) {
		s.Fatal("After 1st update, RW_B should be active")
	}

	// Ti50 will reject updates for 60 seconds after an update attempt.
	s.Log("Sleep for 30 seconds to ensure rate limited")
	testing.Sleep(ctx, 30*time.Second) // GoBigSleepLint: Needed for rate limit testing

	out, _ = b.GSCToolCommand(ctx, ccdImage)
	if !gsctoolUpdateTooSoonRegexp.Match(out) {
		s.Fatalf("Wrong gsctool output for update too soon after 1st update: %s", out)
	}

	_, err = b.ReadSerialSubmatch(ctx, consoleUpdateTooSoonRegexp)
	if err != nil {
		s.Fatal("Wrong console message for update too soon after 1st update: ", err)
	}

	s.Log("Sleep for 30 seconds to ensure no longer rate limited")
	testing.Sleep(ctx, 30*time.Second) // GoBigSleepLint: Needed for rate limit testing

	// Ti50 should accept updates 60 seconds after the last update.
	out, _ = b.GSCToolCommand(ctx, ccdImage)
	if !gsctoolUpdateSuccessRegexp.Match(out) {
		s.Fatalf("Wrong gsctool output for 2nd update: %s", out)
	}

	// Wait for reboot output to finish before reading version.
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	outStr, err = i.Command(ctx, "version")
	if err != nil {
		s.Fatal("Console version: ", err)
	}
	s.Log("Version after 2nd gsctool update: ", outStr)

	matchesB := versionRwBRegexp.FindStringSubmatch(outStr)
	if matchesB == nil {
		s.Fatal("After 2nd update, RW_B should still be active because both slots should have the same version")
	}
	matchesA := versionRwAInactiveRegexp.FindStringSubmatch(outStr)
	if matchesA == nil {
		s.Fatal("After 2nd update, RW_A should be inactive because both slots should have the same version")
	}

	if matchesA[1] != matchesB[1] {
		s.Fatalf("After 2nd update, RW_A and RW_B should have the same version: %s != %s", matchesA[1], matchesB[1])
	}
}

// prepareCcdImageFile copies the Ti50 image and zeros the RO signature field.
// Returns the file path to be used by gsctool to perform the CCD update.
func prepareCcdImageFile(ctx context.Context, s *testing.State, image string) (string, error) {
	if image == "" {
		return "", errors.New("no image file")
	}

	f, err := ioutil.TempFile("", "ccd_")
	if err != nil {
		return "", errors.Wrap(err, "create temp image file")
	}
	f.Close()
	ccdImage := f.Name()

	if err := fsutil.CopyFile(image, ccdImage); err != nil {
		return "", errors.Wrap(err, "copy image file")
	}

	// Zero the signature field (offset 4, length 384) in RO_A and RO_B, and
	// zero the cryptolib magic (offset 0, length 4), since we are using node
	// locked RO on Andreiboard for testing (b/230341252).
	for _, base := range []int{0, 0x800, 0x80000, 0x80800} {
		seek := base + 4
		count := 384
		if base == 0x800 || base == 0x80800 {
			seek = base
			count = 4
		}
		cmd := exec.CommandContext(ctx, "dd", fmt.Sprintf("seek=%d", seek), fmt.Sprintf("count=%d", count), "bs=1", "conv=nocreat,notrunc", "if=/dev/zero", "of="+ccdImage)
		if err := cmd.Run(); err != nil {
			return "", errors.Wrap(err, "zero with dd")
		}
	}

	return ccdImage, nil
}
