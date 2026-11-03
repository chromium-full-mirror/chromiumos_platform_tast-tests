// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

// DefaultNVAttributes returns standard TPMA_NV attributes for an NV index
// created under authHandle (Platform or Owner) with the given NV type (TPM_NT_*).
func DefaultNVAttributes(authHandle tpm2.TPMHandle, nvType tpm2.TPMNT) tpm2.TPMANV {
	if authHandle == tpm2.TPMRHPlatform {
		return tpm2.TPMANV{
			PPWrite:        true,
			PPRead:         true,
			AuthRead:       true,
			AuthWrite:      true,
			PlatformCreate: true,
			NT:             nvType,
		}
	}
	return tpm2.TPMANV{
		OwnerWrite: true,
		OwnerRead:  true,
		AuthRead:   true,
		AuthWrite:  true,
		NT:         nvType,
	}
}

// NVNamedHandle queries TPM2_NV_ReadPublic for nvIndex and returns a
// tpm2.NamedHandle with its current Name.
func (t *TpmHandle) NVNamedHandle(nvIndex tpm2.TPMHandle) (tpm2.NamedHandle, error) {
	nvReadPublic := tpm2.NVReadPublic{
		NVIndex: nvIndex,
	}
	pubResp, err := nvReadPublic.Execute(t)
	if err != nil {
		return tpm2.NamedHandle{}, errors.Wrapf(err, "NVReadPublic failed for 0x%x", uint32(nvIndex))
	}
	return tpm2.NamedHandle{
		Handle: nvIndex,
		Name:   pubResp.NVName,
	}, nil
}

// NVAuthHandle queries TPM2_NV_ReadPublic for nvIndex and returns a
// tpm2.AuthHandle bound to auth (defaulting to PasswordAuth(nil) if auth is nil).
func (t *TpmHandle) NVAuthHandle(nvIndex tpm2.TPMHandle, auth tpm2.Session) (tpm2.AuthHandle, error) {
	nh, err := t.NVNamedHandle(nvIndex)
	if err != nil {
		return tpm2.AuthHandle{}, err
	}
	if auth == nil {
		auth = tpm2.PasswordAuth(nil)
	}
	return tpm2.AuthHandle{
		Handle: nh.Handle,
		Name:   nh.Name,
		Auth:   auth,
	}, nil
}

// DefineNVSpace defines an NV index in one of the allowed test NV index ranges
// (undefining any pre-existing index at nvIndex first).
func (t *TpmHandle) DefineNVSpace(authHandle, nvIndex tpm2.TPMHandle, nameAlg tpm2.TPMAlgID, attrs tpm2.TPMANV, policy []byte, size uint16, authValue []byte) error {
	if !isTestNVIndex(nvIndex) {
		return errors.Errorf("nvIndex 0x%x is outside the allowed test NV index ranges", uint32(nvIndex))
	}
	t.ForceUndefineNVSpace(nvIndex)
	nvDefineSpace := tpm2.NVDefineSpace{
		AuthHandle: authHandle,
		Auth:       tpm2.TPM2BAuth{Buffer: authValue},
		PublicInfo: tpm2.New2B(tpm2.TPMSNVPublic{
			NVIndex:    nvIndex,
			NameAlg:    nameAlg,
			Attributes: attrs,
			AuthPolicy: tpm2.TPM2BDigest{Buffer: policy},
			DataSize:   size,
		}),
	}
	_, err := nvDefineSpace.Execute(t)
	return err
}

// UndefineNVSpace undefines nvIndex using the specified authHandle (e.g.
// TPMRHOwner or TPMRHPlatform).
func (t *TpmHandle) UndefineNVSpace(authHandle, nvIndex tpm2.TPMHandle) error {
	nvUndefineSpace := tpm2.NVUndefineSpace{
		AuthHandle: authHandle,
		NVIndex: tpm2.NamedHandle{
			Handle: nvIndex,
			Name:   PlaceholderEntityName(),
		},
	}
	_, err := nvUndefineSpace.Execute(t)
	return err
}

