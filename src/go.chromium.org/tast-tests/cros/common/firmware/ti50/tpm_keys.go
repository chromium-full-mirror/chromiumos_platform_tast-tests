// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"slices"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

// DefaultPrimaryKeyAttrs returns standard object attributes for a fixed TPM
// signing or decryption key with user authorization.
func DefaultPrimaryKeyAttrs(sign, decrypt, restricted bool) tpm2.TPMAObject {
	return tpm2.TPMAObject{
		FixedTPM:            true,
		FixedParent:         true,
		SensitiveDataOrigin: true,
		UserWithAuth:        true,
		SignEncrypt:         sign,
		Decrypt:             decrypt,
		Restricted:          restricted,
	}
}

func rsaTemplate(nameAlg tpm2.TPMAlgID, bits uint16, attrs tpm2.TPMAObject, sym tpm2.TPMTSymDefObject, scheme tpm2.TPMTRSAScheme) tpm2.TPMTPublic {
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgRSA,
		NameAlg:          nameAlg,
		ObjectAttributes: attrs,
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgRSA,
			&tpm2.TPMSRSAParms{
				Symmetric: sym,
				Scheme:    scheme,
				KeyBits:   tpm2.TPMKeyBits(bits),
			},
		),
		Unique: tpm2.NewTPMUPublicID(tpm2.TPMAlgRSA, &tpm2.TPM2BPublicKeyRSA{}),
	}
}

// RSASSADefaultTemplate returns a TPMTPublic template for an unrestricted
// RSASSA signing key.
func RSASSADefaultTemplate(nameAlg, sigHash tpm2.TPMAlgID, bits uint16) tpm2.TPMTPublic {
	scheme := tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull}
	if sigHash != tpm2.TPMAlgNull {
		scheme = tpm2.TPMTRSAScheme{
			Scheme: tpm2.TPMAlgRSASSA,
			Details: tpm2.NewTPMUAsymScheme(
				tpm2.TPMAlgRSASSA,
				&tpm2.TPMSSigSchemeRSASSA{HashAlg: sigHash},
			),
		}
	}
	return rsaTemplate(
		nameAlg,
		bits,
		DefaultPrimaryKeyAttrs(true, false, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		scheme,
	)
}

// RSAPSSDefaultTemplate returns a TPMTPublic template for an unrestricted
// RSAPSS signing key. Callers targeting GSC must ensure sigHash and bits
// satisfy the target's supported algorithms and key sizes (see TpmConfig).
func RSAPSSDefaultTemplate(nameAlg, sigHash tpm2.TPMAlgID, bits uint16) tpm2.TPMTPublic {
	scheme := tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull}
	if sigHash != tpm2.TPMAlgNull {
		scheme = tpm2.TPMTRSAScheme{
			Scheme: tpm2.TPMAlgRSAPSS,
			Details: tpm2.NewTPMUAsymScheme(
				tpm2.TPMAlgRSAPSS,
				&tpm2.TPMSSigSchemeRSAPSS{HashAlg: sigHash},
			),
		}
	}
	return rsaTemplate(
		nameAlg,
		bits,
		DefaultPrimaryKeyAttrs(true, false, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		scheme,
	)
}

// RSAOAEPDefaultTemplate returns a TPMTPublic template for an unrestricted
// RSA-OAEP decryption key (or TPM_ALG_NULL scheme if encHash == tpm2.TPMAlgNull).
// On GSC firmware, RSA-OAEP requires a hash digest of at most 32 bytes (see
// TpmConfig.OaepHashAlgs).
func RSAOAEPDefaultTemplate(nameAlg, encHash tpm2.TPMAlgID, bits uint16) tpm2.TPMTPublic {
	scheme := tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull}
	if encHash != tpm2.TPMAlgNull {
		scheme = tpm2.TPMTRSAScheme{
			Scheme: tpm2.TPMAlgOAEP,
			Details: tpm2.NewTPMUAsymScheme(
				tpm2.TPMAlgOAEP,
				&tpm2.TPMSEncSchemeOAEP{HashAlg: encHash},
			),
		}
	}
	return rsaTemplate(
		nameAlg,
		bits,
		DefaultPrimaryKeyAttrs(false, true, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		scheme,
	)
}

// RSAESDefaultTemplate returns a TPMTPublic template for an unrestricted
// RSAES-PKCS1-v1_5 decryption key.
func RSAESDefaultTemplate(nameAlg tpm2.TPMAlgID, bits uint16) tpm2.TPMTPublic {
	return rsaTemplate(
		nameAlg,
		bits,
		DefaultPrimaryKeyAttrs(false, true, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		tpm2.TPMTRSAScheme{
			Scheme:  tpm2.TPMAlgRSAES,
			Details: tpm2.NewTPMUAsymScheme(tpm2.TPMAlgRSAES, &tpm2.TPMSEncSchemeRSAES{}),
		},
	)
}

// RSAStorageParentTemplate returns a TPMTPublic template for a restricted RSA
// storage parent key with AES-128-CFB symmetric protection (GSC firmware only
// supports 128-bit AES for storage parent symmetric protection).
func RSAStorageParentTemplate(nameAlg tpm2.TPMAlgID, bits uint16) tpm2.TPMTPublic {
	return rsaTemplate(
		nameAlg,
		bits,
		DefaultPrimaryKeyAttrs(false, true, true),
		tpm2.TPMTSymDefObject{
			Algorithm: tpm2.TPMAlgAES,
			KeyBits:   tpm2.NewTPMUSymKeyBits(tpm2.TPMAlgAES, tpm2.TPMKeyBits(128)),
			Mode:      tpm2.NewTPMUSymMode(tpm2.TPMAlgAES, tpm2.TPMAlgCFB),
		},
		tpm2.TPMTRSAScheme{Scheme: tpm2.TPMAlgNull},
	)
}

func eccTemplate(nameAlg tpm2.TPMAlgID, curve tpm2.TPMECCCurve, attrs tpm2.TPMAObject, sym tpm2.TPMTSymDefObject, scheme tpm2.TPMTECCScheme) tpm2.TPMTPublic {
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgECC,
		NameAlg:          nameAlg,
		ObjectAttributes: attrs,
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgECC,
			&tpm2.TPMSECCParms{
				Symmetric: sym,
				Scheme:    scheme,
				CurveID:   curve,
				KDF:       tpm2.TPMTKDFScheme{Scheme: tpm2.TPMAlgNull},
			},
		),
		Unique: tpm2.NewTPMUPublicID(tpm2.TPMAlgECC, &tpm2.TPMSECCPoint{}),
	}
}

