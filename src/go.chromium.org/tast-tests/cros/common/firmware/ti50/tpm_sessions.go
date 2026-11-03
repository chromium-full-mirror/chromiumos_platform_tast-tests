// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"crypto/rand"
	"crypto/rsa"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

// NullSymDef returns a TPMTSymDef with Algorithm set to TPM_ALG_NULL.
func NullSymDef() tpm2.TPMTSymDef {
	return tpm2.TPMTSymDef{
		Algorithm: tpm2.TPMAlgNull,
	}
}

// AESSymDef returns a TPMTSymDef for AES with the specified key bit size and mode
// (e.g., tpm2.TPMAlgCFB or tpm2.TPMAlgXOR).
func AESSymDef(bits uint16, mode tpm2.TPMAlgID) tpm2.TPMTSymDef {
	return tpm2.TPMTSymDef{
		Algorithm: tpm2.TPMAlgAES,
		KeyBits:   tpm2.NewTPMUSymKeyBits(tpm2.TPMAlgAES, tpm2.TPMKeyBits(bits)),
		Mode:      tpm2.NewTPMUSymMode(tpm2.TPMAlgAES, mode),
	}
}

// XORSymDef returns a TPMTSymDef for TPM_ALG_XOR parameter obfuscation using hashAlg.
func XORSymDef(hashAlg tpm2.TPMAlgID) tpm2.TPMTSymDef {
	return tpm2.TPMTSymDef{
		Algorithm: tpm2.TPMAlgXOR,
		KeyBits:   tpm2.NewTPMUSymKeyBits(tpm2.TPMAlgXOR, hashAlg),
	}
}

// PasswordHandle constructs a tpm2.AuthHandle using password authorization.
// If name.Buffer is empty, it defaults to the 4-byte big-endian representation
// of handle (or PlaceholderEntityName for non-permanent handles).
func PasswordHandle(handle tpm2.TPMHandle, name tpm2.TPM2BName, password []byte) tpm2.AuthHandle {
	if len(name.Buffer) == 0 {
		name = namedOrRawHandle(handle).Name
	}
	return tpm2.AuthHandle{
		Handle: handle,
		Name:   name,
		Auth:   tpm2.PasswordAuth(password),
	}
}

// SessionHandle constructs a tpm2.AuthHandle bound to an HMAC or Policy session.
func SessionHandle(handle tpm2.TPMHandle, name tpm2.TPM2BName, session tpm2.Session) tpm2.AuthHandle {
	return tpm2.AuthHandle{
		Handle: handle,
		Name:   name,
		Auth:   session,
	}
}

// NewHMACSession starts and initializes a reusable HMAC authorization session on the TPM.
// The returned cleanup function flushes the session handle when called.
func (t *TpmHandle) NewHMACSession(hashAlg tpm2.TPMAlgID, nonceLen int, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	sess, closer, err := tpm2.HMACSession(t, hashAlg, nonceLen, opts...)
	if err != nil {
		return nil, func() {}, err
	}
	return sess, func() {
		if err := closer(); err != nil {
			t.logDiscoveryError("failed to close HMAC session 0x%x: %v", uint32(sess.Handle()), err)
		}
	}, nil
}

// sanitizeSaltKeyPub clears a non-null RSA scheme on a copy of pub so that
// go-tpm's ImportEncapsulationKey accepts any decrypt-capable RSA key (per
// TPM 2.0 Part 3 TPM2_StartAuthSession, which always uses OAEP with the key's
// nameAlg for salt encryption regardless of the key's scheme).
func sanitizeSaltKeyPub(pub tpm2.TPMTPublic) tpm2.TPMTPublic {
	if pub.Type == tpm2.TPMAlgRSA {
		if rsaDetail, err := pub.Parameters.RSADetail(); err == nil && rsaDetail.Scheme.Scheme != tpm2.TPMAlgNull {
			rsaCopy := *rsaDetail
			rsaCopy.Scheme = tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull}
			pub.Parameters = tpm2.NewTPMUPublicParms(tpm2.TPMAlgRSA, &rsaCopy)
		}
	}
	return pub
}

