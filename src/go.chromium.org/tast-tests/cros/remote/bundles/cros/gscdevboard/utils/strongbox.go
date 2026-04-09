// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"reflect"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// StrongboxCmd type is 2 bytes
type StrongboxCmd uint16

// Strongbox command codes
const (
	DeviceGetHardwareInfo                 StrongboxCmd = 0x11
	DeviceAddRngEntropy                   StrongboxCmd = 0x12
	DeviceGenerateKey                     StrongboxCmd = 0x13
	DeviceImportKey                       StrongboxCmd = 0x14
	DeviceImportWrappedKey                StrongboxCmd = 0x15
	DeviceUpgradeKey                      StrongboxCmd = 0x16
	DeviceDeleteKey                       StrongboxCmd = 0x17
	DeviceDeleteAllKeys                   StrongboxCmd = 0x18
	DeviceDestroyAttestationIds           StrongboxCmd = 0x19
	DeviceBegin                           StrongboxCmd = 0x1A
	DeviceEarlyBootEnded                  StrongboxCmd = 0x1C
	DeviceConvertStorageKeyToEphemeral    StrongboxCmd = 0x1D
	DeviceGetKeyCharacteristics           StrongboxCmd = 0x1E
	OperationUpdateAad                    StrongboxCmd = 0x31
	OperationUpdate                       StrongboxCmd = 0x32
	OperationFinish                       StrongboxCmd = 0x33
	OperationAbort                        StrongboxCmd = 0x34
	RPCGetHardwareInfo                    StrongboxCmd = 0x41
	RPCGenerateEcdsaP256KeyPair           StrongboxCmd = 0x42
	RPCGenerateCertificateRequest         StrongboxCmd = 0x43
	RPCGenerateCertificateV2Request       StrongboxCmd = 0x44
	SharedSecretGetSharedSecretParameters StrongboxCmd = 0x51
	SharedSecretComputeSharedSecret       StrongboxCmd = 0x52
	SecureClockGenerateTimeStamp          StrongboxCmd = 0x61
	GetRootOfTrustChallenge               StrongboxCmd = 0x71
	GetRootOfTrust                        StrongboxCmd = 0x72
	SendRootOfTrust                       StrongboxCmd = 0x73
	SetHalInfo                            StrongboxCmd = 0x81
	SetBootInfo                           StrongboxCmd = 0x82
	SetAttestationIds                     StrongboxCmd = 0x83
	SetHalVersion                         StrongboxCmd = 0x84
	SetAdditionalAttestationInfo          StrongboxCmd = 0x91
	GetDiceChain                          StrongboxCmd = 0xA0
	SetHalBootInfo                        StrongboxCmd = 0xA1
)

// StrongboxError is 0 for success or -1 to -255 for errors.
type StrongboxError int

// Strongbox response codes
const (
	StrongboxSuccess        StrongboxError = 0
	InvalidArgument         StrongboxError = -38
	UnsupportedTag          StrongboxError = -39
	InvalidTag              StrongboxError = -40
	HardwareNotYetAvailable StrongboxError = -85
	Unimplemented           StrongboxError = -100
)

const strongboxTpmVendorCommand uint32 = 0x20000001
const strongboxTpmVendorResponseCode uint32 = 0x500

const (
	kmTagTypeInvalid  uint32 = 0
	kmTagTypeEnum     uint32 = 1
	kmTagTypeEnumRep  uint32 = 2
	kmTagTypeUint     uint32 = 3
	kmTagTypeUintRep  uint32 = 4
	kmTagTypeUlong    uint32 = 5
	kmTagTypeDate     uint32 = 6
	kmTagTypeBool     uint32 = 7
	kmTagTypeBignum   uint32 = 8
	kmTagTypeBytes    uint32 = 9
	kmTagTypeUlongRep uint32 = 10
)

const kmTagTypeShift uint32 = 28

const (
	kmTypeInvalid  uint32 = (kmTagTypeInvalid << kmTagTypeShift)
	kmTypeEnum     uint32 = (kmTagTypeEnum << kmTagTypeShift)
	kmTypeEnumRep  uint32 = (kmTagTypeEnumRep << kmTagTypeShift)
	kmTypeUint     uint32 = (kmTagTypeUint << kmTagTypeShift)
	kmTypeUintRep  uint32 = (kmTagTypeUintRep << kmTagTypeShift)
	kmTypeUlong    uint32 = (kmTagTypeUlong << kmTagTypeShift)
	kmTypeDate     uint32 = (kmTagTypeDate << kmTagTypeShift)
	kmTypeBool     uint32 = (kmTagTypeBool << kmTagTypeShift)
	kmTypeBignum   uint32 = (kmTagTypeBignum << kmTagTypeShift)
	kmTypeBytes    uint32 = (kmTagTypeBytes << kmTagTypeShift)
	kmTypeUlongRep uint32 = (kmTagTypeUlongRep << kmTagTypeShift)
)