// ECDSATemplate returns a TPMTPublic template for an unrestricted ECDSA signing
// key on the specified curve. On GSC firmware, standard ECDSA key generation is
// restricted to TPM_ECC_NIST_P256 (see ECDSAP256Template).
func ECDSATemplate(nameAlg, sigHash tpm2.TPMAlgID, curve tpm2.TPMECCCurve) tpm2.TPMTPublic {
	scheme := tpm2.TPMTECCScheme{Scheme: tpm2.TPMAlgNull}
	if sigHash != tpm2.TPMAlgNull {
		scheme = tpm2.TPMTECCScheme{
			Scheme: tpm2.TPMAlgECDSA,
			Details: tpm2.NewTPMUAsymScheme(
				tpm2.TPMAlgECDSA,
				&tpm2.TPMSSigSchemeECDSA{HashAlg: sigHash},
			),
		}
	}
	return eccTemplate(
		nameAlg,
		curve,
		DefaultPrimaryKeyAttrs(true, false, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		scheme,
	)
}

// ECDSAP256Template returns a TPMTPublic template for an unrestricted NIST P-256
// ECDSA signing key.
func ECDSAP256Template(nameAlg, sigHash tpm2.TPMAlgID) tpm2.TPMTPublic {
	return ECDSATemplate(nameAlg, sigHash, tpm2.TPMECCNistP256)
}

// ECDHTemplate returns a TPMTPublic template for an unrestricted ECDH decryption
// key on the specified curve. On GSC firmware, ECDH key generation is restricted
// to TPM_ECC_NIST_P256 (see ECDHP256Template).
func ECDHTemplate(nameAlg tpm2.TPMAlgID, curve tpm2.TPMECCCurve) tpm2.TPMTPublic {
	return eccTemplate(
		nameAlg,
		curve,
		DefaultPrimaryKeyAttrs(false, true, false),
		tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
		tpm2.TPMTECCScheme{Scheme: tpm2.TPMAlgNull},
	)
}

// ECDHP256Template returns a TPMTPublic template for an unrestricted NIST P-256
// ECDH decryption key.
func ECDHP256Template(nameAlg tpm2.TPMAlgID) tpm2.TPMTPublic {
	return ECDHTemplate(nameAlg, tpm2.TPMECCNistP256)
}

// ECCStorageParentTemplate returns a TPMTPublic template for a restricted ECC
// storage parent key with AES-128-CFB symmetric protection (GSC firmware only
// supports 128-bit AES for storage parent symmetric protection).
func ECCStorageParentTemplate(nameAlg tpm2.TPMAlgID, curve tpm2.TPMECCCurve) tpm2.TPMTPublic {
	return eccTemplate(
		nameAlg,
		curve,
		DefaultPrimaryKeyAttrs(false, true, true),
		tpm2.TPMTSymDefObject{
			Algorithm: tpm2.TPMAlgAES,
			KeyBits:   tpm2.NewTPMUSymKeyBits(tpm2.TPMAlgAES, tpm2.TPMKeyBits(128)),
			Mode:      tpm2.NewTPMUSymMode(tpm2.TPMAlgAES, tpm2.TPMAlgCFB),
		},
		tpm2.TPMTECCScheme{Scheme: tpm2.TPMAlgNull},
	)
}

// SymCipherTemplate returns a TPMTPublic template for a symmetric block cipher key.
func SymCipherTemplate(nameAlg, symAlg, mode tpm2.TPMAlgID, keyBits uint16, sign, decrypt bool) tpm2.TPMTPublic {
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgSymCipher,
		NameAlg:          nameAlg,
		ObjectAttributes: DefaultPrimaryKeyAttrs(sign, decrypt, false),
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgSymCipher,
			&tpm2.TPMSSymCipherParms{
				Sym: tpm2.TPMTSymDefObject{
					Algorithm: symAlg,
					KeyBits:   tpm2.NewTPMUSymKeyBits(symAlg, tpm2.TPMKeyBits(keyBits)),
					Mode:      tpm2.NewTPMUSymMode(symAlg, mode),
				},
			},
		),
		Unique: tpm2.NewTPMUPublicID(tpm2.TPMAlgSymCipher, &tpm2.TPM2BDigest{}),
	}
}

// AESCFBTemplate returns a TPMTPublic template for an AES-CFB symmetric key
// with both SignEncrypt and Decrypt enabled.
func AESCFBTemplate(nameAlg tpm2.TPMAlgID, keyBits uint16) tpm2.TPMTPublic {
	return SymCipherTemplate(nameAlg, tpm2.TPMAlgAES, tpm2.TPMAlgCFB, keyBits, true, true)
}

// HMACTemplate returns a TPMTPublic template for an unrestricted keyed-hash
// HMAC signing key. Passing hashAlg == tpm2.TPMAlgNull produces a template with
// SignEncrypt: true and Scheme: TPM_ALG_NULL, which is invalid per the TPM 2.0
// specification and intended only for negative tests (use SealingTemplate for
// valid data-sealing keyed-hash objects).
func HMACTemplate(nameAlg, hashAlg tpm2.TPMAlgID) tpm2.TPMTPublic {
	scheme := tpm2.TPMTKeyedHashScheme{Scheme: tpm2.TPMAlgNull}
	if hashAlg != tpm2.TPMAlgNull {
		scheme = tpm2.TPMTKeyedHashScheme{
			Scheme: tpm2.TPMAlgHMAC,
			Details: tpm2.NewTPMUSchemeKeyedHash(
				tpm2.TPMAlgHMAC,
				&tpm2.TPMSSchemeHMAC{HashAlg: hashAlg},
			),
		}
	}
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgKeyedHash,
		NameAlg:          nameAlg,
		ObjectAttributes: DefaultPrimaryKeyAttrs(true, false, false),
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgKeyedHash,
			&tpm2.TPMSKeyedHashParms{
				Scheme: scheme,
			},
		),
		Unique: tpm2.NewTPMUPublicID(tpm2.TPMAlgKeyedHash, &tpm2.TPM2BDigest{}),
	}
}

// SealingTemplate returns a TPMTPublic template for a keyed-hash data sealing
// object (`TPM_ALG_NULL` scheme, `SensitiveDataOrigin: false`).
func SealingTemplate(nameAlg tpm2.TPMAlgID, authPolicy []byte) tpm2.TPMTPublic {
	return tpm2.TPMTPublic{
		Type:    tpm2.TPMAlgKeyedHash,
		NameAlg: nameAlg,
		ObjectAttributes: tpm2.TPMAObject{
			FixedTPM:     true,
			FixedParent:  true,
			UserWithAuth: true,
		},
		AuthPolicy: tpm2.TPM2BDigest{Buffer: authPolicy},
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgKeyedHash,
			&tpm2.TPMSKeyedHashParms{
				Scheme: tpm2.TPMTKeyedHashScheme{Scheme: tpm2.TPMAlgNull},
			},
		),
		Unique: tpm2.NewTPMUPublicID(tpm2.TPMAlgKeyedHash, &tpm2.TPM2BDigest{}),
	}
}

