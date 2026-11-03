// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

func TestBuildRawCommandAndAuthArea(t *testing.T) {
	pw1 := []byte("alpha")
	pw2 := []byte("beta")
	s1 := BuildPasswordAuthSession(pw1)
	s2 := BuildPasswordAuthSession(pw2)
	wrapped := WrapAuthArea(s1, s2)
	direct := BuildPasswordAuthArea(pw1, pw2)
	if !bytes.Equal(wrapped, direct) {
		t.Fatalf("WrapAuthArea(%x) != BuildPasswordAuthArea(%x)", wrapped, direct)
	}
	wantBodyLen := (9 + len(pw1)) + (9 + len(pw2))
	gotBodyLen := int(binary.BigEndian.Uint32(direct[:4]))
	if gotBodyLen != wantBodyLen {
		t.Fatalf("authArea size prefix = %d, want %d", gotBodyLen, wantBodyLen)
	}

	handles := []tpm2.TPMHandle{tpm2.TPMRHOwner, 0x80000001}
	params := []byte{0x11, 0x22, 0x33}
	raw := BuildRawCommand(tpm2.TPMSTSessions, tpm2.TPMCCClear, handles, direct, params)
	wantSize := 10 + 4*len(handles) + len(direct) + len(params)
	if len(raw) != wantSize {
		t.Fatalf("len(raw) = %d, want %d", len(raw), wantSize)
	}
	if gotTag := tpm2.TPMST(binary.BigEndian.Uint16(raw[0:2])); gotTag != tpm2.TPMSTSessions {
		t.Fatalf("tag = 0x%x, want 0x%x", uint16(gotTag), uint16(tpm2.TPMSTSessions))
	}
	if gotSize := int(binary.BigEndian.Uint32(raw[2:6])); gotSize != wantSize {
		t.Fatalf("header size = %d, want %d", gotSize, wantSize)
	}
	if gotCC := tpm2.TPMCC(binary.BigEndian.Uint32(raw[6:10])); gotCC != tpm2.TPMCCClear {
		t.Fatalf("cc = 0x%x, want 0x%x", uint32(gotCC), uint32(tpm2.TPMCCClear))
	}
}

func TestIsRCAndIsRCAt(t *testing.T) {
	if !IsRC(nil, tpm2.TPMRCSuccess) {
		t.Fatal("IsRC(nil, TPMRCSuccess) = false, want true")
	}
	if IsRC(nil, tpm2.TPMRCValue) {
		t.Fatal("IsRC(nil, TPMRCValue) = true, want false")
	}

	paramErr := RCParam(tpm2.TPMRCValue, 2)
	handleErr := RCHandle(tpm2.TPMRCHandle, 1)
	sessErr := RCSession(tpm2.TPMRCAuthFail, 3)

	if !IsRC(paramErr, tpm2.TPMRCValue) {
		t.Fatal("IsRC(RCParam(TPMRCValue, 2), TPMRCValue) = false, want true")
	}
	if !IsRCAt(paramErr, tpm2.TPMRCValue, RCKindParameter, 2) {
		t.Fatal("IsRCAt(paramErr, TPMRCValue, RCKindParameter, 2) = false, want true")
	}
	if IsRCAt(paramErr, tpm2.TPMRCValue, RCKindParameter, 1) {
		t.Fatal("IsRCAt(paramErr, TPMRCValue, RCKindParameter, 1) = true, want false")
	}
	if IsRCAt(paramErr, tpm2.TPMRCValue, RCKindHandle, 2) {
		t.Fatal("IsRCAt(paramErr, TPMRCValue, RCKindHandle, 2) = true, want false")
	}
	if !IsRCAt(handleErr, tpm2.TPMRCHandle, RCKindHandle, 1) {
		t.Fatal("IsRCAt(handleErr, TPMRCHandle, RCKindHandle, 1) = false, want true")
	}
	if !IsRCAt(sessErr, tpm2.TPMRCAuthFail, RCKindSession, 3) {
		t.Fatal("IsRCAt(sessErr, TPMRCAuthFail, RCKindSession, 3) = false, want true")
	}
	if RCErr(tpm2.TPMRCSuccess) != nil {
		t.Fatal("RCErr(TPMRCSuccess) != nil")
	}
}

