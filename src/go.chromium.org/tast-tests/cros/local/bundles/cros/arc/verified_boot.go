// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	vendorImgPath    = "/opt/google/vms/android/vendor.raw.img"
	systemImgPath    = "/opt/google/vms/android/system.raw.img"
	vbmetaDigestPath = "/opt/google/vms/android/arcvm_vbmeta_digest.sha256"

	vbootLogPath = "/var/log/debug_vboot_noisy.log"
	bootKeyRegex = "bios::GBB::root_key::sha1_sum::(\\w+)"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     VerifiedBoot,
		Desc:     "Validate vboot log format and value for vbmeta digest",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		Fixture:      "arcBootedWithoutUIAutomator",
		// TODO update dep to android_vm after KeyMint is launched on V+ in b/308630124
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
		VarDeps:      []string{},
	})
}

func VerifiedBoot(ctx context.Context, s *testing.State) {
	if err := validateVbmetaDigest(ctx); err != nil {
		s.Fatal("Error while validating vb meta digest: ", err)
	}

	if err := validateVbootLogFormat(ctx); err != nil {
		s.Fatal("Error while validating vboot log format: ", err)
	}
}

// validateVbmetaDigest calculates the vbmeta digest, in the same way that it is
// calculated during image build/signing. As long as no code has been changed on
// the device, the calculated vbmeta digest will match the stored value in
// arcvm_vbmeta_digest.sha256.
func validateVbmetaDigest(ctx context.Context) error {
	// Calculate the hashes of the system and vendor images.
	vendorImgHash, err := calculateHash(ctx, vendorImgPath)
	if err != nil {
		return errors.Wrap(err, "failed to get sha256sum of the vendor image")
	}
	testing.ContextLog(ctx, "Vendor image hash: "+vendorImgHash)
	systemImgHash, err := calculateHash(ctx, systemImgPath)
	if err != nil {
		return errors.Wrap(err, "failed to get sha256sum of the system image")
	}
	testing.ContextLog(ctx, "System image hash: "+systemImgHash)

	// Concatenate the hashes and store the result in a tmp file.
	combinedHash := systemImgHash + vendorImgHash
	path := filepath.Join("/tmp", "combined_hash")
	if err = os.WriteFile(path, []byte(combinedHash), 0644); err != nil {
		return errors.Wrap(err, "failed to write combined hash to tmp file")
	}
	defer os.Remove(path)

	// Calculate the hash of the concatenated hashes to get the vbmeta digest.
	actualDigest, err := calculateHash(ctx, path)
	if err != nil {
		return errors.Wrap(err, "failed to get sha256sum of the combined hashes")
	}
	testing.ContextLog(ctx, "Actual vbmeta digest: "+actualDigest)

	// Retrieve the value for vbmeta digest that was calculated during image build/signing
	// and is used for verified boot/attestation.
	vbmetaDigest, err := os.ReadFile(vbmetaDigestPath)
	if err != nil {
		return errors.Wrap(err, "failed to read arcvm_vbmeta_digest.sha256")
	}
	expectedDigest := string(vbmetaDigest)
	testing.ContextLog(ctx, "Expected vbmeta digest: "+expectedDigest)

	// Compare the values. This will fail if new code was pushed to the device for development.
	if expectedDigest != actualDigest {
		return errors.New("Calculated and expected values for VB meta digest do not match")
	}

	return nil
}

// calculateHash returns the sha256sum of the file at the given path.
func calculateHash(ctx context.Context, path string) (string, error) {
	// Sample output:
	// 		4b1093909c91c70fd7d054481abf02c9b3c0d6057a6579654b207587e4e0e831 *system.raw.img
	sha256sumOutput, err := testexec.CommandContext(ctx, "sha256sum", "-b", path).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	// Extract only the hash value from sha256sum output.
	outString := string(sha256sumOutput)
	hash := outString[:strings.Index(string(outString), " ")]
	return hash, nil
}

// validateVbootLogFormat confirms that the verified boot key can be properly extracted
// from the vboot log. This value does not need to be compared with the actual value
// since it is already validated in arc.AtttestationCertificate.
func validateVbootLogFormat(ctx context.Context) error {
	vbootLog, err := os.ReadFile(vbootLogPath)
	if err != nil {
		return errors.Wrap(err, "failed to read "+vbootLogPath)
	}

	re := regexp.MustCompile(bootKeyRegex)
	regexMatch := re.FindStringSubmatch(string(vbootLog))
	if regexMatch == nil {
		return errors.New("failed to find vboot key in " + vbootLogPath)
	}
	testing.ContextLog(ctx, "Verified boot key from log: "+regexMatch[1])
	return nil
}