// NewSaltedHMACSession starts and initializes a salted HMAC authorization
// session using saltKeyHandle and saltKeyPub.
func (t *TpmHandle) NewSaltedHMACSession(hashAlg tpm2.TPMAlgID, nonceLen int, saltKeyHandle tpm2.TPMHandle, saltKeyPub tpm2.TPMTPublic, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	allOpts := append([]tpm2.AuthOption{tpm2.Salted(saltKeyHandle, sanitizeSaltKeyPub(saltKeyPub))}, opts...)
	return t.NewHMACSession(hashAlg, nonceLen, allOpts...)
}

// NewBoundHMACSession starts and initializes a bound HMAC authorization session
// bound to bindHandle.
func (t *TpmHandle) NewBoundHMACSession(hashAlg tpm2.TPMAlgID, nonceLen int, bindHandle tpm2.TPMHandle, bindName tpm2.TPM2BName, bindAuth []byte, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	allOpts := append([]tpm2.AuthOption{tpm2.Bound(bindHandle, bindName, bindAuth)}, opts...)
	return t.NewHMACSession(hashAlg, nonceLen, allOpts...)
}

// NewPolicySession starts and initializes a policy authorization session on the TPM.
// The returned cleanup function flushes the session handle when called.
func (t *TpmHandle) NewPolicySession(hashAlg tpm2.TPMAlgID, nonceLen int, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	sess, closer, err := tpm2.PolicySession(t, hashAlg, nonceLen, opts...)
	if err != nil {
		return nil, func() {}, err
	}
	return sess, func() {
		if err := closer(); err != nil {
			t.logDiscoveryError("failed to close policy session 0x%x: %v", uint32(sess.Handle()), err)
		}
	}, nil
}

// PolicyCommandCodeSession starts a policy authorization session and asserts
// TPM2_PolicyCommandCode(cc) on it, flushing the session if PolicyCommandCode
// fails.
func (t *TpmHandle) PolicyCommandCodeSession(hashAlg tpm2.TPMAlgID, nonceLen int, cc tpm2.TPMCC, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	sess, closeSess, err := t.NewPolicySession(hashAlg, nonceLen, opts...)
	if err != nil {
		return nil, func() {}, err
	}
	policyCC := tpm2.PolicyCommandCode{
		PolicySession: sess.Handle(),
		Code:          cc,
	}
	if _, err := policyCC.Execute(t); err != nil {
		closeSess()
		return nil, func() {}, err
	}
	return sess, closeSess, nil
}

// PolicyGetDigest returns the current policy digest of sessionHandle.
func (t *TpmHandle) PolicyGetDigest(sessionHandle tpm2.TPMHandle) ([]byte, error) {
	cmd := tpm2.PolicyGetDigest{PolicySession: sessionHandle}
	resp, err := cmd.Execute(t)
	if err != nil {
		return nil, err
	}
	return resp.PolicyDigest.Buffer, nil
}

// PolicyPasswordAuthOption returns tpm2.Password(auth) when the TPM supports
// TPM_CC_PolicyPassword, or tpm2.Auth(auth) when falling back to
// TPM2_PolicyAuthValue.
func (t *TpmHandle) PolicyPasswordAuthOption(auth []byte) tpm2.AuthOption {
	if t.Config != nil && !t.Config.IsCommandSupported(tpm2.TPMCCPolicyPassword) {
		return tpm2.Auth(auth)
	}
	return tpm2.Password(auth)
}

// NewTrialSession starts and initializes a trial policy session on the TPM.
// The returned cleanup function flushes the session handle when called.
func (t *TpmHandle) NewTrialSession(hashAlg tpm2.TPMAlgID, nonceLen int, opts ...tpm2.AuthOption) (tpm2.Session, func(), error) {
	allOpts := append([]tpm2.AuthOption{tpm2.Trial()}, opts...)
	return t.NewPolicySession(hashAlg, nonceLen, allOpts...)
}