func TestExpectError(t *testing.T) {
	if err := ExpectError(nil, tpm2.TPMRCValue); err == nil {
		t.Fatal("ExpectError(nil, TPMRCValue) succeeded, want error")
	}
	if err := ExpectError(errors.New("transport broken")); err == nil {
		t.Fatal("ExpectError(non-TPM error) succeeded, want error")
	}
	if err := ExpectError(tpm2.TPMRCRetry); err == nil {
		t.Fatal("ExpectError(TPMRCRetry warning) with empty expected succeeded, want error")
	}
	if err := ExpectError(tpm2.TPMRCRetry, tpm2.TPMRCRetry); err != nil {
		t.Fatalf("ExpectError(TPMRCRetry, TPMRCRetry) failed: %v", err)
	}
	if err := ExpectError(RCParam(tpm2.TPMRCSize, 1)); err != nil {
		t.Fatalf("ExpectError(RCParam(TPMRCSize, 1)) failed: %v", err)
	}
}

func TestAllowLists(t *testing.T) {
	allowedNV := []tpm2.TPMHandle{0x01000000, 0x01000FFF, 0x01500000, 0x01500123, 0x01800020, 0x01800FFF}
	for _, h := range allowedNV {
		if !isTestNVIndex(h) {
			t.Errorf("isTestNVIndex(0x%x) = false, want true", uint32(h))
		}
	}
	deniedNV := []tpm2.TPMHandle{KernelNvIndex, FwmpNvIndex, EncstatefulNvIndex, WideVineRotIndex, 0x01C00002, 0x01800000}
	for _, h := range deniedNV {
		if isTestNVIndex(h) {
			t.Errorf("isTestNVIndex(0x%x) = true, want false", uint32(h))
		}
	}

	allowedPersist := []tpm2.TPMHandle{0x81000000, 0x810003FF, 0x81800000, 0x818000FF, 0x81AA00AA}
	for _, h := range allowedPersist {
		if !isTestPersistentHandle(h) {
			t.Errorf("isTestPersistentHandle(0x%x) = false, want true", uint32(h))
		}
	}
	deniedPersist := []tpm2.TPMHandle{0x81010001, 0x81008001, 0x817FFFFF, 0x80000000}
	for _, h := range deniedPersist {
		if isTestPersistentHandle(h) {
			t.Errorf("isTestPersistentHandle(0x%x) = true, want false", uint32(h))
		}
	}
}

func TestPolicyDigests(t *testing.T) {
	avDigest, err := PolicyAuthValueDigest(tpm2.TPMAlgSHA256)
	if err != nil {
		t.Fatalf("PolicyAuthValueDigest failed: %v", err)
	}
	ccDigest, err := PolicyCommandCodeDigest(tpm2.TPMAlgSHA256, tpm2.TPMCCPolicyPassword)
	if err != nil {
		t.Fatalf("PolicyCommandCodeDigest(TPMCCPolicyPassword) failed: %v", err)
	}
	if bytes.Equal(avDigest, ccDigest) {
		t.Fatal("PolicyAuthValueDigest and PolicyCommandCodeDigest(TPMCCPolicyPassword) must differ")
	}
}