// CreatePrimary creates a primary object from tmpl in hierarchy and returns its
// password AuthHandle (with userAuth) and public area. For keyed-hash and
// symmetric cipher templates, if sensitiveData is non-empty,
// SensitiveDataOrigin is cleared so that the TPM uses the caller-supplied data.
// The caller is responsible for flushing the returned handle (e.g. via
// defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) CreatePrimary(hierarchy tpm2.TPMHandle, tmpl tpm2.TPMTPublic, userAuth, sensitiveData []byte) (tpm2.AuthHandle, *tpm2.TPMTPublic, error) {
	if len(sensitiveData) > 0 && (tmpl.Type == tpm2.TPMAlgKeyedHash || tmpl.Type == tpm2.TPMAlgSymCipher) {
		tmpl.ObjectAttributes.SensitiveDataOrigin = false
	}
	createPrimary := tpm2.CreatePrimary{
		PrimaryHandle: hierarchy,
		InSensitive:   sensitiveCreate(userAuth, sensitiveData),
		InPublic:      tpm2.New2B(tmpl),
	}
	resp, err := createPrimary.Execute(t)
	if err != nil {
		return tpm2.AuthHandle{}, nil, err
	}
	pub, err := resp.OutPublic.Contents()
	if err != nil {
		t.FlushHandle(resp.ObjectHandle)
		return tpm2.AuthHandle{}, nil, err
	}
	return PasswordHandle(resp.ObjectHandle, resp.Name, userAuth), pub, nil
}

// CreateKey creates a child object from tmpl under parent without loading it,
// returning the full TPM2_Create response (including OutPrivate) and unpacked
// public area. For keyed-hash and symmetric cipher templates, if sensitiveData
// is non-empty, SensitiveDataOrigin is cleared so that the TPM uses the
// caller-supplied data.
func (t *TpmHandle) CreateKey(parent tpm2.AuthHandle, tmpl tpm2.TPMTPublic, userAuth, sensitiveData []byte) (*tpm2.CreateResponse, *tpm2.TPMTPublic, error) {
	if len(sensitiveData) > 0 && (tmpl.Type == tpm2.TPMAlgKeyedHash || tmpl.Type == tpm2.TPMAlgSymCipher) {
		tmpl.ObjectAttributes.SensitiveDataOrigin = false
	}
	create := tpm2.Create{
		ParentHandle: parent,
		InSensitive:  sensitiveCreate(userAuth, sensitiveData),
		InPublic:     tpm2.New2B(tmpl),
	}
	createResp, err := create.Execute(t)
	if err != nil {
		return nil, nil, err
	}
	pub, err := createResp.OutPublic.Contents()
	if err != nil {
		return nil, nil, err
	}
	return createResp, pub, nil
}

// CreateAndLoadKey creates a child object from tmpl under parent, loads it, and
// returns its password AuthHandle (with userAuth) and public area. For
// keyed-hash and symmetric cipher templates, if sensitiveData is non-empty,
// SensitiveDataOrigin is cleared so that the TPM uses the caller-supplied data.
// The caller is responsible for flushing the returned handle (e.g. via
// defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) CreateAndLoadKey(parent tpm2.AuthHandle, tmpl tpm2.TPMTPublic, userAuth, sensitiveData []byte) (tpm2.AuthHandle, *tpm2.TPMTPublic, error) {
	createResp, pub, err := t.CreateKey(parent, tmpl, userAuth, sensitiveData)
	if err != nil {
		return tpm2.AuthHandle{}, nil, err
	}
	load := tpm2.Load{
		ParentHandle: parent,
		InPrivate:    createResp.OutPrivate,
		InPublic:     createResp.OutPublic,
	}
	loadResp, err := load.Execute(t)
	if err != nil {
		return tpm2.AuthHandle{}, nil, err
	}
	return PasswordHandle(loadResp.ObjectHandle, loadResp.Name, userAuth), pub, nil
}

// ImportKey imports duplicate (with optional inner wrapper encryptionKey/symDef
// and outer wrapper inSymSeed) under parent without loading it, returning the
// TPM2_Import response (including OutPrivate).
func (t *TpmHandle) ImportKey(parent tpm2.AuthHandle, pub tpm2.TPMTPublic, duplicate, inSymSeed, encryptionKey []byte, symDef tpm2.TPMTSymDef) (*tpm2.ImportResponse, error) {
	if len(encryptionKey) > 0 && symDef.Algorithm == tpm2.TPMAlgNull {
		return nil, errors.New("encryptionKey provided with TPM_ALG_NULL symDef")
	}
	imp := tpm2.Import{
		ParentHandle:  parent,
		EncryptionKey: tpm2.TPM2BData{Buffer: encryptionKey},
		ObjectPublic:  tpm2.New2B(pub),
		Duplicate:     tpm2.TPM2BPrivate{Buffer: duplicate},
		InSymSeed:     tpm2.TPM2BEncryptedSecret{Buffer: inSymSeed},
		Symmetric:     symDef,
	}
	return imp.Execute(t)
}

// ImportAndLoad imports duplicate (with optional inner wrapper encryptionKey/symDef
// and outer wrapper inSymSeed) under parent, loads the resulting private blob
// with pub, and returns a password AuthHandle (with userAuth). The caller is
// responsible for flushing the returned handle (e.g. via
// defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) ImportAndLoad(parent tpm2.AuthHandle, pub tpm2.TPMTPublic, duplicate, inSymSeed, encryptionKey []byte, symDef tpm2.TPMTSymDef, userAuth []byte) (tpm2.AuthHandle, error) {
	impResp, err := t.ImportKey(parent, pub, duplicate, inSymSeed, encryptionKey, symDef)
	if err != nil {
		return tpm2.AuthHandle{}, err
	}
	load := tpm2.Load{
		ParentHandle: parent,
		InPrivate:    impResp.OutPrivate,
		InPublic:     tpm2.New2B(pub),
	}
	loadResp, err := load.Execute(t)
	if err != nil {
		return tpm2.AuthHandle{}, err
	}
	return PasswordHandle(loadResp.ObjectHandle, loadResp.Name, userAuth), nil
}

// sensitiveCreate builds the TPM2B_SENSITIVE_CREATE for Create/CreatePrimary.
func sensitiveCreate(userAuth, sensitiveData []byte) tpm2.TPM2BSensitiveCreate {
	return tpm2.TPM2BSensitiveCreate{
		Sensitive: &tpm2.TPMSSensitiveCreate{
			UserAuth: tpm2.TPM2BAuth{Buffer: userAuth},
			Data:     tpm2.NewTPMUSensitiveCreate(&tpm2.TPM2BSensitiveData{Buffer: sensitiveData}),
		},
	}
}

