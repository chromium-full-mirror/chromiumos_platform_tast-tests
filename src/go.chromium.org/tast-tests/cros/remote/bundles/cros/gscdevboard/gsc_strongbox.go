// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCStrongbox,
		Desc:    "Test strongbox commands",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_nightly"},
		Params: []testing.Param{{
			Name:      "cr50",
			Fixture:   fixture.GSCOpenCCD,
			ExtraAttr: []string{"gsc_image_ti50", "gsc_h1_shield"},
		}, {
			Name:      "ti50a",
			Fixture:   fixture.GSCOpenCCDTi50a,
			ExtraAttr: []string{"gsc_image_ti50a", "gsc_dt_shield", "gsc_ot_shield"},
		}},
	})
}

func GSCStrongbox(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusSpi, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(tpm.TpmvSetStrongboxState(true), "Enable Strongbox")

	err := utils.StrongboxHardwareInfo(ctx, tpm)
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

	rkpBlob1, macedKey1, attestPubKey1 := strongboxRPCGenerateKey(ctx, s, tpm, "maced_key1")
	_, macedKey2, _ := strongboxRPCGenerateKey(ctx, s, tpm, "maced_key2")

	challenge := []byte("1234567890abcdefghijklmnopqrstuv")
	deviceInfo := utils.DeviceInfo{
		Brand:            "Google",
		Fused:            1,
		Model:            "model",
		Device:           "device",
		Product:          "Brya",
		OSVersion:        "17",
		Manufacturer:     "Google",
		VBMetaDigest:     "112233445566778899AABBCCDDEEFF",
		BootPatchLevel:   20251026,
		SystemPatchLevel: 20251025,
		VendorPatchLevel: 20251027,
		SecurityLevel:    "strongbox",
		VBState:          "green",
		BootloaderState:  "locked",
	}
	strongboxRPCGenerateCertificate(ctx, s, tpm, nil, challenge, deviceInfo, cdiPubKey, "csr0")
	strongboxRPCGenerateCertificate(ctx, s, tpm, [][]byte{macedKey1}, challenge, deviceInfo, cdiPubKey, "csr1")
	strongboxRPCGenerateCertificate(ctx, s, tpm, [][]byte{macedKey1, macedKey2}, challenge, deviceInfo, cdiPubKey, "csr2")

	// Test without attestation key
	generateAndTestKey(ctx, s, tpm, nil, nil, "self")

	// Test with attestation key
	generateAndTestKey(ctx, s, tpm, rkpBlob1, attestPubKey1, "attest")
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

func strongboxRPCGenerateKey(ctx context.Context, s *testing.State, tpm *utils.TpmHelper, label string) (rkpBlob, macedKey []byte, attestPubKey *ecdsa.PublicKey) {
	rkpBlob, macedKey, err := utils.StrongboxRPCGenerateKey(ctx, tpm)
	if err != nil {
		s.Fatal("Failed RPCGenerateKey: ", err)
	}
	err = saveFile(ctx, label+".cbor", macedKey)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	attestPubKey, err = utils.CheckMacedKeyCbor(macedKey)
	if err != nil {
		s.Fatal("Failed to parse maced key: ", err)
	}
	return
}

func strongboxRPCGenerateCertificate(ctx context.Context, s *testing.State, tpm *utils.TpmHelper, macedKeys [][]byte, challenge []byte, deviceInfo utils.DeviceInfo, cdiPubKey *ecdsa.PublicKey, label string) {
	deviceInfoCbor := deviceInfo.ToCBOR()
	csr, err := utils.StrongboxRPCGenerateCertificate(ctx, tpm, macedKeys, challenge, deviceInfoCbor)
	if err != nil {
		s.Fatal("Failed RPCGenerateCertificate: ", err)
	}
	err = saveFile(ctx, label+".cbor", csr)
	if err != nil {
		s.Fatal("Failed to save file: ", err)
	}
	challenge2, deviceInfo2, keysCount, err := utils.CheckCsrCbor(ctx, csr, cdiPubKey)
	if err != nil {
		s.Fatal("Failed to parse CSR: ", err)
	}
	if !bytes.Equal(challenge, challenge2) {
		s.Fatal("Wrong challenge in CSR")
	}
	if len(macedKeys) != keysCount {
		s.Fatal("Wrong number of keys in CSR")
	}
	deviceInfo.VBState = "orange"
	if !bytes.Equal(deviceInfo.ToCBOR(), deviceInfo2) {
		s.Fatal("Wrong device info in CSR")
	}
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