func TestSensitiveToDuplicateValidation(t *testing.T) {
	tmpl := AESCFBTemplate(tpm2.TPMAlgSHA256, 128)
	key := make([]byte, 16)
	seed := make([]byte, 32)
	pub, sens, err := SoftwareSymmetricObject(tmpl, []byte("auth"), seed, key)
	if err != nil {
		t.Fatalf("SoftwareSymmetricObject failed: %v", err)
	}

	// Unencrypted duplicate.
	dup, encSeed, err := SensitiveToDuplicate(sens, pub, nil, nil, nil)
	if err != nil || len(dup) == 0 || len(encSeed) != 0 {
		t.Fatalf("unencrypted SensitiveToDuplicate failed: len(dup)=%d len(encSeed)=%d err=%v", len(dup), len(encSeed), err)
	}

	// encryptionKey without symDef must fail.
	if _, _, err := SensitiveToDuplicate(sens, pub, nil, key, nil); err == nil {
		t.Fatal("SensitiveToDuplicate with encryptionKey and nil symDef succeeded, want error")
	}

	// Mismatched encryptionKey length must fail.
	symDef256 := tpm2.TPMTSymDefObject{
		Algorithm: tpm2.TPMAlgAES,
		KeyBits:   tpm2.NewTPMUSymKeyBits(tpm2.TPMAlgAES, tpm2.TPMKeyBits(256)),
		Mode:      tpm2.NewTPMUSymMode(tpm2.TPMAlgAES, tpm2.TPMAlgCFB),
	}
	if _, _, err := SensitiveToDuplicate(sens, pub, nil, key, &symDef256); err == nil {
		t.Fatal("SensitiveToDuplicate with 128-bit key and 256-bit symDef succeeded, want error")
	}
}

func TestTPMCommandName(t *testing.T) {
	if got := tpmCommandName(tpm2.TPMCCCreatePrimary); got != "TPM2_CreatePrimary" {
		t.Fatalf("tpmCommandName(TPMCCCreatePrimary) = %q, want TPM2_CreatePrimary", got)
	}
	if got := tpmCommandName(tpm2.TPMCC(0x20000123)); got != "TPM_CC(0x20000123)" {
		t.Fatalf("tpmCommandName(0x20000123) = %q, want TPM_CC(0x20000123)", got)
	}
}

type fakeTPMDevBoard struct {
	DevBoard
	rc tpm2.TPMRC
}

func (f *fakeTPMDevBoard) OpenTitanToolCommand(_ context.Context, _ string, _ ...string) (map[string]interface{}, error) {
	var resp []byte
	if f.rc == tpm2.TPMRCSuccess {
		// TPM_ST_SESSIONS (0x8002), size 19, TPM_RC_SUCCESS (0), parameterSize 0,
		// and a 5-byte empty password TPMS_AUTH_RESPONSE (nonceTPM=0, attrs=1, hmac=0).
		resp = []byte{
			0x80, 0x02,
			0x00, 0x00, 0x00, 0x13,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x01, 0x00, 0x00,
		}
	} else {
		resp = make([]byte, 10)
		binary.BigEndian.PutUint16(resp[0:2], uint16(tpm2.TPMSTNoSessions))
		binary.BigEndian.PutUint32(resp[2:6], 10)
		binary.BigEndian.PutUint32(resp[6:10], uint32(f.rc))
	}
	return map[string]interface{}{"hexdata": hex.EncodeToString(resp)}, nil
}

func TestSessionCommandWrappersErrorHandling(t *testing.T) {
	board := &fakeTPMDevBoard{rc: tpm2.TPMRCSuccess}
	h := NewTpmHandle(context.Background(), board, TpmBusSpi)
	auth := PasswordHandle(tpm2.TPMRHPlatform, tpm2.TPM2BName{}, nil)

	calls := []struct {
		name string
		fn   func() error
	}{
		{"ClearControl", func() error { return h.ClearControl(auth, false) }},
		{"DictionaryAttackParameters", func() error { return h.DictionaryAttackParameters(auth, 200, 0, 0) }},
		{"ClockSet", func() error { return h.ClockSet(auth, 1000) }},
		{"SetPrimaryPolicy", func() error { return h.SetPrimaryPolicy(auth, nil, tpm2.TPMAlgNull) }},
		{"HierarchyControl", func() error { return h.HierarchyControl(auth, tpm2.TPMRHOwner, true) }},
		{"ChangePPS", func() error { return h.ChangePPS(auth) }},
		{"ChangeEPS", func() error { return h.ChangeEPS(auth) }},
		{"SetCommandCodeAuditStatus", func() error { return h.SetCommandCodeAuditStatus(auth, tpm2.TPMAlgSHA256, nil, nil) }},
		{"ResetLockoutWithAuth", func() error { return h.ResetLockoutWithAuth(auth) }},
	}

	for _, tc := range calls {
		board.rc = tpm2.TPMRCSuccess
		if err := tc.fn(); err != nil {
			t.Errorf("%s returned error on TPM_RC_SUCCESS: %v", tc.name, err)
		}
		board.rc = tpm2.TPMRCAuthFail
		if err := tc.fn(); !IsRC(err, tpm2.TPMRCAuthFail) {
			t.Errorf("%s error = %v, want TPM_RC_AUTH_FAIL", tc.name, err)
		}
	}
}