// tamperedUnique returns a copy of pub.Unique whose buffer (the RSA modulus,
// the ECC X coordinate, or the KEYEDHASH/SYMCIPHER digest) is passed through
// mutate. For ECC, flipY also mutates the Y coordinate. Returns an error if the
// underlying Unique buffer is empty.
func tamperedUnique(pub tpm2.TPMTPublic, mutate, flipY func([]byte) []byte) (tpm2.TPMUPublicID, error) {
	switch pub.Type {
	case tpm2.TPMAlgRSA:
		u, err := pub.Unique.RSA()
		if err != nil {
			return tpm2.TPMUPublicID{}, err
		}
		if len(u.Buffer) == 0 {
			return tpm2.TPMUPublicID{}, errors.New("RSA OutPublic has empty Unique buffer")
		}
		return tpm2.NewTPMUPublicID(tpm2.TPMAlgRSA, &tpm2.TPM2BPublicKeyRSA{Buffer: mutate(u.Buffer)}), nil
	case tpm2.TPMAlgECC:
		u, err := pub.Unique.ECC()
		if err != nil {
			return tpm2.TPMUPublicID{}, err
		}
		if len(u.X.Buffer) == 0 || len(u.Y.Buffer) == 0 {
			return tpm2.TPMUPublicID{}, errors.New("ECC OutPublic has empty Unique coordinate buffer")
		}
		y := u.Y.Buffer
		if flipY != nil {
			y = flipY(y)
		}
		return tpm2.NewTPMUPublicID(tpm2.TPMAlgECC, &tpm2.TPMSECCPoint{
			X: tpm2.TPM2BECCParameter{Buffer: mutate(u.X.Buffer)},
			Y: tpm2.TPM2BECCParameter{Buffer: y},
		}), nil
	case tpm2.TPMAlgKeyedHash:
		u, err := pub.Unique.KeyedHash()
		if err != nil {
			return tpm2.TPMUPublicID{}, err
		}
		if len(u.Buffer) == 0 {
			return tpm2.TPMUPublicID{}, errors.New("KeyedHash OutPublic has empty Unique buffer")
		}
		return tpm2.NewTPMUPublicID(tpm2.TPMAlgKeyedHash, &tpm2.TPM2BDigest{Buffer: mutate(u.Buffer)}), nil
	case tpm2.TPMAlgSymCipher:
		u, err := pub.Unique.SymCipher()
		if err != nil {
			return tpm2.TPMUPublicID{}, err
		}
		if len(u.Buffer) == 0 {
			return tpm2.TPMUPublicID{}, errors.New("SymCipher OutPublic has empty Unique buffer")
		}
		return tpm2.NewTPMUPublicID(tpm2.TPMAlgSymCipher, &tpm2.TPM2BDigest{Buffer: mutate(u.Buffer)}), nil
	}
	return tpm2.TPMUPublicID{}, errors.Errorf("unsupported public type 0x%x", uint16(pub.Type))
}

// VerifyLoadTamperedOutPublic checks that TPM2_Load of outPrivate under parent
// rejects every tampered variant of pub (toggled sensitiveDataOrigin, toggled
// adminWithPolicy, bit-flipped, truncated, and extended Unique), and that the
// untampered pub still loads after each rejection.
func (t *TpmHandle) VerifyLoadTamperedOutPublic(parent tpm2.AuthHandle, outPrivate tpm2.TPM2BPrivate, pub tpm2.TPMTPublic) error {
	load := func(p tpm2.TPMTPublic) error {
		resp, err := (tpm2.Load{ParentHandle: parent, InPrivate: outPrivate, InPublic: tpm2.New2B(p)}).Execute(t)
		if err == nil {
			t.FlushHandle(resp.ObjectHandle)
		}
		return err
	}
	expectRejected := func(desc string, p tpm2.TPMTPublic, rcs ...tpm2.TPMRC) error {
		if err := ExpectError(load(p), rcs...); err != nil {
			return errors.Wrapf(err, "Load with %s", desc)
		}
		if err := load(pub); err != nil {
			return errors.Wrapf(err, "Load after restoring %s", desc)
		}
		return nil
	}

	p := pub
	p.ObjectAttributes.SensitiveDataOrigin = !pub.ObjectAttributes.SensitiveDataOrigin
	if err := expectRejected("toggled SensitiveDataOrigin", p, tpm2.TPMRCIntegrity); err != nil {
		return err
	}
	p = pub
	p.ObjectAttributes.AdminWithPolicy = !pub.ObjectAttributes.AdminWithPolicy
	if err := expectRejected("toggled AdminWithPolicy", p, tpm2.TPMRCIntegrity); err != nil {
		return err
	}

	cases := []struct {
		desc   string
		mutate func([]byte) []byte
		flipY  func([]byte) []byte
		rcs    []tpm2.TPMRC
	}{
		{"bit-flipped Unique #1", t.Rand.ModifyBytes, t.Rand.ModifyBytes, []tpm2.TPMRC{tpm2.TPMRCIntegrity, tpm2.TPMRCBinding}},
		{"bit-flipped Unique #2", t.Rand.ModifyBytes, t.Rand.ModifyBytes, []tpm2.TPMRC{tpm2.TPMRCIntegrity, tpm2.TPMRCBinding}},
		{"bit-flipped Unique #3", t.Rand.ModifyBytes, t.Rand.ModifyBytes, []tpm2.TPMRC{tpm2.TPMRCIntegrity, tpm2.TPMRCBinding}},
		{"truncated Unique", func(b []byte) []byte { return append([]byte(nil), b[:len(b)-1]...) }, nil,
			[]tpm2.TPMRC{tpm2.TPMRCKey, tpm2.TPMRCKeySize, tpm2.TPMRCIntegrity, tpm2.TPMRCSize}},
		{"extended Unique", func(b []byte) []byte { return append(append([]byte(nil), b...), 0x01) }, nil,
			[]tpm2.TPMRC{tpm2.TPMRCSize, tpm2.TPMRCIntegrity, tpm2.TPMRCKey, tpm2.TPMRCKeySize}},
	}
	for _, c := range cases {
		u, err := tamperedUnique(pub, c.mutate, c.flipY)
		if err != nil {
			return err
		}
		p = pub
		p.Unique = u
		if err := expectRejected(c.desc, p, c.rcs...); err != nil {
			return err
		}
	}
	return nil
}

