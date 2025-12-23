// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Cr50Strongbox,
		Desc:    "Test strongbox commands",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Cr50Strongbox(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusSpi, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	sbState, err := i.StrongboxState(ctx)
	th.MustSucceed(err, "failed to get strongbox state")
	if sbState != ti50.StrongboxUnset {
		s.Errorf("Unexpected strongbox state %v: Strongbox set after reset", sbState)
	}
	// Commands should be rejected before Strongbox is enabled.
	sbErr, response, err := utils.StrongboxCommand(ctx, tpm, utils.DeviceGetHardwareInfo, nil)
	if err != nil {
		s.Fatal("Failed HardwareInfo: ", err)
	}
	if sbErr != utils.HardwareNotYetAvailable {
		s.Fatal("Failed HardwareInfo: ", err)
	}
	if len(response) != 0 {
		s.Fatal("Unexpected response")
	}

	th.MustSucceed(tpm.TpmvSetStrongboxState(true), "Enable Strongbox")

	sbState, err = i.StrongboxState(ctx)
	th.MustSucceed(err, "failed to get strongbox state")
	if sbState != ti50.StrongboxEnabled {
		s.Errorf("Unexpected strongbox state %v: Strongbox is not enabled after enable command", sbState)
	}
	err = utils.StrongboxHardwareInfo(ctx, tpm)
	if err != nil {
		s.Fatal("Failed HardwareInfo: ", err)
	}

	err = utils.StrongboxSetHalBootInfo(ctx, tpm, 0x027100, 0x031710, 0x01350245, 0x013502450)
	if err != nil {
		s.Fatal("Failed SetHalBootInfo: ", err)
	}

	diceChain, err := utils.StrongboxGetDiceChain(ctx, tpm)
	if err != nil {
		s.Fatal("Failed GetDiceChain: ", err)
	}
	err = saveFile(ctx, "dice_chain.cbor", diceChain)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	cdiPubKey, err := utils.CheckDiceChainCbor(ctx, diceChain)
	if err != nil {
		s.Fatal("Failed to parse dice chain: ", err)
	}

	rkpBlob, macedKey, err := utils.StrongboxRPCGenerateKey(ctx, tpm)
	if err != nil {
		s.Fatal("Failed RPCGenerateKey: ", err)
	}
	err = saveFile(ctx, "maced_key.cbor", macedKey)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	attestPubKey, err := utils.CheckMacedKeyCbor(macedKey)
	if err != nil {
		s.Fatal("Failed to parse maced key: ", err)
	}

	challenge := []byte("1234567890abcdefghijklmnopqrstuv")
	deviceInfo, _ := hex.DecodeString(
		"AE656272616E6466476F6F676C6565667573656401656D6F64656C656D6F64656C" +
			"66646576696365666465766963656770726F647563746442727961" +
			"6A6F735F76657273696F6E6231376C6D616E756661637475726572" +
			"66476F6F676C656D76626D6574615F6469676573744F112233445566778899AABBCC" +
			"DDEEFF70626F6F745F70617463685F6C6576656C1A013501927273797374656D5F70" +
			"617463685F6C6576656C1A013501917276656E646F725F70617463685F6C6576656C" +
			"1A01350193" +
			"6E73656375726974795F6C6576656C697374726F6E67626F78" + // "security_level" : "strongbox"
			"6876625F737461746565677265656E" + // "vb_state":"green"
			"70626F6F746C6F616465725F7374617465666C6F636B6564") // "bootloader_state":"locked"
	csr, err := utils.StrongboxRPCGenerateCertificate(ctx, tpm, macedKey, challenge, deviceInfo)
	if err != nil {
		s.Fatal("Failed RPCGenerateCertificate: ", err)
	}
	err = saveFile(ctx, "csr.cbor", csr)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	err = utils.CheckCsrCbor(ctx, csr, cdiPubKey)
	if err != nil {
		s.Fatal("Failed to parse CSR: ", err)
	}

	// Test without attestation key
	generateAndTestKey(ctx, s, tpm, nil, nil, "self")

	// Test with attestation key
	generateAndTestKey(ctx, s, tpm, rkpBlob, attestPubKey, "attest")

	th.MustSucceed(tpm.TpmvSetStrongboxState(false), "Disable Strongbox")
	sbState, err = i.StrongboxState(ctx)
	th.MustSucceed(err, "failed to get strongbox state")
	if sbState != ti50.StrongboxDisabled {
		s.Errorf("Unexpected strongbox state %v: Strongbox is not disabled after disabled command", sbState)
	}
	err = tpm.TpmvSetStrongboxState(true)
	if err == nil {
		s.Fatal("Enable after disable should fail")
	}

	sbState, err = i.StrongboxState(ctx)
	th.MustSucceed(err, "failed to get strongbox state")
	if sbState != ti50.StrongboxDisabled {
		s.Errorf("Unexpected strongbox state %v: Strongbox disable was cleared", sbState)
	}
}

func saveFile(ctx context.Context, filename string, contents []byte) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get directory")
	}
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "failed to create file `%s`", path)
	}
	defer f.Close()
	if _, err := f.Write(contents); err != nil {
		return errors.Wrapf(err, "failed to write data to %s", path)
	}
	return nil
}

func generateAndTestKey(ctx context.Context, s *testing.State, tpm *utils.TpmHelper, attestKey []byte, attestPubKey *ecdsa.PublicKey, label string) {
	blob, cert, err := utils.StrongboxGenerateKey(ctx, tpm, attestKey)
	if err != nil {
		s.Fatal("Failed GenerateKey: ", err)
	}
	err = saveFile(ctx, fmt.Sprintf("cert_%s.der", label), cert)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	cert2, err := x509.ParseCertificate(cert)
	if err != nil {
		s.Fatal("Failed to parse certificate: ", err)
	}
	pubKey, ok := cert2.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		s.Fatal("Public key is not an ECDSA key")
	}
	if attestPubKey == nil {
		attestPubKey = pubKey
	}
	h := sha256.Sum256(cert2.RawTBSCertificate)
	valid := ecdsa.VerifyASN1(attestPubKey, h[:], cert2.Signature)
	if !valid {
		s.Fatal("Failed to verify certificate")
	}

	operationID, err := utils.StrongboxBegin(ctx, tpm, blob)
	if err != nil {
		s.Fatal("Failed begin: ", err)
	}

	input := []byte("0123456789ABCDEF0123456789ABCDEF")
	err = utils.StrongboxUpdate(ctx, tpm, operationID, input)
	if err != nil {
		s.Fatal("Failed update: ", err)
	}

	sig, err := utils.StrongboxFinish(ctx, tpm, operationID, nil)
	if err != nil {
		s.Fatal("Failed finish: ", err)
	}
	h = sha256.Sum256(input)
	err = utils.CheckSignature(ctx, "finish", pubKey, h[:], sig)
	if err != nil {
		s.Fatal("Failed finish: ", err)
	}
}