// ForceUndefineNVSpace attempts to undefine nvIndex using both Owner and Platform
// hierarchies (falling back to TPM2_NV_UndefineSpaceSpecial for PolicyDelete
// spaces), ignoring any errors (intended for defer cleanup).
func (t *TpmHandle) ForceUndefineNVSpace(nvIndex tpm2.TPMHandle) {
	if t == nil || nvIndex == 0 {
		return
	}
	err := t.UndefineNVSpace(tpm2.TPMRHOwner, nvIndex)
	if err == nil || IsRC(err, tpm2.TPMRCHandle) {
		return
	}
	err = t.UndefineNVSpace(tpm2.TPMRHPlatform, nvIndex)
	if err == nil || IsRC(err, tpm2.TPMRCHandle) || !IsRC(err, tpm2.TPMRCAttributes) {
		return
	}
	if t.Config != nil && !t.Config.IsCommandSupported(tpm2.TPMCCNVUndefineSpaceSpecial) {
		return
	}
	nvReadPublic := tpm2.NVReadPublic{NVIndex: nvIndex}
	pubResp, err := nvReadPublic.Execute(t)
	if err != nil {
		return
	}
	pub, err := pubResp.NVPublic.Contents()
	if err != nil || !pub.Attributes.PolicyDelete {
		return
	}
	for _, withPassword := range []bool{false, true} {
		var opts []tpm2.AuthOption
		if withPassword {
			opts = append(opts, t.PolicyPasswordAuthOption(nil))
		}
		sess, closeSess, err := t.PolicyCommandCodeSession(pub.NameAlg, 16, tpm2.TPMCCNVUndefineSpaceSpecial, opts...)
		if err != nil {
			return
		}
		if withPassword {
			if err := t.PolicyPasswordOrAuthValue(sess.Handle()); err != nil {
				closeSess()
				return
			}
		}
		undefSpecial := tpm2.NVUndefineSpaceSpecial{
			NVIndex:  SessionHandle(nvIndex, pubResp.NVName, sess),
			Platform: tpm2.TPMRHPlatform,
		}
		_, err = undefSpecial.Execute(t)
		closeSess()
		if err == nil || !IsRC(err, tpm2.TPMRCPolicyFail) {
			return
		}
	}
}

// isTestNVIndex reports whether h is within the explicit NV index ranges used
// by TPM compliance and functional tests, ensuring system NV spaces (such as
// KernelNvIndex 0x01001008, FwmpNvIndex 0x0100100a, EncstatefulNvIndex
// 0x01800005, WideVineRotIndex 0x013fff05, and EK certs 0x01c00000+) are never
// touched by test helpers.
func isTestNVIndex(h tpm2.TPMHandle) bool {
	return (h >= 0x01000000 && h <= 0x01000FFF) ||
		(h >= 0x01500000 && h <= 0x01500FFF) ||
		(h >= 0x01800020 && h <= 0x01800FFF)
}

// UndefineAllTestNVSpaces undefines any lingering test NV indices in the
// allowed test NV index ranges.
func (t *TpmHandle) UndefineAllTestNVSpaces() {
	if t == nil {
		return
	}
	handles, err := t.ListHandles(nvIndexFirst)
	if err != nil {
		return
	}
	for _, h := range handles {
		if isTestNVIndex(h) {
			t.ForceUndefineNVSpace(h)
		}
	}
}

// SafeNVWrite writes data to nvIndex starting at offset, splitting the write
// across chunks of at most maxBufSize bytes and refreshing the NV index Name
// after each chunk (since setting TPMA_NV_WRITTEN changes the index Name).
func (t *TpmHandle) SafeNVWrite(authHandle tpm2.AuthHandle, nvIndex tpm2.TPMHandle, data []byte, offset, maxBufSize uint16) error {
	if maxBufSize == 0 {
		maxBufSize = uint16(t.Config.MaxNVBufferSize())
	}
	if authHandle.Auth == nil {
		authHandle.Auth = tpm2.PasswordAuth(nil)
	}

	remaining := data
	curOffset := offset
	for {
		chunkLen := len(remaining)
		if chunkLen > int(maxBufSize) {
			chunkLen = int(maxBufSize)
		}
		nh, err := t.NVNamedHandle(nvIndex)
		if err != nil {
			return err
		}
		if authHandle.Handle == nvIndex {
			authHandle.Name = nh.Name
		}
		nvWrite := tpm2.NVWrite{
			AuthHandle: authHandle,
			NVIndex:    nh,
			Data:       tpm2.TPM2BMaxNVBuffer{Buffer: remaining[:chunkLen]},
			Offset:     curOffset,
		}
		if _, err := nvWrite.Execute(t); err != nil {
			return err
		}
		remaining = remaining[chunkLen:]
		curOffset += uint16(chunkLen)
		if len(remaining) == 0 {
			break
		}
	}
	return nil
}

// SafeNVRead reads size bytes from nvIndex starting at offset, splitting the read
// across chunks of at most maxBufSize bytes.
func (t *TpmHandle) SafeNVRead(authHandle tpm2.AuthHandle, nvIndex tpm2.TPMHandle, size, offset, maxBufSize uint16) ([]byte, error) {
	if maxBufSize == 0 {
		maxBufSize = uint16(t.Config.MaxNVBufferSize())
	}
	if authHandle.Auth == nil {
		authHandle.Auth = tpm2.PasswordAuth(nil)
	}

	nh, err := t.NVNamedHandle(nvIndex)
	if err != nil {
		return nil, err
	}
	if authHandle.Handle == nvIndex {
		authHandle.Name = nh.Name
	}

	out := make([]byte, 0, size)
	remaining := size
	curOffset := offset
	for {
		chunkLen := remaining
		if chunkLen > maxBufSize {
			chunkLen = maxBufSize
		}
		nvRead := tpm2.NVRead{
			AuthHandle: authHandle,
			NVIndex:    nh,
			Size:       chunkLen,
			Offset:     curOffset,
		}
		resp, err := nvRead.Execute(t)
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Data.Buffer...)
		remaining -= chunkLen
		curOffset += chunkLen
		if remaining == 0 {
			break
		}
	}
	return out, nil
}
