// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// TPMA_PERMANENT bit definitions (TPM 2.0 Part 2 Section 8.2).
	tpmaPermanentOwnerAuthSet       uint32 = 1 << 0
	tpmaPermanentEndorsementAuthSet uint32 = 1 << 1
	tpmaPermanentLockoutAuthSet     uint32 = 1 << 2
	tpmaPermanentDisableClear       uint32 = 1 << 8
	tpmaPermanentInLockout          uint32 = 1 << 9
	tpmaPermanentAuthOrDisableMask  uint32 = tpmaPermanentOwnerAuthSet |
		tpmaPermanentEndorsementAuthSet |
		tpmaPermanentLockoutAuthSet |
		tpmaPermanentDisableClear

	// Default GSC Dictionary Attack parameters.
	defaultDAMaxTries        uint32 = 200
	defaultDARecoveryTime    uint32 = 0
	defaultDALockoutRecovery uint32 = 0
)

// FlushHandle flushes the given handle on the TPM, ignoring any errors so it
// can be used directly with defer.
func (t *TpmHandle) FlushHandle(handle tpm2.TPMHandle) {
	if t == nil || handle == 0 || handle == tpm2.TPMRHNull {
		return
	}
	_, _ = (tpm2.FlushContext{FlushHandle: handle}).Execute(t)
}

// ListHandles queries TPM_CAP_HANDLES starting at startHandle and returns all
// matching handles of the same handle type (or session handles in 0x02/0x03
// when querying TPM_HT_LOADED_SESSION / TPM_HT_SAVED_SESSION).
func (t *TpmHandle) ListHandles(startHandle tpm2.TPMHandle) ([]tpm2.TPMHandle, error) {
	var all []tpm2.TPMHandle
	ht := uint8(startHandle >> 24)
	basePrefix := uint32(startHandle) & 0xFF000000
	isSess := ht == 0x02 || ht == 0x03
	next := uint32(startHandle)
	for {
		getCapability := tpm2.GetCapability{
			Capability:    tpm2.TPMCapHandles,
			Property:      next,
			PropertyCount: 64,
		}
		resp, err := getCapability.Execute(t)
		if err != nil {
			return all, errors.Wrap(err, "failed to query TPM_CAP_HANDLES")
		}
		hList, err := resp.CapabilityData.Data.Handles()
		if err != nil {
			return all, errors.Wrap(err, "failed to unpack TPM_CAP_HANDLES")
		}
		if len(hList.Handle) == 0 {
			break
		}
		var lastIdx uint32
		for _, h := range hList.Handle {
			curHT := uint8(h >> 24)
			if isSess {
				if curHT != 0x02 && curHT != 0x03 {
					return all, nil
				}
			} else if curHT != ht {
				return all, nil
			}
			all = append(all, h)
			lastIdx = uint32(h) & 0x00FFFFFF
		}
		nextCandidate := basePrefix | (lastIdx + 1)
		if !resp.MoreData || nextCandidate <= next {
			break
		}
		next = nextCandidate
	}
	return all, nil
}

// FlushAllTransientsAndSessions flushes all loaded transient objects (0x80000000),
// active HMAC sessions (0x02000000), and policy/saved sessions (0x03000000).
func (t *TpmHandle) FlushAllTransientsAndSessions() {
	if t == nil {
		return
	}
	for _, base := range []tpm2.TPMHandle{0x80000000, 0x02000000, 0x03000000} {
		handles, err := t.ListHandles(base)
		if err != nil {
			continue
		}
		for _, h := range handles {
			t.FlushHandle(h)
		}
	}
}

// isTestPersistentHandle reports whether h is within the persistent handle
// ranges reserved for TCG compliance tests.
func isTestPersistentHandle(h tpm2.TPMHandle) bool {
	switch {
	case h >= 0x81000000 && h <= 0x810003FF:
		return true
	case h >= 0x81800000 && h <= 0x818000FF:
		return true
	case h == 0x81AA00AA:
		return true
	default:
		return false
	}
}

// EvictAllPersistent evicts persistent objects in the test-reserved persistent
// handle ranges, selecting Owner auth for 0x81000000..0x817FFFFF and Platform
// auth for 0x81800000..0x81FFFFFF.
func (t *TpmHandle) EvictAllPersistent() {
	if t == nil {
		return
	}
	handles, err := t.ListHandles(0x81000000)
	if err != nil {
		return
	}
	for _, h := range handles {
		if !isTestPersistentHandle(h) {
			continue
		}
		auth := tpm2.TPMRHOwner
		if h >= 0x81800000 {
			auth = tpm2.TPMRHPlatform
		}
		evictControl := tpm2.EvictControl{
			Auth:             auth,
			ObjectHandle:     tpm2.NamedHandle{Handle: h, Name: PlaceholderEntityName()},
			PersistentHandle: h,
		}
		_, _ = evictControl.Execute(t)
	}
}

// ResetLockout resets the TPM Dictionary Attack lockout counter using empty
// Lockout password authorization.
func (t *TpmHandle) ResetLockout() error {
	return t.ResetLockoutWithAuth(PasswordHandle(tpm2.TPMRHLockout, tpm2.TPM2BName{}, nil))
}

// ResetLockoutWithAuth resets the TPM Dictionary Attack lockout counter using
// the specified Lockout authorization handle/session.
func (t *TpmHandle) ResetLockoutWithAuth(auth tpm2.AuthHandle) error {
	if auth.Handle == 0 {
		auth.Handle = tpm2.TPMRHLockout
	}
	if _, err := t.ExecSessionCommand(tpm2.TPMCCDictionaryAttackLockReset, auth, nil); err != nil {
		return errors.Wrap(err, "TPM2_DictionaryAttackLockReset")
	}
	return nil
}

