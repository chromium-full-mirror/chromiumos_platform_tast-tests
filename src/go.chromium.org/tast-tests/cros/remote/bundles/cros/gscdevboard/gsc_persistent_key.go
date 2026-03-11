// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

// The persistent handle for the salting key defined by trunks.
const saltingKey tpm2.TPMHandle = 0x81000002

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCPersistentKey,
		Desc:    "Test a persistent key can be imported from older version",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_he", "gsc_image_ti50"},
		Fixture:      fixture.GSCOpenCCD,
		Data:         []string{"cross_version_login_r112_flash_bank0.bin", "cross_version_login_r112_flash_bank1.bin"},
	})
}

// GSCPersistentKey checks that we can import a persistent key created by an
// older GSC version. This is host_emulation only since we can set the flash
// contents of the emulator. The flash contents are taken from the
// hwsec.CrossVersionChromeLogin.ti50_r112 test that runs on the betty VM with
// ti50-emulator. The StartAuthSession TPM command will import the salting key
// from the data in the filesystem. This is similar to what trunks does on
// ChromeOS boot (StartSession).
func GSCPersistentKey(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	err := b.EmulatorWriteFile(ctx, s.DataPath("cross_version_login_r112_flash_bank0.bin"), "flash_bank0")
	if err != nil {
		s.Fatal("Failed to write emulator file: ", err)
	}
	err = b.EmulatorWriteFile(ctx, s.DataPath("cross_version_login_r112_flash_bank1.bin"), "flash_bank1")
	if err != nil {
		s.Fatal("Failed to write emulator file: ", err)
	}

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)

	readPublicCmd := tpm2.ReadPublic{ObjectHandle: saltingKey}
	readPublicResp, err := readPublicCmd.Execute(tpm)
	if err != nil {
		s.Fatal("Failed to read public of persistent salting key: ", err)
	}

	public, err := readPublicResp.OutPublic.Contents()
	if err != nil {
		s.Fatal("Failed to get public contents: ", err)
	}
	key, err := tpm2.ImportEncapsulationKey(public)
	if err != nil {
		s.Fatal("Failed to import encapsulation key: ", err)
	}
	_, encSalt, err := tpm2.CreateEncryptedSalt(rand.Reader, key)
	if err != nil {
		s.Fatal("Failed to create encrypted salt: ", err)
	}
	nonceCaller := make([]byte, 20)
	if _, err := rand.Read(nonceCaller); err != nil {
		s.Fatal("Failed to generate nonceCaller: ", err)
	}

	sasResp, err := tpm2.StartAuthSession{
		TPMKey:        saltingKey,
		Bind:          tpm2.TPMRHNull,
		NonceCaller:   tpm2.TPM2BNonce{Buffer: nonceCaller},
		EncryptedSalt: tpm2.TPM2BEncryptedSecret{Buffer: encSalt},
		SessionType:   tpm2.TPMSEHMAC,
		Symmetric: tpm2.TPMTSymDef{
			Algorithm: tpm2.TPMAlgAES,
			KeyBits: tpm2.NewTPMUSymKeyBits(
				tpm2.TPMAlgAES,
				tpm2.TPMKeyBits(128),
			),
			Mode: tpm2.NewTPMUSymMode(
				tpm2.TPMAlgAES,
				tpm2.TPMAlgCFB,
			),
		},
		AuthHash: tpm2.TPMAlgSHA256,
	}.Execute(tpm)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}

	if _, err := (tpm2.FlushContext{FlushHandle: sasResp.SessionHandle}).Execute(tpm); err != nil {
		s.Log("Failed to flush auth session: ", err)
	}
}