const (
	kmTagInvalid uint32 = 0

	kmTagPurpose                     uint32 = (kmTypeEnumRep | 1)
	kmTagAlgorithm                   uint32 = (kmTypeEnum | 2)
	kmTagKeySize                     uint32 = (kmTypeUint | 3)
	kmTagBlockMode                   uint32 = (kmTypeEnumRep | 4)
	kmTagDigest                      uint32 = (kmTypeEnumRep | 5)
	kmTagPadding                     uint32 = (kmTypeEnumRep | 6)
	kmTagCallerNonce                 uint32 = (kmTypeBool | 7)
	kmTagMinMacLength                uint32 = (kmTypeUint | 8)
	kmTagEcCurve                     uint32 = (kmTypeEnum | 10)
	kmTagRsaPublicExponent           uint32 = (kmTypeUlong | 200)
	kmTagIncludeUniqueID             uint32 = (kmTypeBool | 202)
	kmTagRsaOaepMgfDigest            uint32 = (kmTypeEnumRep | 203)
	kmTagBootloaderOnly              uint32 = (kmTypeBool | 302)
	kmTagRollbackResistance          uint32 = (kmTypeBool | 303)
	kmTagHardwareType                uint32 = (kmTypeEnum | 304)
	kmTagEarlyBootOnly               uint32 = (kmTypeBool | 305)
	kmTagMaxUsesPerBoot              uint32 = (kmTypeUint | 404)
	kmTagUsageCountLimit             uint32 = (kmTypeUint | 405)
	kmTagUserSecureID                uint32 = (kmTypeUlongRep | 502)
	kmTagNoAuthRequired              uint32 = (kmTypeBool | 503)
	kmTagUserAuthType                uint32 = (kmTypeEnum | 504)
	kmTagAuthTimeout                 uint32 = (kmTypeUint | 505)
	kmTagTrustedUserPresenceRequired uint32 = (kmTypeBool | 507)
	kmTagTrustedConfirmationRequired uint32 = (kmTypeBool | 508)
	kmTagUnlockedDeviceRequired      uint32 = (kmTypeBool | 509)
	kmTagOrigin                      uint32 = (kmTypeEnum | 702)
	kmTagOsVersion                   uint32 = (kmTypeUint | 705)
	kmTagOsPatchlevel                uint32 = (kmTypeUint | 706)
	kmTagUniqueID                    uint32 = (kmTypeBytes | 707)
	kmTagVendorPatchlevel            uint32 = (kmTypeUint | 718)
	kmTagBootPatchlevel              uint32 = (kmTypeUint | 719)
	kmTagDeviceUniqueAttestation     uint32 = (kmTypeBool | 720)
	kmTagIDentityCredentialKey       uint32 = (kmTypeBool | 721)
	kmTagStorageKey                  uint32 = (kmTypeBool | 722)
	kmTagMacLength                   uint32 = (kmTypeUint | 1003)
	kmTagMaxBootLevel                uint32 = (kmTypeUint | 1010)

	kmTagActiveDatetime            uint32 = (kmTypeDate | 400)
	kmTagOriginationExpireDatetime uint32 = (kmTypeDate | 401)
	kmTagUsageExpireDatetime       uint32 = (kmTypeDate | 402)
	kmTagUserID                    uint32 = (kmTypeUint | 501)
	kmTagAllowWhileOnBody          uint32 = (kmTypeBool | 506)
	kmTagCreationDatetime          uint32 = (kmTypeDate | 701)
	kmTagAttestationApplicationID  uint32 = (kmTypeBytes | 709)

	kmTagMinSecondsBetweenOps      uint32 = (kmTypeUint | 403)
	kmTagApplicationID             uint32 = (kmTypeBytes | 601)
	kmTagApplicationData           uint32 = (kmTypeBytes | 700)
	kmTagRootOfTrust               uint32 = (kmTypeBytes | 704)
	kmTagAttestationChallenge      uint32 = (kmTypeBytes | 708)
	kmTagAttestationIDBrand        uint32 = (kmTypeBytes | 710)
	kmTagAttestationIDDevice       uint32 = (kmTypeBytes | 711)
	kmTagAttestationIDProduct      uint32 = (kmTypeBytes | 712)
	kmTagAttestationIDSerial       uint32 = (kmTypeBytes | 713)
	kmTagAttestationIDImei         uint32 = (kmTypeBytes | 714)
	kmTagAttestationIDMeid         uint32 = (kmTypeBytes | 715)
	kmTagAttestationIDManufacturer uint32 = (kmTypeBytes | 716)
	kmTagAttestationIDModel        uint32 = (kmTypeBytes | 717)
	kmTagAttestationIDSecondImei   uint32 = (kmTypeBytes | 723)
	kmTagModuleHash                uint32 = (kmTypeBytes | 724)
	kmTagAssociatedData            uint32 = (kmTypeBytes | 1000)
	kmTagNonce                     uint32 = (kmTypeBytes | 1001)
	kmTagResetSinceIDRotation      uint32 = (kmTypeBool | 1004)
	kmTagConfirmationToken         uint32 = (kmTypeBytes | 1005)
	kmTagCertificateSerial         uint32 = (kmTypeBignum | 1006)
	kmTagCertificateSubject        uint32 = (kmTypeBytes | 1007)
	kmTagCertificateNotBefore      uint32 = (kmTypeDate | 1008)
	kmTagCertificateNotAfter       uint32 = (kmTypeDate | 1009)
)

const (
	kmAlgNone uint32 = 0
	kmAlgRsa  uint32 = 1
	kmAlgEc   uint32 = 3
	kmAlgAes  uint32 = 32
	kmAlgTdes uint32 = 33
	kmAlgHmac uint32 = 128
)

const (
	kmPurposeEncrypt   uint32 = 0
	kmPurposeDecrypt   uint32 = 1
	kmPurposeSign      uint32 = 2
	kmPurposeVerify    uint32 = 3
	kmPurposeWrapKey   uint32 = 5
	kmPurposeAgreeKey  uint32 = 6
	kmPurposeAttestKey uint32 = 7
)

const (
	kmEcCurveP224 uint32 = 0
	kmEcCurveP256 uint32 = 1
	kmEcCurveP384 uint32 = 2
	kmEcCurveP521 uint32 = 3
)

const (
	kmSecuritySoftware           uint32 = 0
	kmSecurityTrustedEnvironment uint32 = 1
	kmSecurityStrongbox          uint32 = 2
	kmSecurityKeystore           uint32 = 100
)

const (
	kmOriginGenerated        uint32 = 0
	kmOriginDerived          uint32 = 1
	kmOriginImported         uint32 = 2
	kmOriginReserved         uint32 = 3
	kmOriginSecurelyImported uint32 = 4
)

const (
	kmDigestNone    uint32 = 0
	kmDigestMd5     uint32 = 1
	kmDigestSha1    uint32 = 2
	kmDigestSha2224 uint32 = 3
	kmDigestSha2256 uint32 = 4
	kmDigestSha2384 uint32 = 5
	kmDigestSha2512 uint32 = 6
)