// ClearTPM enables Clear via Platform authorization, executes TPM2_Clear with
// Platform or Lockout authorization, and resets the Dictionary Attack lockout
// counter.
func (t *TpmHandle) ClearTPM() error {
	t.FlushAllTransientsAndSessions()

	var errs []error
	if err := t.ClearControl(PasswordHandle(tpm2.TPMRHPlatform, tpm2.TPM2BName{}, nil), false); err != nil {
		errs = append(errs, err)
	}
	for _, auth := range []tpm2.TPMHandle{tpm2.TPMRHPlatform, tpm2.TPMRHLockout} {
		clearCmd := tpm2.Clear{
			AuthHandle: auth,
		}
		_, err := clearCmd.Execute(t)
		if err == nil {
			_ = t.ResetLockout()
			return nil
		}
		errs = append(errs, errors.Wrapf(err, "TPM2_Clear(0x%x)", uint32(auth)))
	}
	return errors.Join(errs...)
}

// EntityCleanup flushes transient objects and sessions, recovers Dictionary
// Attack lockout and hierarchy authorization/policies, restores GSC's default
// Dictionary Attack parameters (200, 0, 0), evicts persistent test handles, and
// undefines test NV spaces.
func (t *TpmHandle) EntityCleanup() error {
	if t == nil {
		return nil
	}
	if t.Config == nil {
		return errors.New("EntityCleanup requires initialized TpmHandle.Config")
	}
	t.FlushAllTransientsAndSessions()

	var errs []error
	needClear := false
	disableClearSet := false
	inLockout := false
	lockoutAuthSet := false
	if perm, err := t.GetTPMProperty(tpm2.TPMPTPermanent); err != nil {
		errs = append(errs, err)
	} else {
		needClear = perm&tpmaPermanentAuthOrDisableMask != 0
		disableClearSet = perm&tpmaPermanentDisableClear != 0
		inLockout = perm&tpmaPermanentInLockout != 0
		lockoutAuthSet = perm&tpmaPermanentLockoutAuthSet != 0
	}

	// Only attempt ResetLockout with an empty password when lockoutAuth is not
	// set; calling ResetLockout with an empty password when lockoutAuthSet is 1
	// would trigger TPM_RC_AUTH_FAIL on TPM_RH_LOCKOUT and disable lockoutAuth.
	if inLockout && !lockoutAuthSet {
		if err := t.ResetLockout(); err != nil {
			if IsRC(err, tpm2.TPMRCLockout, tpm2.TPMRCAuthFail, tpm2.TPMRCBadAuth) {
				needClear = true
			} else {
				errs = append(errs, err)
			}
		}
	}

	if !needClear && t.Config.IsCommandSupported(tpm2.TPMCCSetPrimaryPolicy) {
		for _, h := range []tpm2.TPMHandle{tpm2.TPMRHOwner, tpm2.TPMRHEndorsement, tpm2.TPMRHLockout} {
			err := t.SetPrimaryPolicy(PasswordHandle(h, tpm2.TPM2BName{}, nil), nil, tpm2.TPMAlgNull)
			if IsRC(err, tpm2.TPMRCAuthFail, tpm2.TPMRCBadAuth, tpm2.TPMRCLockout) {
				needClear = true
				break
			}
		}
	}

	if (disableClearSet || needClear) && t.Config.IsCommandSupported(tpm2.TPMCCClearControl) {
		if err := t.ClearControl(PasswordHandle(tpm2.TPMRHPlatform, tpm2.TPM2BName{}, nil), false); err != nil {
			errs = append(errs, err)
		}
	}
	if needClear && t.Config.IsCommandSupported(tpm2.TPMCCClear) {
		if t.Ctx != nil {
			testing.ContextLog(t.Ctx, "EntityCleanup: executing TPM2_Clear(Platform) to restore hierarchy state")
		}
		clearCmd := tpm2.Clear{AuthHandle: tpm2.TPMRHPlatform}
		if _, err := clearCmd.Execute(t); err != nil {
			errs = append(errs, errors.Wrap(err, "TPM2_Clear(Platform)"))
		}
		if err := t.ResetLockout(); err != nil {
			errs = append(errs, err)
		}
		// GSC's ms-tpm-20-ref fork disables clearing hierarchy policy digests in
		// Clear.c (#if 0), so explicitly reset primary policies after Clear.
		if t.Config.IsCommandSupported(tpm2.TPMCCSetPrimaryPolicy) {
			for _, h := range []tpm2.TPMHandle{tpm2.TPMRHOwner, tpm2.TPMRHEndorsement, tpm2.TPMRHLockout} {
				if err := t.SetPrimaryPolicy(PasswordHandle(h, tpm2.TPM2BName{}, nil), nil, tpm2.TPMAlgNull); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	if t.Config.IsCommandSupported(tpm2.TPMCCDictionaryAttackParameters) {
		if err := t.DictionaryAttackParameters(PasswordHandle(tpm2.TPMRHLockout, tpm2.TPM2BName{}, nil), defaultDAMaxTries, defaultDARecoveryTime, defaultDALockoutRecovery); err != nil {
			errs = append(errs, err)
		}
	}
	t.EvictAllPersistent()
	t.UndefineAllTestNVSpaces()
	return errors.Join(errs...)
}