// CreateStorageParent creates a restricted RSA (or, without RSA, NIST P-256 ECC)
// storage parent primary key in hierarchy and returns its AuthHandle and public
// area. Unlike tpm2.RSASRKTemplate/ECCSRKTemplate, the template leaves NoDA
// clear (via DefaultPrimaryKeyAttrs) so dictionary-attack tests can exercise
// storage parents. The caller is responsible for flushing the returned handle
// (e.g. via defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) CreateStorageParent(hierarchy tpm2.TPMHandle) (tpm2.AuthHandle, *tpm2.TPMTPublic, error) {
	nameAlg := t.Config.DefaultHashAlg()
	var tmpl tpm2.TPMTPublic
	switch {
	case t.Config.IsAlgSupported(tpm2.TPMAlgRSA):
		bits := uint16(2048)
		if len(t.Config.RsaKeySizes) > 0 && !slices.Contains(t.Config.RsaKeySizes, tpm2.TPMKeyBits(bits)) {
			bits = uint16(t.Config.RsaKeySizes[0])
		}
		tmpl = RSAStorageParentTemplate(nameAlg, bits)
	case t.Config.IsAlgSupported(tpm2.TPMAlgECC):
		// GSC firmware only supports NIST P-256 for restricted ECC storage parents.
		if !t.Config.IsCurveSupported(tpm2.TPMECCNistP256) {
			return tpm2.AuthHandle{}, nil, errors.New("ECC storage parent requires TPM_ECC_NIST_P256, which is not supported")
		}
		tmpl = ECCStorageParentTemplate(nameAlg, tpm2.TPMECCNistP256)
	default:
		return tpm2.AuthHandle{}, nil, errors.New("neither RSA nor ECC supported for storage parent")
	}
	h, pub, err := t.CreatePrimary(hierarchy, tmpl, nil, nil)
	if err != nil {
		return tpm2.AuthHandle{}, nil, errors.Wrap(err, "failed to create storage parent")
	}
	return h, pub, nil
}

// RSASSASigScheme returns a TPMTSigScheme for RSASSA-PKCS1-v1_5 with hashAlg.
func RSASSASigScheme(hashAlg tpm2.TPMAlgID) tpm2.TPMTSigScheme {
	return tpm2.TPMTSigScheme{
		Scheme: tpm2.TPMAlgRSASSA,
		Details: tpm2.NewTPMUSigScheme(
			tpm2.TPMAlgRSASSA,
			&tpm2.TPMSSchemeHash{HashAlg: hashAlg},
		),
	}
}

// RSAPSSSigScheme returns a TPMTSigScheme for RSASSA-PSS with hashAlg.
func RSAPSSSigScheme(hashAlg tpm2.TPMAlgID) tpm2.TPMTSigScheme {
	return tpm2.TPMTSigScheme{
		Scheme: tpm2.TPMAlgRSAPSS,
		Details: tpm2.NewTPMUSigScheme(
			tpm2.TPMAlgRSAPSS,
			&tpm2.TPMSSchemeHash{HashAlg: hashAlg},
		),
	}
}

// ECDSASigScheme returns a TPMTSigScheme for ECDSA with hashAlg.
func ECDSASigScheme(hashAlg tpm2.TPMAlgID) tpm2.TPMTSigScheme {
	return tpm2.TPMTSigScheme{
		Scheme: tpm2.TPMAlgECDSA,
		Details: tpm2.NewTPMUSigScheme(
			tpm2.TPMAlgECDSA,
			&tpm2.TPMSSchemeHash{HashAlg: hashAlg},
		),
	}
}

// HMACSigScheme returns a TPMTSigScheme for HMAC with hashAlg.
func HMACSigScheme(hashAlg tpm2.TPMAlgID) tpm2.TPMTSigScheme {
	return tpm2.TPMTSigScheme{
		Scheme: tpm2.TPMAlgHMAC,
		Details: tpm2.NewTPMUSigScheme(
			tpm2.TPMAlgHMAC,
			&tpm2.TPMSSchemeHMAC{HashAlg: hashAlg},
		),
	}
}

// NullSigScheme returns a null signature scheme (TPM_ALG_NULL).
func NullSigScheme() tpm2.TPMTSigScheme {
	return tpm2.TPMTSigScheme{Scheme: tpm2.TPMAlgNull}
}

// NullHashCheckTicket returns a null validation ticket for TPM2_Sign.
func NullHashCheckTicket() tpm2.TPMTTKHashCheck {
	return tpm2.TPMTTKHashCheck{
		Tag:       tpm2.TPMSTHashCheck,
		Hierarchy: tpm2.TPMRHNull,
	}
}

// SanitizePublicForLoadExternal clears FixedTPM, FixedParent, and Restricted
// attributes on pub, and also sets asymmetric Symmetric definitions to
// TPM_ALG_NULL when stripping Restricted so the public structure can be loaded
// via TPM2_LoadExternal. Negative tests specifically expecting TPM_RC_SYMMETRIC
// on LoadExternal should call tpm2.LoadExternal directly without this helper.
func SanitizePublicForLoadExternal(pub tpm2.TPMTPublic) tpm2.TPMTPublic {
	pub.ObjectAttributes.FixedTPM = false
	pub.ObjectAttributes.FixedParent = false
	pub.ObjectAttributes.Restricted = false

	switch pub.Type {
	case tpm2.TPMAlgRSA:
		if rsaDetail, err := pub.Parameters.RSADetail(); err == nil {
			rsaCopy := *rsaDetail
			rsaCopy.Symmetric = tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull}
			pub.Parameters = tpm2.NewTPMUPublicParms(tpm2.TPMAlgRSA, &rsaCopy)
		}
	case tpm2.TPMAlgECC:
		if eccDetail, err := pub.Parameters.ECCDetail(); err == nil {
			eccCopy := *eccDetail
			eccCopy.Symmetric = tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull}
			pub.Parameters = tpm2.NewTPMUPublicParms(tpm2.TPMAlgECC, &eccCopy)
		}
	}
	return pub
}

// LoadExternalPublic sanitizes pub (clearing FixedTPM, FixedParent, Restricted,
// and non-null asymmetric Symmetric parameters), loads it as a public-only
// external object in hierarchy, and returns its AuthHandle. The caller is
// responsible for flushing the returned handle (e.g. via
// defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) LoadExternalPublic(pub tpm2.TPMTPublic, hierarchy tpm2.TPMHandle) (tpm2.AuthHandle, error) {
	return t.LoadExternalKey(nil, pub, hierarchy)
}