// StrongboxCommand sends a command and returns the response.
func StrongboxCommand(ctx context.Context, tpm *TpmHelper, command StrongboxCmd, params []byte) (sbErr StrongboxError, response []byte, err error) {
	var buf []byte
	buf = binary.BigEndian.AppendUint16(buf, 0x8001)                 // TPM_ST_NO_SESSIONS
	buf = binary.BigEndian.AppendUint32(buf, uint32(12+len(params))) // size
	buf = binary.BigEndian.AppendUint32(buf, strongboxTpmVendorCommand)
	buf = binary.BigEndian.AppendUint16(buf, uint16(command))
	buf = append(buf, params...)
	response, err = tpm.Send(buf)
	if err != nil {
		return sbErr, response, errors.Wrap(err, "failed to send")
	}
	if len(response) < 12 {
		return sbErr, response, errors.Errorf("Response too small: %v", response)
	}
	rc := binary.BigEndian.Uint32(response[6:10])
	if rc == 0 {
		sbErr = StrongboxError(0)
	} else {
		if rc < strongboxTpmVendorResponseCode {
			return sbErr, response, errors.Errorf("Wrong response code: 0x%x", rc)
		}
		sbErr = -StrongboxError(rc - strongboxTpmVendorResponseCode)
	}
	testing.ContextLogf(ctx, "command 0x%x -> %d", command, sbErr)
	testing.ContextLogf(ctx, "  command %x", buf)
	testing.ContextLogf(ctx, "  response %x", response)
	return sbErr, response[12:], nil
}

func align(len uint32) uint32 {
	if len%4 == 0 {
		return 0
	}
	return 4 - (len % 4)
}

// StrongboxHardwareInfo sends DeviceGetHardwareInfo.
func StrongboxHardwareInfo(ctx context.Context, tpm *TpmHelper) (err error) {
	sbErr, response, err := StrongboxCommand(ctx, tpm, DeviceGetHardwareInfo, nil)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	version := binary.BigEndian.Uint32(response[0:4])
	securityLevel := binary.BigEndian.Uint32(response[4:8])
	len1 := binary.BigEndian.Uint32(response[8:12])
	name := response[12 : 12+len1]
	data2 := response[12+len1+align(len1):]
	len2 := binary.BigEndian.Uint32(data2[0:4])
	author := data2[4 : 4+len2]
	data3 := data2[4+len2+align(len2):]
	timestampToken := binary.BigEndian.Uint32(data3[0:4])
	leftover := len(data3) - 4
	if leftover != 0 {
		err = errors.Errorf("Leftover data: %d", leftover)
		return
	}

	testing.ContextLogf(ctx, "version %d", version)
	testing.ContextLogf(ctx, "securityLevel %d", securityLevel)
	testing.ContextLogf(ctx, "name %s", name)
	testing.ContextLogf(ctx, "author %s", author)
	testing.ContextLogf(ctx, "timestampToken %d", timestampToken)
	return
}

// StrongboxSetHalBootInfo sends SetHalBootInfo.
func StrongboxSetHalBootInfo(ctx context.Context, tpm *TpmHelper, osVersion, osPatchlevel, vendorPatchlevel, bootPatchlevel uint32) (err error) {
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, osVersion)
	buf = binary.LittleEndian.AppendUint32(buf, osPatchlevel)
	buf = binary.LittleEndian.AppendUint32(buf, vendorPatchlevel)
	buf = binary.LittleEndian.AppendUint32(buf, bootPatchlevel)
	sbErr, response, err := StrongboxCommand(ctx, tpm, SetHalBootInfo, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	leftover := len(response)
	if leftover != 0 {
		err = errors.Errorf("Leftover data: %d", leftover)
		return
	}
	return
}

// StrongboxGetDiceChain sends GetDiceChain.
func StrongboxGetDiceChain(ctx context.Context, tpm *TpmHelper) (diceChain []byte, err error) {
	sbErr, response, err := StrongboxCommand(ctx, tpm, GetDiceChain, nil)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	diceChain = response
	return
}

// StrongboxRPCGenerateKey sends RPCGenerateEcdsaP256KeyPair.
func StrongboxRPCGenerateKey(ctx context.Context, tpm *TpmHelper) (blob, macedKey []byte, err error) {
	sbErr, response, err := StrongboxCommand(ctx, tpm, RPCGenerateEcdsaP256KeyPair, nil)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	len1 := binary.LittleEndian.Uint32(response[0:4]) * 4
	blob = response[:4+len1]
	data2 := response[4+len1:]
	len2 := binary.LittleEndian.Uint32(data2[0:4])
	macedKey = data2[4 : 4+len2]
	leftover := len(data2) - int(4+len2)
	if leftover != int(align(len2)) {
		err = errors.Errorf("Leftover data: %d", leftover)
		return
	}

	testing.ContextLogf(ctx, "blob len=%d %x", len1, blob)
	testing.ContextLogf(ctx, "macedKey len=%d %x", len2, macedKey)
	return
}

func appendAlignedBytes(buf, bytes []byte) []byte {
	len := uint32(len(bytes))
	buf = binary.LittleEndian.AppendUint32(buf, len)
	buf = append(buf, bytes...)
	return append(buf, make([]byte, align(len))...)
}

// StrongboxRPCGenerateCertificate sends RPCGenerateCertificateV2Request.
func StrongboxRPCGenerateCertificate(ctx context.Context, tpm *TpmHelper, macedKeys [][]byte, challenge, deviceInfo []byte) (csr []byte, err error) {
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(macedKeys)))
	for _, k := range macedKeys {
		buf = appendAlignedBytes(buf, k)
	}
	buf = appendAlignedBytes(buf, challenge)
	buf = appendAlignedBytes(buf, deviceInfo)
	sbErr, response, err := StrongboxCommand(ctx, tpm, RPCGenerateCertificateV2Request, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	csr = response
	return
}