func TestKDFaAndComputeHashHMAC(t *testing.T) {
	if got := HashSize(tpm2.TPMAlgSHA256); got != 32 {
		t.Fatalf("HashSize(TPMAlgSHA256) = %d, want 32", got)
	}
	if got := HashSize(tpm2.TPMAlgNull); got != 0 {
		t.Fatalf("HashSize(TPMAlgNull) = %d, want 0", got)
	}

	d1, err := ComputeHash(tpm2.TPMAlgSHA256, []byte("hello "), []byte("world"))
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	d2, err := ComputeHash(tpm2.TPMAlgSHA256, []byte("hello world"))
	if err != nil || !bytes.Equal(d1, d2) {
		t.Fatalf("ComputeHash(chunks) = %x, want %x (err=%v)", d1, d2, err)
	}
	if _, err := ComputeHash(tpm2.TPMAlgNull, []byte("x")); err == nil {
		t.Fatal("ComputeHash(TPMAlgNull) succeeded, want error")
	}

	key := []byte("0123456789abcdef")
	m1, err := ComputeHMAC(tpm2.TPMAlgSHA256, key, []byte("hello "), []byte("world"))
	if err != nil {
		t.Fatalf("ComputeHMAC failed: %v", err)
	}
	m2, err := ComputeHMAC(tpm2.TPMAlgSHA256, key, []byte("hello world"))
	if err != nil || !bytes.Equal(m1, m2) {
		t.Fatalf("ComputeHMAC(chunks) = %x, want %x (err=%v)", m1, m2, err)
	}
	if _, err := ComputeHMAC(tpm2.TPMAlgNull, key, []byte("x")); err == nil {
		t.Fatal("ComputeHMAC(TPMAlgNull) succeeded, want error")
	}

	ctxU := []byte("contextU")
	ctxV := []byte("contextV")
	derived, err := KDFa(tpm2.TPMAlgSHA256, key, "STORAGE", ctxU, ctxV, 128)
	if err != nil {
		t.Fatalf("KDFa(TPMAlgSHA256) failed: %v", err)
	}
	if len(derived) != 16 {
		t.Fatalf("len(derived) = %d, want 16", len(derived))
	}
	h, err := tpm2.TPMAlgSHA256.Hash()
	if err != nil {
		t.Fatalf("TPMAlgSHA256.Hash() failed: %v", err)
	}
	wantDerived := tpm2.KDFa(h, key, "STORAGE", ctxU, ctxV, 128)
	if !bytes.Equal(derived, wantDerived) {
		t.Fatalf("KDFa = %x, want %x", derived, wantDerived)
	}
	if _, err := KDFa(tpm2.TPMAlgNull, key, "STORAGE", ctxU, ctxV, 128); err == nil {
		t.Fatal("KDFa(TPMAlgNull) succeeded, want error")
	}
}

func TestTamperedUniqueAndSeedOverride(t *testing.T) {
	t.Setenv("TPM_TEST_SEED", "123456789")
	if got := defaultTestSeed(); got != 123456789 {
		t.Fatalf("defaultTestSeed() with TPM_TEST_SEED=123456789 = %d, want 123456789", got)
	}

	emptyRSA := RSASSADefaultTemplate(tpm2.TPMAlgSHA256, tpm2.TPMAlgSHA256, 2048)
	if _, err := tamperedUnique(emptyRSA, func(b []byte) []byte { return b }, nil); err == nil {
		t.Fatal("tamperedUnique(empty RSA Unique) succeeded, want error")
	}
}
