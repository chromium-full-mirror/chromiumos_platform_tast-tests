// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// Get AP RO digest
	aproDigestRE = regexp.MustCompile(`digest: ([a-f0-9]+)\b`)
)

const (
	// Command that initializes the AP RO hash data with two regions.
	setAPROHash string = "80010000003c200000000036b0863a864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e0000c100000800000010c10000f00e00"
	// expectedHash is the hash the previous command sets
	expectedHash string = "b0863a864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e"
	// erasedHash is the gsctool output when the hash is erased
	erasedHash string = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	// Command that initializes a different AP RO hash.
	setOtherAPROHash string = "80010000003c200000000036aaaaaa864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e0000c100000800000010c10000f00e00"
	// Cr50 won't clear the hash for 10 seconds after it's triggered.
	aPROHashClearedDelay time.Duration = time.Second * 10
	extraDelay           time.Duration = time.Second * 10

	// expectedStartResult is NOT_RUN(0)
	expectedStartResult = "apro result (0)"
	// expectedDefaultTriggeredResult is 5 UNSUPPORTED_TRIGGERED
	expectedTriggeredResult = "apro result (5)"

	// unsupportedBIDType is a board id type that blocks AP RO verification
	unsupportedBIDType = 0x5256504a
	// cr50APROTestFlags are flags that should work on all test images
	cr50APROTestFlags = 0x37f7f
)

type cr50APROTestConfig struct {
	sendTPMCmd  bool
	expectClear bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Cr50APROVerificationClearResult,
		Desc:    "Verify Cr50 clears the AP RO verification result after the device is reset",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCInitialFactory,
		Params: []testing.Param{{
			Name: "tpm_activity",
			Val: cr50APROTestConfig{
				sendTPMCmd:  true,
				expectClear: true,
			},
		}, {
			Name: "no_tpm_activity",
			Val: cr50APROTestConfig{
				sendTPMCmd:  false,
				expectClear: false,
			},
		}},
	})
}

// Cr50APROVerificationClearResult verifies Cr50 clears the AP RO verificaiton result after the device is reset.
func Cr50APROVerificationClearResult(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	config := s.Param().(cr50APROTestConfig)

	bus := ti50.TpmBusSpi
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")
	b.WaitUntilCCDConnected(ctx)

	th.MustSucceed(i.CCDOpen(ctx), "ccd open")
	th.MustSucceed(i.CCDResetFactory(ctx), "reset factory")

	out, err := b.GSCToolCommandViaTPM(ctx, bus, "", "--erase_ap_ro_hash")
	th.MustSucceed(err, "Could not erase the hash")
	s.Logf("GSCTool output: %s", out)

	out, err = b.GSCToolCommand(ctx, "", "--get_apro_hash")
	th.MustSucceed(err, "Could not get the hash")
	digest, err := parseCr50APRODigest(out)
	th.MustSucceed(err, "Could not parse the hash")
	s.Logf("saved hash: %s", digest)
	if digest != erasedHash {
		s.Fatalf("Digest not erased: got %s", digest)
	}
	// Set the AP RO hash
	setHash, err := hex.DecodeString(setAPROHash)
	th.MustSucceed(err, "Failed to decode command")
	_, err = tpm.Send(setHash)
	th.MustSucceed(err, "Failed to set hash")

	// Set an unsupported BID, so verification won't run. This test just
	// verifies the result is cleared after the AP resets. It doesn't
	// run verification.
	err = tpm.TpmvSetBoardID(unsupportedBIDType, cr50APROTestFlags)
	th.MustSucceed(err, "Failed to set board id")

	_, err = b.GSCToolCommandViaTPM(ctx, bus, "", "--erase_ap_ro_hash")
	if err == nil {
		s.Fatal("Cleared hash after the board id was set")
	}
	setOtherHash, err := hex.DecodeString(setOtherAPROHash)
	th.MustSucceed(err, "Failed to decode command")
	_, err = tpm.Send(setOtherHash)
	th.MustSucceed(err, "Failed to run set AP RO hash command")

	out, err = b.GSCToolCommand(ctx, "", "--get_apro_hash")
	th.MustSucceed(err, "Could not get the hash")
	digest, err = parseCr50APRODigest(out)
	th.MustSucceed(err, "Could not parse the hash")
	s.Logf("saved hash: %s", digest)
	if digest != expectedHash {
		s.Fatalf("Saved incorrect digest: expected %q got %q", expectedHash, digest)
	}

	out, err = b.GSCToolCommand(ctx, "", "--apro_boot")
	th.MustSucceed(err, "Could not get status")
	s.Logf("out: %s", out)
	if !strings.Contains(string(out), expectedStartResult) {
		s.Fatalf("Unexpected result after setup: expected %q got %q", expectedStartResult, out)
	}

	out, err = b.GSCToolCommand(ctx, "", "--get_apro_hash")
	th.MustSucceed(err, "Could not get the hash")
	digest, err = parseCr50APRODigest(out)
	th.MustSucceed(err, "Could not parse the hash")
	s.Logf("saved hash: %s", digest)
	if digest != expectedHash {
		s.Fatalf("Saved incorrect digest: expected %q got %q", expectedHash, digest)
	}

	out, err = b.GSCToolCommandViaTPM(ctx, bus, "", "--apro_boot", "start")
	th.MustSucceed(err, "Could not trigger verification")
	startTime := time.Now()
	s.Logf("started verification: %s", out)

	out, err = b.GSCToolCommand(ctx, "", "--apro_boot")
	th.MustSucceed(err, "Could not get verification status")
	s.Logf("out: %s", out)
	if !strings.Contains(string(out), expectedTriggeredResult) {
		s.Fatalf("Unexpected result after verification was triggered: expected %q got %q", expectedTriggeredResult, out)
	}
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: aPROHashClearedDelay + extraDelay}
	err = testing.Poll(ctx, func(ctx context.Context) error {
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
		if config.sendTPMCmd {
			// Cr50 debounces TPM_RST_L pulses. It won't process
			// multiple TPM_RST_L pulses unless there's some TPM
			// activity.
			// Call WaitForTpm to send the TPM_STS get DID VID
			// command, so Cr50 will process the next reset.
			b.WaitForTpm(ctx, tpm)
		}

		out, err := b.GSCToolCommand(ctx, "", "--apro_boot")
		th.MustSucceed(err, "Could not get verification status")
		s.Logf("out: %s", out)
		if strings.Contains(string(out), expectedStartResult) {
			s.Log("APRO result was cleared")
			return nil
		}
		return errors.Errorf("APRO info still shows triggered: %s", out)
	}, &pOpts)

	if !config.expectClear {
		if err == nil {
			s.Fatal("APRO result was cleared when it shouldn't have been")
		}
		return
	}

	if err != nil {
		s.Fatal("APRO result was not cleared")
	}

	clearedTime := time.Since(startTime)
	s.Log("APRO result was cleared in ", clearedTime)
	if clearedTime < aPROHashClearedDelay {
		s.Fatal("APRO result was cleared too soon ", clearedTime)
	}
}

func parseCr50APRODigest(out []byte) (string, error) {
	matches := aproDigestRE.FindStringSubmatch(string(out))
	if len(matches) != 2 {
		return "", errors.Errorf("failed to find digest in %q", out)
	}
	return matches[1], nil
}