func appendBytesTag(tags []byte, tag uint32, bytes []byte) []byte {
	// Encode length into tag and pad value to 32-bit
	len := uint32(len(bytes))
	tags = binary.LittleEndian.AppendUint32(tags, tag|len<<16)
	tags = append(tags, bytes...)
	return append(tags, make([]byte, align(len))...)
}

// StrongboxGenerateKey sends DeviceGenerateKey.
func StrongboxGenerateKey(ctx context.Context, tpm *TpmHelper, attestKey []byte) (blob, cert []byte, err error) {
	// Issuer is ASN1:
	// SEQUENCE 0x30 <size = 0x1f>
	// SET 0x31 <size = 0x1d>
	// SEQUENCE 0x30 <size = 0x1b>
	// OBJECT 0x06 <size = 0x03> 0x55 0x04 0x03 (:commonName)
	// PRINTABLESTRING 0x13 <size = 0x14> "Android Keystore Key"
	issuer, _ := hex.DecodeString("301f311d301b06035504031314416e64726f6964204b657973746f7265204b6579")
	var tags []byte
	haveAttestKey := attestKey != nil
	if !haveAttestKey {
		tags = binary.LittleEndian.AppendUint32(tags, kmTagAlgorithm)
		tags = binary.LittleEndian.AppendUint32(tags, kmAlgEc)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagKeySize)
		tags = binary.LittleEndian.AppendUint32(tags, 256)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagPurpose)
		tags = binary.LittleEndian.AppendUint32(tags, kmPurposeSign)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagDigest)
		tags = binary.LittleEndian.AppendUint32(tags, kmDigestSha2256)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagDigest)
		tags = binary.LittleEndian.AppendUint32(tags, kmDigestSha2512)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagAllowWhileOnBody)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagUserID)
		tags = binary.LittleEndian.AppendUint32(tags, 0xf00)
		tags = appendBytesTag(tags, kmTagApplicationID, []byte("\xaa\xaa\xaa\xaa"))
		tags = appendBytesTag(tags, kmTagApplicationData, []byte("\xbb\xbb\xbb\xbb"))
		attestKey = binary.LittleEndian.AppendUint32(attestKey, 0)
	} else {
		tags = binary.LittleEndian.AppendUint32(tags, kmTagAlgorithm)
		tags = binary.LittleEndian.AppendUint32(tags, kmAlgEc)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagKeySize)
		tags = binary.LittleEndian.AppendUint32(tags, 256)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagPurpose)
		tags = binary.LittleEndian.AppendUint32(tags, kmPurposeSign)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagDigest)
		tags = binary.LittleEndian.AppendUint32(tags, kmDigestSha2256)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagDigest)
		tags = binary.LittleEndian.AppendUint32(tags, kmDigestSha2512)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagAllowWhileOnBody)
		tags = binary.LittleEndian.AppendUint32(tags, kmTagUserID)
		tags = binary.LittleEndian.AppendUint32(tags, 0xf00)
		tags = appendBytesTag(tags, kmTagCertificateSubject, issuer)
		tags = appendBytesTag(tags, kmTagCertificateSerial, []byte("\x01\x23\x45\x67\x89\xab\xcd\xef01234567"))
		tags = appendBytesTag(tags, kmTagApplicationID, []byte("\xaa\xaa\xaa\xaa"))
		tags = appendBytesTag(tags, kmTagApplicationData, []byte("\xbb\xbb\xbb\xbb"))
		tags = binary.LittleEndian.AppendUint32(tags, kmTagNoAuthRequired)
		tags = appendBytesTag(tags, kmTagAttestationChallenge, []byte("2025-11-05T19:57:14.294Z"))
		tags = appendBytesTag(tags, kmTagAttestationApplicationID, []byte("com.google.android.gms"))
		tags = appendBytesTag(tags, kmTagAttestationIDBrand, []byte("google"))
		tags = appendBytesTag(tags, kmTagAttestationIDDevice, []byte("brya"))
		tags = appendBytesTag(tags, kmTagAttestationIDProduct, []byte("brya"))
		tags = appendBytesTag(tags, kmTagAttestationIDSerial, []byte("5CD5231RQK"))
		tags = appendBytesTag(tags, kmTagAttestationIDManufacturer, []byte("Google"))
		tags = appendBytesTag(tags, kmTagAttestationIDModel, []byte("Brya"))
	}
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(tags)/4))
	buf = append(buf, tags...)
	buf = append(buf, attestKey...)
	buf = appendAlignedBytes(buf, issuer)
	sbErr, response, err := StrongboxCommand(ctx, tpm, DeviceGenerateKey, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	len1 := binary.LittleEndian.Uint32(response[0:4])*4 + 4
	blob = response[:len1]
	data2 := response[len1:]
	len2 := binary.LittleEndian.Uint32(data2[0:4])
	cert = data2[4 : 4+len2]
	leftover := len(data2) - int(4+len2)
	if leftover != int(align(len2)) {
		err = errors.Errorf("Leftover data: %d", leftover)
		return
	}
	testing.ContextLogf(ctx, "blob len=%d %x", len1, blob)
	testing.ContextLogf(ctx, "cert len=%d %x", len2, cert)

	// blob contents should be:
	// word 0: total length of key blob in words
	// word 1: KM_SECURITY_STRONGBOX
	// word 2: length of hw tags in words
	// .. hw tags
	// word 3+n: KM_SECURITY_KEYSTORE
	// word 4+n: length of sw tags in words
	// .. sw tags

	hwStart := binary.LittleEndian.Uint32(blob[4:8])
	hwLen := binary.LittleEndian.Uint32(blob[8:12])
	swTags := blob[12+hwLen*4:]
	swStart := binary.LittleEndian.Uint32(swTags[0:4])
	swLen := binary.LittleEndian.Uint32(swTags[4:8])
	wantLen := 12 + hwLen*4 + 8 + swLen*4
	if uint32(len(blob)) < wantLen {
		err = errors.Errorf("expected blob len: %d", wantLen)
		return
	}
	if hwStart != kmSecurityStrongbox {
		err = errors.Errorf("Wrong tag: got 0x%04x want 0x%04x", hwStart, kmSecurityStrongbox)
		return
	}
	if swStart != kmSecurityKeystore {
		err = errors.Errorf("Wrong tag: got 0x%04x want 0x%04x", swStart, kmSecurityKeystore)
		return
	}
	hwTags := blob[12 : 12+hwLen*4]
	testing.ContextLogf(ctx, "HW tags: len=%d %x", hwLen, hwTags)
	swTags2 := swTags[8 : 8+swLen*4]
	testing.ContextLogf(ctx, "SW tags: len=%d %x", swLen, swTags2)
	blob2 := swTags[8+swLen*4:]
	len3 := binary.LittleEndian.Uint32(blob2[0:4]) + 4
	testing.ContextLogf(ctx, "blob: len=%d %x", len3, blob2)
	if uint32(len(blob2)) != len3 {
		err = errors.Errorf("Wrong inner blob len: got %d want %d", len(blob2), len3)
		return
	}

	if haveAttestKey {
		if hwLen != 21 {
			err = errors.Errorf("Wrong HW len: %d", hwLen)
			return
		}
		var want []byte
		want = binary.LittleEndian.AppendUint32(want, kmTagOrigin)
		want = binary.LittleEndian.AppendUint32(want, kmOriginGenerated)
		want = binary.LittleEndian.AppendUint32(want, kmTagOsVersion)
		want = binary.LittleEndian.AppendUint32(want, 0x027100)
		want = binary.LittleEndian.AppendUint32(want, kmTagOsPatchlevel)
		want = binary.LittleEndian.AppendUint32(want, 0x031710)
		want = binary.LittleEndian.AppendUint32(want, kmTagVendorPatchlevel)
		want = binary.LittleEndian.AppendUint32(want, 0x013502450)
		want = binary.LittleEndian.AppendUint32(want, kmTagBootPatchlevel)
		want = binary.LittleEndian.AppendUint32(want, 0x01350245)
		want = binary.LittleEndian.AppendUint32(want, kmTagAlgorithm)
		want = binary.LittleEndian.AppendUint32(want, kmAlgEc)
		want = binary.LittleEndian.AppendUint32(want, kmTagKeySize)
		want = binary.LittleEndian.AppendUint32(want, 256)
		want = binary.LittleEndian.AppendUint32(want, kmTagPurpose)
		want = binary.LittleEndian.AppendUint32(want, kmPurposeSign)
		want = binary.LittleEndian.AppendUint32(want, kmTagDigest)
		want = binary.LittleEndian.AppendUint32(want, kmDigestSha2256)
		want = binary.LittleEndian.AppendUint32(want, kmTagDigest)
		want = binary.LittleEndian.AppendUint32(want, kmDigestSha2512)
		want = binary.LittleEndian.AppendUint32(want, kmTagNoAuthRequired)
		if !bytes.Equal(hwTags, want) {
			err = errors.Errorf("Wrong HW tags: want %x", want)
			return
		}
		if swLen != 3 {
			err = errors.Errorf("Wrong SW len: %d", swLen)
			return
		}
		want = binary.LittleEndian.AppendUint32(nil, kmTagAllowWhileOnBody)
		want = binary.LittleEndian.AppendUint32(want, kmTagUserID)
		want = binary.LittleEndian.AppendUint32(want, 0xf00)
		if !bytes.Equal(swTags2, want) {
			err = errors.Errorf("Wrong SW tags: want %x", want)
			return
		}
	}
	return
}