// LoadExternalKey sanitizes pub, loads it (and priv, if non-nil) via
// TPM2_LoadExternal into hierarchy, and returns its password AuthHandle (with
// priv's authValue). The caller is responsible for flushing the returned
// handle (e.g. via defer t.FlushHandle(h.Handle)).
func (t *TpmHandle) LoadExternalKey(priv *tpm2.TPMTSensitive, pub tpm2.TPMTPublic, hierarchy tpm2.TPMHandle) (tpm2.AuthHandle, error) {
	sanitized := SanitizePublicForLoadExternal(pub)
	cmd := tpm2.LoadExternal{Hierarchy: hierarchy}
	var auth []byte
	if priv != nil {
		sanitized.ObjectAttributes.SensitiveDataOrigin = false
		cmd.InPrivate = tpm2.New2B(*priv)
		auth = priv.AuthValue.Buffer
	}
	cmd.InPublic = tpm2.New2B(sanitized)
	resp, err := cmd.Execute(t)
	if err != nil {
		return tpm2.AuthHandle{}, err
	}
	return PasswordHandle(resp.ObjectHandle, resp.Name, auth), nil
}

// GenerateSoftwareRSAKey generates a software RSA key pair of the requested bit size.
func GenerateSoftwareRSAKey(bits int) (*rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate %d-bit RSA key", bits)
	}
	return key, nil
}

// ECCCurveToGo maps a TPM ECC curve ID to the corresponding Go elliptic.Curve.
func ECCCurveToGo(curve tpm2.TPMECCCurve) (elliptic.Curve, error) {
	switch curve {
	case tpm2.TPMECCNistP224:
		return elliptic.P224(), nil
	case tpm2.TPMECCNistP256:
		return elliptic.P256(), nil
	case tpm2.TPMECCNistP384:
		return elliptic.P384(), nil
	case tpm2.TPMECCNistP521:
		return elliptic.P521(), nil
	default:
		return nil, errors.Errorf("unsupported ECC curve 0x%x", uint16(curve))
	}
}

// GenerateSoftwareECCKey generates a software ECDSA/ECDH key pair on curve.
func GenerateSoftwareECCKey(curve elliptic.Curve) (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate software ECC key")
	}
	return key, nil
}

// SoftwareRSATPMPublic converts a Go *rsa.PublicKey into a TPMTPublic structure.
func SoftwareRSATPMPublic(pub *rsa.PublicKey, nameAlg tpm2.TPMAlgID, scheme tpm2.TPMTRSAScheme, attrs tpm2.TPMAObject) tpm2.TPMTPublic {
	modBytes := make([]byte, pub.Size())
	pub.N.FillBytes(modBytes)
	exp := uint32(pub.E)
	if exp == 65537 {
		exp = 0
	}
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgRSA,
		NameAlg:          nameAlg,
		ObjectAttributes: attrs,
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgRSA,
			&tpm2.TPMSRSAParms{
				Symmetric: tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
				Scheme:    scheme,
				KeyBits:   tpm2.TPMKeyBits(pub.Size() * 8),
				Exponent:  exp,
			},
		),
		Unique: tpm2.NewTPMUPublicID(
			tpm2.TPMAlgRSA,
			&tpm2.TPM2BPublicKeyRSA{Buffer: modBytes},
		),
	}
}

// SoftwareRSATPMPrivate converts a Go *rsa.PrivateKey into a TPMTSensitive
// structure (storing prime P in big-endian padded to keySize/2 bytes).
func SoftwareRSATPMPrivate(priv *rsa.PrivateKey, authValue, seed []byte) tpm2.TPMTSensitive {
	primeLen := priv.Size() / 2
	primeBytes := make([]byte, primeLen)
	priv.Primes[0].FillBytes(primeBytes)
	return tpm2.TPMTSensitive{
		SensitiveType: tpm2.TPMAlgRSA,
		AuthValue:     tpm2.TPM2BAuth{Buffer: authValue},
		SeedValue:     tpm2.TPM2BDigest{Buffer: seed},
		Sensitive: tpm2.NewTPMUSensitiveComposite(
			tpm2.TPMAlgRSA,
			&tpm2.TPM2BPrivateKeyRSA{Buffer: primeBytes},
		),
	}
}

// SoftwareECCTPMPublic converts a Go *ecdsa.PublicKey into a TPMTPublic structure.
func SoftwareECCTPMPublic(pub *ecdsa.PublicKey, nameAlg tpm2.TPMAlgID, curve tpm2.TPMECCCurve, scheme tpm2.TPMTECCScheme, attrs tpm2.TPMAObject) tpm2.TPMTPublic {
	coordLen := (pub.Curve.Params().BitSize + 7) / 8
	xBytes := make([]byte, coordLen)
	yBytes := make([]byte, coordLen)
	pub.X.FillBytes(xBytes)
	pub.Y.FillBytes(yBytes)
	return tpm2.TPMTPublic{
		Type:             tpm2.TPMAlgECC,
		NameAlg:          nameAlg,
		ObjectAttributes: attrs,
		Parameters: tpm2.NewTPMUPublicParms(
			tpm2.TPMAlgECC,
			&tpm2.TPMSECCParms{
				Symmetric: tpm2.TPMTSymDefObject{Algorithm: tpm2.TPMAlgNull},
				Scheme:    scheme,
				CurveID:   curve,
				KDF:       tpm2.TPMTKDFScheme{Scheme: tpm2.TPMAlgNull},
			},
		),
		Unique: tpm2.NewTPMUPublicID(
			tpm2.TPMAlgECC,
			&tpm2.TPMSECCPoint{
				X: tpm2.TPM2BECCParameter{Buffer: xBytes},
				Y: tpm2.TPM2BECCParameter{Buffer: yBytes},
			},
		),
	}
}

// SoftwareECCTPMPrivate converts a Go *ecdsa.PrivateKey into a TPMTSensitive
// structure (storing private scalar D padded to coordinate byte length).
func SoftwareECCTPMPrivate(priv *ecdsa.PrivateKey, authValue, seed []byte) tpm2.TPMTSensitive {
	coordLen := (priv.Curve.Params().BitSize + 7) / 8
	dBytes := make([]byte, coordLen)
	priv.D.FillBytes(dBytes)
	return tpm2.TPMTSensitive{
		SensitiveType: tpm2.TPMAlgECC,
		AuthValue:     tpm2.TPM2BAuth{Buffer: authValue},
		SeedValue:     tpm2.TPM2BDigest{Buffer: seed},
		Sensitive: tpm2.NewTPMUSensitiveComposite(
			tpm2.TPMAlgECC,
			&tpm2.TPM2BECCParameter{Buffer: dBytes},
		),
	}
}

