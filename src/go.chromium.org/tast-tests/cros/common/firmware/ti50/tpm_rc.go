// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"slices"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

// RCIndexKind identifies whether a Format-1 TPM response code index refers to
// a parameter, a handle, or a session.
type RCIndexKind int

const (
	// RCKindParameter indicates a Format-1 parameter index (1..15).
	RCKindParameter RCIndexKind = iota
	// RCKindHandle indicates a Format-1 handle index (1..7).
	RCKindHandle
	// RCKindSession indicates a Format-1 session index (1..7).
	RCKindSession
)

// RCErr converts a raw tpm2.TPMRC into an error, returning nil when
// rc == tpm2.TPMRCSuccess.
func RCErr(rc tpm2.TPMRC) error {
	if rc == tpm2.TPMRCSuccess {
		return nil
	}
	return rc
}

// RCParam encodes a Format-1 TPM_RC with parameter index idx (1..15).
func RCParam(rc tpm2.TPMRC, idx uint8) tpm2.TPMRC {
	return (rc & 0xBF) | 0x080 | 0x040 | (tpm2.TPMRC(idx&0x0F) << 8)
}

// RCHandle encodes a Format-1 TPM_RC with handle index idx (1..7).
func RCHandle(rc tpm2.TPMRC, idx uint8) tpm2.TPMRC {
	return (rc & 0xBF) | 0x080 | (tpm2.TPMRC(idx&0x07) << 8)
}

// RCSession encodes a Format-1 TPM_RC with session index idx (1..7).
func RCSession(rc tpm2.TPMRC, idx uint8) tpm2.TPMRC {
	return (rc & 0xBF) | 0x080 | 0x800 | (tpm2.TPMRC(idx&0x07) << 8)
}

// IsRC reports whether err is a TPM response code matching one of expected.
// A nil err matches tpm2.TPMRCSuccess. Note that Format-1 errors match their
// canonical base code at any parameter, handle, or session index (via
// tpm2.TPMRC.Is), so e.g. tpm2.TPMRCValue matches 0x184, 0x1C4, 0x2C4, etc.
// To require a specific parameter, handle, or session index, use IsRCAt.
func IsRC(err error, expected ...tpm2.TPMRC) bool {
	if err == nil {
		return slices.Contains(expected, tpm2.TPMRCSuccess)
	}
	return slices.ContainsFunc(expected, func(rc tpm2.TPMRC) bool { return errors.Is(err, rc) })
}

// IsRCAt reports whether err is a Format-1 TPM response code matching canonical
// code rc at the specified parameter, handle, or session 1-based index.
func IsRCAt(err error, rc tpm2.TPMRC, kind RCIndexKind, index int) bool {
	if err == nil || !errors.Is(err, rc) {
		return false
	}
	var fmt1 tpm2.TPMFmt1Error
	if !errors.As(err, &fmt1) {
		return false
	}
	switch kind {
	case RCKindParameter:
		ok, idx := fmt1.Parameter()
		return ok && idx == index
	case RCKindHandle:
		ok, idx := fmt1.Handle()
		return ok && idx == index
	case RCKindSession:
		ok, idx := fmt1.Session()
		return ok && idx == index
	default:
		return false
	}
}

// ExpectError verifies that err is a TPM error matching one of expected.
// If expected is empty, any non-nil, non-warning TPM error is accepted
// (Format-0 warning codes such as TPM_RC_RETRY, TPM_RC_YIELDED, TPM_RC_TESTING,
// TPM_RC_NV_RATE, and TPM_RC_NV_UNAVAILABLE are rejected unless explicitly
// listed in expected).
func ExpectError(err error, expected ...tpm2.TPMRC) error {
	if IsRC(err, expected...) {
		return nil
	}
	if err == nil {
		return errors.Errorf("expected TPM error %v, but command succeeded", expected)
	}
	var rc tpm2.TPMRC
	if !errors.As(err, &rc) {
		return errors.Wrapf(err, "expected TPM error %v, got non-TPM error", expected)
	}
	if len(expected) == 0 {
		if rc.IsWarning() {
			return errors.Wrapf(err, "expected TPM error, got warning response code 0x%x", uint32(rc))
		}
		return nil
	}
	return errors.Wrapf(err, "unexpected TPM response code 0x%x, want one of %v", uint32(rc), expected)
}