// StrongboxBegin sends DeviceBegin.
func StrongboxBegin(ctx context.Context, tpm *TpmHelper, blob []byte) (operationID []byte, err error) {
	var buf []byte
	buf = binary.LittleEndian.AppendUint32(buf, kmPurposeSign)
	buf = append(buf, blob...)
	buf = binary.LittleEndian.AppendUint32(buf, 8)
	buf = appendBytesTag(buf, kmTagApplicationID, []byte("\xaa\xaa\xaa\xaa"))
	buf = appendBytesTag(buf, kmTagApplicationData, []byte("\xbb\xbb\xbb\xbb"))
	buf = binary.LittleEndian.AppendUint32(buf, kmTagDigest)
	buf = binary.LittleEndian.AppendUint32(buf, kmDigestSha2256)
	buf = binary.LittleEndian.AppendUint32(buf, kmTagAlgorithm)
	buf = binary.LittleEndian.AppendUint32(buf, kmAlgEc)
	sbErr, response, err := StrongboxCommand(ctx, tpm, DeviceBegin, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	if len(response) != 16 {
		err = errors.Errorf("Wrong response length: %v", response)
		return
	}
	operationID = response[12:16]
	return
}

// StrongboxUpdate sends OperationUpdate.
func StrongboxUpdate(ctx context.Context, tpm *TpmHelper, operationID, input []byte) (err error) {
	var buf []byte
	buf = append(buf, operationID...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(input)))
	buf = append(buf, input...)
	sbErr, response, err := StrongboxCommand(ctx, tpm, OperationUpdate, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	if len(response) != 0 {
		err = errors.Errorf("Wrong response length: %v", response)
		return
	}
	return
}

// StrongboxFinish sends OperationFinish.
func StrongboxFinish(ctx context.Context, tpm *TpmHelper, operationID, input []byte) (signature []byte, err error) {
	var buf []byte
	buf = append(buf, operationID...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(input)))
	buf = append(buf, input...)
	sbErr, response, err := StrongboxCommand(ctx, tpm, OperationFinish, buf)
	if err != nil {
		return
	}
	if sbErr != StrongboxSuccess {
		err = errors.Errorf("Command failed: %v", sbErr)
		return
	}
	if len(response) != 64 {
		err = errors.Errorf("Wrong response length: %v", response)
		return
	}
	signature = response
	testing.ContextLogf(ctx, "signature %x", signature)
	return
}