// SoftwareSymmetricObject prepares a software-constructed SymCipher or
// KeyedHash TPMTPublic (clearing FixedTPM, FixedParent, Restricted, and
// SensitiveDataOrigin, and setting Unique = H_nameAlg(seed || key)) together
// with its matching TPMTSensitive structure.
func SoftwareSymmetricObject(tmpl tpm2.TPMTPublic, authValue, seed, key []byte) (tpm2.TPMTPublic, tpm2.TPMTSensitive, error) {
	pub := SanitizePublicForLoadExternal(tmpl)
	pub.ObjectAttributes.SensitiveDataOrigin = false
	var unique []byte
	if pub.NameAlg != tpm2.TPMAlgNull {
		var err error
		unique, err = ComputeHash(pub.NameAlg, seed, key)
		if err != nil {
			return tpm2.TPMTPublic{}, tpm2.TPMTSensitive{}, err
		}
	}
	sens := tpm2.TPMTSensitive{
		SensitiveType: pub.Type,
		AuthValue:     tpm2.TPM2BAuth{Buffer: authValue},
		SeedValue:     tpm2.TPM2BDigest{Buffer: seed},
	}
	switch pub.Type {
	case tpm2.TPMAlgSymCipher:
		pub.Unique = tpm2.NewTPMUPublicID(tpm2.TPMAlgSymCipher, &tpm2.TPM2BDigest{Buffer: unique})
		sens.Sensitive = tpm2.NewTPMUSensitiveComposite(tpm2.TPMAlgSymCipher, &tpm2.TPM2BSymKey{Buffer: key})
	case tpm2.TPMAlgKeyedHash:
		pub.Unique = tpm2.NewTPMUPublicID(tpm2.TPMAlgKeyedHash, &tpm2.TPM2BDigest{Buffer: unique})
		sens.Sensitive = tpm2.NewTPMUSensitiveComposite(tpm2.TPMAlgKeyedHash, &tpm2.TPM2BSensitiveData{Buffer: key})
	default:
		return tpm2.TPMTPublic{}, tpm2.TPMTSensitive{}, errors.Errorf("unsupported symmetric/keyed-hash object type 0x%x", uint16(pub.Type))
	}
	return pub, sens, nil
}

// ComputeEntityName computes the TPM2BName of pub (nameAlg || H_nameAlg(Marshal(pub))).
func ComputeEntityName(pub tpm2.TPMTPublic) (tpm2.TPM2BName, error) {
	name, err := tpm2.ObjectName(&pub)
	if err != nil {
		return tpm2.TPM2BName{}, errors.Wrap(err, "failed to compute object Name")
	}
	return *name, nil
}

// TPMPublicToRSA extracts a Go *rsa.PublicKey from pub.
func TPMPublicToRSA(pub tpm2.TPMTPublic) (*rsa.PublicKey, error) {
	k, err := tpm2.Pub(pub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert TPMTPublic to RSA public key")
	}
	rsaPub, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("TPMTPublic is not an RSA key")
	}
	return rsaPub, nil
}

// TPMPublicToECDSA extracts a Go *ecdsa.PublicKey from pub.
func TPMPublicToECDSA(pub tpm2.TPMTPublic) (*ecdsa.PublicKey, error) {
	k, err := tpm2.Pub(pub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert TPMTPublic to ECDSA public key")
	}
	eccPub, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("TPMTPublic is not an ECC key")
	}
	return eccPub, nil
}

// VerifySignatureSoftware verifies a TPMTSignature (RSASSA, RSAPSS, or ECDSA)
// over digest in software using the public key in pub.
func VerifySignatureSoftware(pub *tpm2.TPMTPublic, digest []byte, sig tpm2.TPMTSignature) error {
	if pub == nil {
		return errors.New("nil public key")
	}
	switch pub.Type {
	case tpm2.TPMAlgRSA:
		rsaPub, err := TPMPublicToRSA(*pub)
		if err != nil {
			return err
		}
		switch sig.SigAlg {
		case tpm2.TPMAlgRSASSA:
			rsaSig, err := sig.Signature.RSASSA()
			if err != nil {
				return errors.Wrap(err, "failed to extract RSASSA signature")
			}
			h, err := rsaSig.Hash.Hash()
			if err != nil {
				return errors.Wrapf(err, "unsupported RSASSA hash algorithm 0x%x", uint16(rsaSig.Hash))
			}
			if err := rsa.VerifyPKCS1v15(rsaPub, h, digest, rsaSig.Sig.Buffer); err != nil {
				return errors.Wrap(err, "software RSASSA signature verification failed")
			}
			return nil
		case tpm2.TPMAlgRSAPSS:
			rsaSig, err := sig.Signature.RSAPSS()
			if err != nil {
				return errors.Wrap(err, "failed to extract RSAPSS signature")
			}
			h, err := rsaSig.Hash.Hash()
			if err != nil {
				return errors.Wrapf(err, "unsupported RSAPSS hash algorithm 0x%x", uint16(rsaSig.Hash))
			}
			if err := rsa.VerifyPSS(rsaPub, h, digest, rsaSig.Sig.Buffer, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto}); err != nil {
				return errors.Wrap(err, "software RSAPSS signature verification failed")
			}
			return nil
		default:
			return errors.Errorf("unexpected RSA signature algorithm 0x%x", uint16(sig.SigAlg))
		}
	case tpm2.TPMAlgECC:
		eccPub, err := TPMPublicToECDSA(*pub)
		if err != nil {
			return err
		}
		eccSig, err := sig.Signature.ECDSA()
		if err != nil {
			return errors.Wrap(err, "failed to extract ECDSA signature")
		}
		r := new(big.Int).SetBytes(eccSig.SignatureR.Buffer)
		s := new(big.Int).SetBytes(eccSig.SignatureS.Buffer)
		if !ecdsa.Verify(eccPub, digest, r, s) {
			return errors.New("software ECDSA signature verification failed")
		}
		return nil
	default:
		return errors.Errorf("unsupported public key type 0x%x for software verification", uint16(pub.Type))
	}
}

func validateAESCFBSymDef(sym tpm2.TPMTSymDefObject) (int, error) {
	if sym.Algorithm != tpm2.TPMAlgAES {
		return 0, errors.Errorf("unsupported symmetric wrapper algorithm 0x%x (only TPM_ALG_AES is supported)", uint16(sym.Algorithm))
	}
	mode, err := sym.Mode.AES()
	if err != nil || *mode != tpm2.TPMAlgCFB {
		return 0, errors.New("unsupported symmetric wrapper mode (only TPM_ALG_CFB is supported)")
	}
	bits, err := sym.KeyBits.AES()
	if err != nil {
		return 0, errors.Wrap(err, "failed to read AES key bits from symDef")
	}
	return int(*bits), nil
}