// StartAuthSessionRaw executes TPM2_StartAuthSession directly with explicit
// parameters (useful for testing boundary conditions, salted RSA/ECC secrets,
// bound handles, and invalid nonces/algorithms).
func (t *TpmHandle) StartAuthSessionRaw(tpmKey, bind tpm2.TPMHandle, nonceCaller, encryptedSalt []byte, sessionType tpm2.TPMSE, sym tpm2.TPMTSymDef, authHash tpm2.TPMAlgID) (*tpm2.StartAuthSessionResponse, error) {
	startAuthSession := tpm2.StartAuthSession{
		TPMKey:        namedOrRawHandle(tpmKey),
		Bind:          namedOrRawHandle(bind),
		NonceCaller:   tpm2.TPM2BNonce{Buffer: nonceCaller},
		EncryptedSalt: tpm2.TPM2BEncryptedSecret{Buffer: encryptedSalt},
		SessionType:   sessionType,
		Symmetric:     sym,
		AuthHash:      authHash,
	}
	return startAuthSession.Execute(t)
}

// CreateSalt generates a random salt and encapsulates it for pub (supporting
// both RSA-OAEP and ECDH key encapsulation with the "SECRET\x00" label).
func CreateSalt(pub tpm2.TPMTPublic) (salt, encryptedSalt []byte, err error) {
	sanitized := sanitizeSaltKeyPub(pub)
	encapKey, err := tpm2.ImportEncapsulationKey(&sanitized)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to import encapsulation key for salt")
	}
	return tpm2.CreateEncryptedSalt(rand.Reader, encapKey)
}

// EncryptCustomSaltRSA encrypts a caller-supplied salt byte slice using RSA-OAEP
// with pub's NameAlg and the null-terminated "SECRET\x00" label (useful for
// testing custom salt sizes in TPM2_StartAuthSession).
func EncryptCustomSaltRSA(pub tpm2.TPMTPublic, salt []byte) ([]byte, error) {
	rsaPub, err := TPMPublicToRSA(pub)
	if err != nil {
		return nil, err
	}
	h, err := pub.NameAlg.Hash()
	if err != nil {
		return nil, errors.Wrap(err, "unsupported NameAlg for RSA-OAEP salt encryption")
	}
	return rsa.EncryptOAEP(h.New(), rand.Reader, rsaPub, salt, []byte("SECRET\x00"))
}

// PolicyAuthValueDigest computes the software policy digest for a single
// TPM2_PolicyPassword or TPM2_PolicyAuthValue assertion (both update the policy
// digest with TPM_CC_PolicyAuthValue per TPM 2.0 Part 3 §23.7 and §23.8) using
// hashAlg.
func PolicyAuthValueDigest(hashAlg tpm2.TPMAlgID) ([]byte, error) {
	calc, err := tpm2.NewPolicyCalculator(hashAlg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create policy calculator for 0x%x", uint16(hashAlg))
	}
	policyAV := tpm2.PolicyAuthValue{}
	if err := policyAV.Update(calc); err != nil {
		return nil, errors.Wrap(err, "failed to update PolicyAuthValue digest")
	}
	return calc.Hash().Digest, nil
}

// PolicyCommandCodeDigest computes the software policy digest for a single
// TPM2_PolicyCommandCode(cc) assertion using hashAlg.
func PolicyCommandCodeDigest(hashAlg tpm2.TPMAlgID, cc tpm2.TPMCC) ([]byte, error) {
	calc, err := tpm2.NewPolicyCalculator(hashAlg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create policy calculator for 0x%x", uint16(hashAlg))
	}
	policyCC := tpm2.PolicyCommandCode{Code: cc}
	if err := policyCC.Update(calc); err != nil {
		return nil, errors.Wrapf(err, "failed to update PolicyCommandCode(0x%x) digest", uint32(cc))
	}
	return calc.Hash().Digest, nil
}