const (
	cborMajorUint   uint8 = (0 << 5)
	cborMajorNint   uint8 = (1 << 5)
	cborMajorBstr   uint8 = (2 << 5)
	cborMajorTstr   uint8 = (3 << 5)
	cborMajorArr    uint8 = (4 << 5)
	cborMajorMap    uint8 = (5 << 5)
	cborMajorTag    uint8 = (6 << 5)
	cborMajorSimple uint8 = (7 << 5)
	cborMajorMask   uint8 = (7 << 5)
	cborValueMask   uint8 = 0x1f
)

const ecdsaPointBytes int = 32
const ecdsaSigBytes int = 2 * ecdsaPointBytes
const cborPublicKeyLen int = 13 + ecdsaSigBytes
const sha256DigestSize int = 32

type cborChecker struct {
	data       []byte
	lastHeader []byte
}

func newCborChecker(data []byte) *cborChecker {
	return &cborChecker{
		data:       data,
		lastHeader: nil,
	}
}

func (c *cborChecker) headerMajor() (uint8, error) {
	if len(c.data) == 0 {
		return 0, errors.New("no data left")
	}
	m := c.data[0] & cborMajorMask
	return m, nil
}

func (c *cborChecker) checkHeaderMajor(want uint8) error {
	m, err := c.headerMajor()
	if err != nil {
		return err
	}
	if m != want {
		return errors.Errorf("Wrong type: got 0x%x want 0x%x", m, want)
	}
	return nil
}

func (c *cborChecker) takeData(count int) []byte {
	d := c.data[:count]
	c.data = c.data[count:]
	return d
}

func (c *cborChecker) takeHeader(count int) []byte {
	d := c.takeData(count)
	c.lastHeader = append(c.lastHeader, d...)
	return d
}

func (c *cborChecker) headerValue() (int, error) {
	c.lastHeader = nil
	d := c.takeHeader(1)[0]
	v := int(d & cborValueMask)
	if v < 24 {
		return v, nil
	}
	if v == 24 {
		return int(c.takeHeader(1)[0]), nil
	}
	if v == 25 {
		return int(binary.BigEndian.Uint16(c.takeHeader(2))), nil
	}
	if v == 26 {
		return int(binary.BigEndian.Uint32(c.takeHeader(4))), nil
	}
	if v == 27 {
		return int(binary.BigEndian.Uint64(c.takeHeader(8))), nil
	}
	return 0, errors.Errorf("invalid header value %v", v)
}

func (c *cborChecker) checkHeader(major uint8, want int) error {
	if err := c.checkHeaderMajor(major); err != nil {
		return err
	}
	v, err := c.headerValue()
	if err != nil {
		return err
	}
	if v != want {
		return errors.Errorf("Wrong value: got %v want %v", v, want)
	}
	return nil
}

func (c *cborChecker) uint(want int) error {
	return c.checkHeader(cborMajorUint, want)
}

func (c *cborChecker) nint(want int) error {
	return c.checkHeader(cborMajorNint, want)
}

func (c *cborChecker) takeBytesValue(major uint8) ([]byte, error) {
	if err := c.checkHeaderMajor(major); err != nil {
		return nil, err
	}
	n, err := c.headerValue()
	if err != nil {
		return nil, err
	}
	return c.takeData(n), nil
}

func (c *cborChecker) bytes() ([]byte, error) {
	return c.takeBytesValue(cborMajorBstr)
}

func (c *cborChecker) text() ([]byte, error) {
	return c.takeBytesValue(cborMajorTstr)
}

func (c *cborChecker) array(want int) error {
	return c.checkHeader(cborMajorArr, want)
}

func (c *cborChecker) cmap(want int) error {
	return c.checkHeader(cborMajorMap, want)
}

func (c *cborChecker) takeItemAsCbor() ([]byte, error) {
	m, err := c.headerMajor()
	if err != nil {
		return nil, err
	}
	n, err := c.headerValue()
	if err != nil {
		return nil, err
	}
	cb := bytes.Clone(c.lastHeader)
	items := 0
	switch m {
	case cborMajorBstr, cborMajorTstr:
		cb = append(cb, c.takeData(n)...)
	case cborMajorArr:
		items = n
	case cborMajorMap:
		items = n * 2
	}
	for range items {
		i, err := c.takeItemAsCbor()
		if err != nil {
			return nil, err
		}
		cb = append(cb, i...)
	}
	return cb, nil
}

// checkSignedData checks SignedData in COSE CBOR encoding.
// SignedData = [   # array(4)
//
//	protected,   # bytes(3) (Algorithm ES256 = A10126)
//	unprotected, # map(0)
//	payload,     # bytes()
//	signature,   # bytes(64) (ES256)
//
// ]
func (c *cborChecker) checkSignedData(ctx context.Context, label string, pub *ecdsa.PublicKey) ([]byte, error) {
	if err := c.array(4); err != nil {
		return nil, err
	}
	b, err := c.bytes()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(b, []byte{0xA1, 0x01, 0x26}) {
		return nil, errors.Errorf("Wrong bytes: %v", b)
	}
	if err = c.cmap(0); err != nil {
		return nil, err
	}
	payload, err := c.bytes()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	// Fixed prefix for signing: see Cr50 kSigStructFixedHdr.
	sigHeader, _ := hex.DecodeString("846a5369676E61747572653143A1012640")
	h.Write(sigHeader)
	h.Write(c.lastHeader)
	h.Write(payload)
	sig, err := c.bytes()
	if err != nil {
		return nil, err
	}
	if len(sig) != ecdsaSigBytes {
		return nil, errors.Errorf("Wrong length: %v", len(b))
	}
	err = CheckSignature(ctx, label, pub, h.Sum(nil), sig)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// checkPublicKey checks PublicKey in COSE CBOR encoding.
// PublicKey = {  # map(6)
//
//	key_type:  # unsigned(1): unsigned(2)
//	alg:       # unsigned(3): negative(6)
//	key_ops:   # unsigned(4): array(1) [ unsigned(2) ]
//	curve:     # negative(0): unsigned(1)
//	x:         # negative(1): bytes(32)
//	y:         # negative(2): bytes(32)
//
// }
func (c *cborChecker) checkPublicKey(withKeyOps bool) (*ecdsa.PublicKey, error) {
	mapSize := 5
	if withKeyOps {
		mapSize++
	}
	if err := c.cmap(mapSize); err != nil {
		return nil, err
	}
	if err := c.uint(1); err != nil {
		return nil, err
	}
	if err := c.uint(2); err != nil {
		return nil, err
	}
	if err := c.uint(3); err != nil {
		return nil, err
	}
	if err := c.nint(6); err != nil {
		return nil, err
	}
	if withKeyOps {
		if err := c.uint(4); err != nil {
			return nil, err
		}
		if err := c.array(1); err != nil {
			return nil, err
		}
		if err := c.uint(2); err != nil {
			return nil, err
		}
	}
	if err := c.nint(0); err != nil {
		return nil, err
	}
	if err := c.uint(1); err != nil {
		return nil, err
	}
	if err := c.nint(1); err != nil {
		return nil, err
	}
	x, err := c.bytes()
	if err != nil {
		return nil, err
	}
	if len(x) != ecdsaPointBytes {
		return nil, errors.Errorf("Wrong X length: %v", len(x))
	}
	if err := c.nint(2); err != nil {
		return nil, err
	}
	y, err := c.bytes()
	if err != nil {
		return nil, err
	}
	if len(y) != ecdsaPointBytes {
		return nil, errors.Errorf("Wrong Y length: %v", len(y))
	}
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}
	return pubKey, nil
}