// SensitiveToDuplicate wraps a TPMTSensitive structure for TPM2_Import,
// supporting unencrypted, inner-wrapped only, outer-wrapped only, and
// inner+outer wrapped duplication blobs per the TPM 2.0 Part 1 Duplication
// specification.
func SensitiveToDuplicate(sensitive tpm2.TPMTSensitive, childPub tpm2.TPMTPublic, parentPub *tpm2.TPMTPublic, encryptionKey []byte, symDef *tpm2.TPMTSymDefObject) (duplicate, encSecret []byte, err error) {
	if len(encryptionKey) > 0 && (symDef == nil || symDef.Algorithm == tpm2.TPMAlgNull) {
		return nil, nil, errors.New("encryptionKey provided without a non-null symDef")
	}
	hasInnerWrapper := symDef != nil && symDef.Algorithm != tpm2.TPMAlgNull
	if hasInnerWrapper {
		aesBits, err := validateAESCFBSymDef(*symDef)
		if err != nil {
			return nil, nil, err
		}
		if len(encryptionKey)*8 != aesBits {
			return nil, nil, errors.Errorf("encryptionKey length (%d bits) does not match symDef key size (%d bits)", len(encryptionKey)*8, aesBits)
		}
	}

	name, err := ComputeEntityName(childPub)
	if err != nil {
		return nil, nil, err
	}
	sensBytes := tpm2.Marshal(sensitive)

	// Step 1: Apply inner wrapper if encryptionKey and symDef are provided with a non-null algorithm.
	innerData := sensBytes
	if hasInnerWrapper {
		sens2B := tpm2.Marshal(tpm2.TPM2BDigest{Buffer: sensBytes})
		innerIntegrity, err := ComputeHash(childPub.NameAlg, sens2B, name.Buffer)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to compute inner integrity hash")
		}
		innerPlain := append(tpm2.Marshal(tpm2.TPM2BDigest{Buffer: innerIntegrity}), sens2B...)
		block, err := aes.NewCipher(encryptionKey)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create inner AES cipher")
		}
		innerCipher := make([]byte, len(innerPlain))
		cipher.NewCFBEncrypter(block, make([]byte, block.BlockSize())).XORKeyStream(innerCipher, innerPlain)
		innerData = innerCipher
	}

	// Step 2: Apply outer wrapper if parentPub is provided.
	if parentPub != nil {
		encapKey, err := tpm2.ImportEncapsulationKey(parentPub)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to import parent encapsulation key")
		}
		if !hasInnerWrapper {
			return tpm2.CreateDuplicate(rand.Reader, encapKey, name.Buffer, sensBytes)
		}
		var parentSym tpm2.TPMTSymDefObject
		if rsaDetail, err := parentPub.Parameters.RSADetail(); err == nil {
			parentSym = rsaDetail.Symmetric
		} else if eccDetail, err := parentPub.Parameters.ECCDetail(); err == nil {
			parentSym = eccDetail.Symmetric
		} else {
			return nil, nil, errors.New("unsupported parent key type for outer duplication")
		}
		aesBits, err := validateAESCFBSymDef(parentSym)
		if err != nil {
			return nil, nil, errors.Wrap(err, "invalid parent symmetric definition")
		}

		// When inner wrapper is already applied, innerData is already encrypted_sensitive
		// and does not get an additional 2B length prefix before outer encryption.
		secret, ciphertext, err := encapKey.Encapsulate(rand.Reader, "DUPLICATE")
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to encapsulate outer duplicate seed")
		}
		storageKey, err := KDFa(parentPub.NameAlg, secret, "STORAGE", name.Buffer, nil, aesBits)
		if err != nil {
			return nil, nil, err
		}
		block, err := aes.NewCipher(storageKey)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create outer AES cipher")
		}
		dupSensitive := make([]byte, len(innerData))
		cipher.NewCFBEncrypter(block, make([]byte, block.BlockSize())).XORKeyStream(dupSensitive, innerData)

		integrityKey, err := KDFa(parentPub.NameAlg, secret, "INTEGRITY", nil, nil, HashSize(parentPub.NameAlg)*8)
		if err != nil {
			return nil, nil, err
		}
		outerHMAC, err := ComputeHMAC(parentPub.NameAlg, integrityKey, dupSensitive, name.Buffer)
		if err != nil {
			return nil, nil, err
		}
		dup := append(tpm2.Marshal(tpm2.TPM2BDigest{Buffer: outerHMAC}), dupSensitive...)
		return dup, ciphertext, nil
	}

	// No outer wrapper: if inner wrapper was applied, return innerData directly;
	// otherwise return TPM2B_SENSITIVE marshalled bytes.
	if hasInnerWrapper {
		return innerData, nil, nil
	}
	return tpm2.Marshal(tpm2.TPM2BDigest{Buffer: sensBytes}), nil, nil
}

// ExportPlaintextSensitive exports obj via plaintext TPM2_Duplicate
// (NewParentHandle = TPM_RH_NULL, Symmetric = TPM_ALG_NULL) authorized by a
// PolicyCommandCode(TPM_CC_Duplicate) session on policyAlg, flushes the
// temporary policy session, and unmarshals the exported TPMTSensitive.
func (t *TpmHandle) ExportPlaintextSensitive(obj tpm2.AuthHandle, policyAlg tpm2.TPMAlgID, opts ...tpm2.AuthOption) (*tpm2.TPMTSensitive, error) {
	sess, closeSess, err := t.PolicyCommandCodeSession(policyAlg, 16, tpm2.TPMCCDuplicate, opts...)
	if err != nil {
		return nil, err
	}
	dupCmd := tpm2.Duplicate{
		ObjectHandle:    SessionHandle(obj.Handle, obj.Name, sess),
		NewParentHandle: tpm2.TPMRHNull,
		Symmetric:       NullSymDef(),
	}
	dupResp, err := dupCmd.Execute(t)
	closeSess()
	if err != nil {
		return nil, err
	}
	sens2B, err := tpm2.Unmarshal[tpm2.TPM2BSensitive](dupResp.Duplicate.Buffer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal exported TPM2BSensitive")
	}
	return sens2B.Contents()
}

// CreateAndVerifyTamperedOutPublic creates an object under parent using tmpl and
// verifies that TPM2_Load fails when OutPublic fields are tampered with.
func (t *TpmHandle) CreateAndVerifyTamperedOutPublic(parent tpm2.AuthHandle, tmpl tpm2.TPMTPublic) error {
	createCmd := tpm2.Create{
		ParentHandle: parent,
		InPublic:     tpm2.New2B(tmpl),
	}
	createResp, err := createCmd.Execute(t)
	if err != nil {
		return errors.Wrap(err, "TPM2_Create failed")
	}
	origPub, err := createResp.OutPublic.Contents()
	if err != nil {
		return errors.Wrap(err, "failed to unpack OutPublic")
	}
	return t.VerifyLoadTamperedOutPublic(parent, createResp.OutPrivate, *origPub)
}