// checkCert checks Cert in COSE CBOR encoding.
// Cert = {  # map(10)
//
//	issuer:       # unsigned(1): text()
//	subject:      # unsigned(2): text()
//	code_hash:    # negative(4670544): bytes()
//	cfg_hash:     # negative(4670546): bytes()
//	cfg_desc:     # negative(4670547): bytes()
//	auth_hash:    # negative(4670548): bytes()
//	mode:         # negative(4670550): bytes()
//	subject_pk:   # negative(4670551): bytes()
//	key_usage:    # negative(4670552): bytes()
//	profile_name: # negative(4670553): text()
//
// }
func (c *cborChecker) checkCert() (*ecdsa.PublicKey, error) {
	if err := c.cmap(10); err != nil {
		return nil, err
	}
	if err := c.uint(1); err != nil {
		return nil, err
	}
	if _, err := c.text(); err != nil {
		return nil, err
	}
	if err := c.uint(2); err != nil {
		return nil, err
	}
	if _, err := c.text(); err != nil {
		return nil, err
	}
	if err := c.nint(4670544); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670546); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670547); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670548); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670550); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670551); err != nil {
		return nil, err
	}
	pk, err := c.bytes()
	if err != nil {
		return nil, err
	}
	if err := c.nint(4670552); err != nil {
		return nil, err
	}
	if _, err := c.bytes(); err != nil {
		return nil, err
	}
	if err := c.nint(4670553); err != nil {
		return nil, err
	}
	if _, err := c.text(); err != nil {
		return nil, err
	}
	cb := newCborChecker(pk)
	pk2, err := cb.checkPublicKey(true)
	if err != nil {
		return nil, err
	}
	if len(cb.data) != 0 {
		return nil, errors.Errorf("Extra data: %v", cb.data)
	}
	return pk2, nil
}

// CheckMacedKeyCbor checks MacedPublicKey in COSE CBOR encoding.
// MacedPublicKey = [  # array(4)
//
//	protected,      # bytes(3) (Algorithm HMAC-256 = A10105)
//	unprotected,    # map(0)
//	payload,        # bytes(77) (PublicKey)
//	tag,            # bytes(32) (HMAC-256)
//
// ]
func CheckMacedKeyCbor(macedKey []byte) (*ecdsa.PublicKey, error) {
	cb := newCborChecker(macedKey)
	if err := cb.array(4); err != nil {
		return nil, err
	}
	b, err := cb.bytes()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(b, []byte{0xA1, 0x01, 0x05}) {
		return nil, errors.Errorf("Wrong bytes: %v", b)
	}
	if err := cb.cmap(0); err != nil {
		return nil, err
	}
	pk, err := cb.bytes()
	if err != nil {
		return nil, err
	}
	b, err = cb.bytes()
	if err != nil {
		return nil, err
	}
	if len(b) != sha256DigestSize {
		return nil, errors.Errorf("Wrong length: %v", len(b))
	}
	if len(cb.data) != 0 {
		return nil, errors.Errorf("Extra data: %v", cb.data)
	}
	cb = newCborChecker(pk)
	pk2, err := cb.checkPublicKey(false)
	if err != nil {
		return nil, err
	}
	if len(cb.data) != 0 {
		return nil, errors.Errorf("Extra data: %v", cb.data)
	}
	return pk2, nil
}

// CheckCsrCbor checks SignedData in COSE CBOR encoding.
func CheckCsrCbor(ctx context.Context, csr []byte, cdiPubKey *ecdsa.PublicKey) (challenge, deviceInfo []byte, keysCount int, err error) {
	cb := newCborChecker(csr)
	payload, err := cb.checkSignedData(ctx, "CSR", cdiPubKey)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(cb.data) != 0 {
		return nil, nil, 0, errors.Errorf("Extra data: %v", cb.data)
	}
	// SignedData payload = [challenge, CsrPayload]
	cb = newCborChecker(payload)
	if err := cb.array(2); err != nil {
		return nil, nil, 0, err
	}
	challenge, err = cb.bytes()
	if err != nil {
		return nil, nil, 0, err
	}
	csrp, err := cb.bytes()
	if err != nil {
		return nil, nil, 0, err
	}
	if len(cb.data) != 0 {
		return nil, nil, 0, errors.Errorf("Extra data: %v", cb.data)
	}
	// CsrPayload = [version: 3, CertificateType: "keymint", DeviceInfo, KeysToSign: [ *PublicKey]]
	cb = newCborChecker(csrp)
	if err := cb.array(4); err != nil {
		return nil, nil, 0, err
	}
	if err := cb.uint(3); err != nil {
		return nil, nil, 0, err
	}
	certType, err := cb.text()
	if err != nil {
		return nil, nil, 0, err
	}
	if string(certType) != "keymint" {
		return nil, nil, 0, errors.Errorf("Wrong certType: %v", certType)
	}
	if err := cb.checkHeaderMajor(cborMajorMap); err != nil {
		return nil, nil, 0, err
	}
	deviceInfo, err = cb.takeItemAsCbor()
	if err != nil {
		return nil, nil, 0, err
	}
	if err := cb.checkHeaderMajor(cborMajorArr); err != nil {
		return nil, nil, 0, err
	}
	keys, err := cb.takeItemAsCbor()
	if err != nil {
		return nil, nil, 0, err
	}
	if len(cb.data) != 0 {
		return nil, nil, 0, errors.Errorf("Extra data: %v", cb.data)
	}
	cb = newCborChecker(keys)
	keysCount, err = cb.headerValue()
	if err != nil {
		return nil, nil, 0, err
	}
	return
}

// CheckDiceChainCbor checks DiceCertChain in COSE CBOR encoding.
// DiceCertChain = [  # array(2)
//
//	pub_key,      # PublicKey
//	cert,         # SignedData
//
// ]
func CheckDiceChainCbor(ctx context.Context, diceChain []byte) (*ecdsa.PublicKey, error) {
	cb := newCborChecker(diceChain)
	if err := cb.array(2); err != nil {
		return nil, err
	}
	udsPub, err := cb.checkPublicKey(true)
	if err != nil {
		return nil, err
	}
	cert, err := cb.checkSignedData(ctx, "DICE cert chain", udsPub)
	if err != nil {
		return nil, err
	}
	if len(cb.data) != 0 {
		return nil, errors.Errorf("Extra data: %v", cb.data)
	}
	cb = newCborChecker(cert)
	cdiPub, err := cb.checkCert()
	if err != nil {
		return nil, err
	}
	if len(cb.data) != 0 {
		return nil, errors.Errorf("Extra data: %v", cb.data)
	}

	return cdiPub, nil
}

// CheckSignature verifies the signature of hash using the public key.
func CheckSignature(ctx context.Context, label string, pub *ecdsa.PublicKey, hash, sig []byte) error {
	testing.ContextLogf(ctx, "Checking %s signature", label)
	testing.ContextLogf(ctx, "Curve: %s", pub.Curve.Params().Name)
	testing.ContextLogf(ctx, "X: %x", pub.X.Bytes())
	testing.ContextLogf(ctx, "Y: %x", pub.Y.Bytes())
	testing.ContextLogf(ctx, "hash: %x", hash)
	testing.ContextLogf(ctx, "R: %x", sig[:32])
	testing.ContextLogf(ctx, "S: %x", sig[32:])
	sigR := new(big.Int).SetBytes(sig[:32])
	sigS := new(big.Int).SetBytes(sig[32:])
	valid := ecdsa.Verify(pub, hash, sigR, sigS)
	testing.ContextLog(ctx, "Signature valid: ", valid)
	if !valid {
		return errors.New("Signature is not valid")
	}
	return nil
}

// DeviceInfo is used for KeyMint RKP (Remote Key Provisioning).
// https://cs.android.com/android/platform/superproject/+/android-latest-release:hardware/interfaces/security/rkp/aidl/android/hardware/security/keymint/DeviceInfoV3.cddl
type DeviceInfo struct {
	Brand            string `json:"brand"`
	Fused            int    `json:"fused"`
	Model            string `json:"model"`
	Device           string `json:"device"`
	Product          string `json:"product"`
	OSVersion        string `json:"os_version"`
	Manufacturer     string `json:"manufacturer"`
	VBMetaDigest     string `json:"vbmeta_digest"`
	BootPatchLevel   int    `json:"boot_patch_level"`
	SystemPatchLevel int    `json:"system_patch_level"`
	VendorPatchLevel int    `json:"vendor_patch_level"`
	SecurityLevel    string `json:"security_level"`
	VBState          string `json:"vb_state"`
	BootloaderState  string `json:"bootloader_state"`
}

// ToCBOR encodes the DeviceInfo as CBOR bytes.
func (c *DeviceInfo) ToCBOR() []byte {
	var buf []byte
	t := reflect.TypeOf(*c)
	v := reflect.ValueOf(*c)
	buf = append(buf, cborMajorMap|uint8(t.NumField()))
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		c.appendBytes(&buf, cborMajorTstr, []byte(tag))
		switch field.Type.Kind() {
		case reflect.String:
			s := v.FieldByName(field.Name).String()
			if tag == "vbmeta_digest" {
				h, err := hex.DecodeString(s)
				if err != nil {
					panic("Invalid hex string")
				}
				c.appendBytes(&buf, cborMajorBstr, h)
			} else {
				c.appendBytes(&buf, cborMajorTstr, []byte(s))
			}
		case reflect.Int:
			c.appendHeader(&buf, cborMajorUint, int(v.FieldByName(field.Name).Int()))
		default:
			panic("Unexpected type")
		}
	}
	return buf
}

func (c *DeviceInfo) appendBytes(buf *[]byte, major uint8, b []byte) {
	c.appendHeader(buf, major, len(b))
	*buf = append(*buf, b...)
}

func (c *DeviceInfo) appendHeader(buf *[]byte, major uint8, value int) {
	if value < 24 {
		*buf = append(*buf, major|uint8(value))
	} else if value < 0x100 {
		*buf = append(*buf, major|24)
		*buf = append(*buf, uint8(value))
	} else if value < 0x10000 {
		*buf = append(*buf, major|25)
		*buf = binary.BigEndian.AppendUint16(*buf, uint16(value))
	} else if value < 0x100000000 {
		*buf = append(*buf, major|26)
		*buf = binary.BigEndian.AppendUint32(*buf, uint32(value))
	} else {
		*buf = append(*buf, major|27)
		*buf = binary.BigEndian.AppendUint64(*buf, uint64(value))
	}
}
