// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
)

// BuildRawCommand constructs a raw TPM 2.0 command frame from explicit components:
// [tag(2) | totalSize(4) | cc(4) | handles(4*N) | authArea | params].
// Note: If authArea is non-empty, it must already include its 4-byte authorizationSize
// header (as returned by BuildPasswordAuthArea or WrapAuthArea).
func BuildRawCommand(tag tpm2.TPMST, cc tpm2.TPMCC, handles []tpm2.TPMHandle, authArea, params []byte) []byte {
	totalSize := 10 + 4*len(handles) + len(authArea) + len(params)
	buf := bytes.NewBuffer(make([]byte, 0, totalSize))
	_ = binary.Write(buf, binary.BigEndian, tag)
	_ = binary.Write(buf, binary.BigEndian, uint32(totalSize))
	_ = binary.Write(buf, binary.BigEndian, cc)
	for _, h := range handles {
		_ = binary.Write(buf, binary.BigEndian, uint32(h))
	}
	_, _ = buf.Write(authArea)
	_, _ = buf.Write(params)
	return buf.Bytes()
}

// BuildPasswordAuthSession serializes a single TPMS_AUTH_COMMAND structure for
// TPM_RS_PW with ContinueSession=true and the given password (without the 4-byte
// authorizationSize prefix).
func BuildPasswordAuthSession(password []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 9+len(password)))
	_ = binary.Write(buf, binary.BigEndian, uint32(tpm2.TPMRSPW))
	_ = binary.Write(buf, binary.BigEndian, uint16(0)) // Empty nonceCaller
	_ = buf.WriteByte(0x01)                            // ContinueSession = 1
	_ = binary.Write(buf, binary.BigEndian, uint16(len(password)))
	_, _ = buf.Write(password)
	return buf.Bytes()
}

// WrapAuthArea concatenates one or more serialized TPMS_AUTH_COMMAND structures
// and prepends the 4-byte big-endian authorizationSize header.
func WrapAuthArea(authSessions ...[]byte) []byte {
	var bodyLen int
	for _, s := range authSessions {
		bodyLen += len(s)
	}
	buf := bytes.NewBuffer(make([]byte, 0, 4+bodyLen))
	_ = binary.Write(buf, binary.BigEndian, uint32(bodyLen))
	for _, s := range authSessions {
		_, _ = buf.Write(s)
	}
	return buf.Bytes()
}

// BuildPasswordAuthArea builds a complete TPM_ST_SESSIONS authorization area
// (including the 4-byte authorizationSize prefix) for one or more password
// authorizations.
func BuildPasswordAuthArea(passwords ...[]byte) []byte {
	sessions := make([][]byte, len(passwords))
	for i, pw := range passwords {
		sessions[i] = BuildPasswordAuthSession(pw)
	}
	return WrapAuthArea(sessions...)
}

// PlaceholderEntityName returns a zero SHA-256 name, used for handles whose
// real name is unknown (cpHash is then wrong, but the command is still sent).
func PlaceholderEntityName() tpm2.TPM2BName {
	buf := make([]byte, 34)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tpm2.TPMAlgSHA256))
	return tpm2.TPM2BName{Buffer: buf}
}

func namedOrRawHandle(h tpm2.TPMHandle) tpm2.NamedHandle {
	if n := h.KnownName(); n != nil {
		return tpm2.NamedHandle{Handle: h, Name: *n}
	}
	return tpm2.NamedHandle{Handle: h, Name: PlaceholderEntityName()}
}

// SendRaw transmits a raw command byte slice to the TPM, parses the 10-byte
// response header into (tag, respSize, rc, body, err), and validates the
// response header length and tag.
func (t *TpmHandle) SendRaw(raw []byte) (tag tpm2.TPMST, respSize uint32, rc tpm2.TPMRC, body []byte, err error) {
	resp, err := t.Send(raw)
	if err != nil {
		return 0, 0, 0, nil, errors.Wrap(err, "transport Send failed")
	}
	if len(resp) < 10 {
		return 0, uint32(len(resp)), 0, resp, errors.Errorf("TPM response too short: got %d bytes, want at least 10", len(resp))
	}
	tag = tpm2.TPMST(binary.BigEndian.Uint16(resp[0:2]))
	respSize = binary.BigEndian.Uint32(resp[2:6])
	rc = tpm2.TPMRC(binary.BigEndian.Uint32(resp[6:10]))
	body = resp[10:]
	if int(respSize) != len(resp) {
		return tag, respSize, rc, body, errors.Errorf("TPM response size header %d does not match actual length %d", respSize, len(resp))
	}
	if tag != tpm2.TPMSTSessions && tag != tpm2.TPMSTNoSessions {
		return tag, respSize, rc, body, errors.Errorf("unexpected TPM response tag 0x%x", uint16(tag))
	}
	if rc == tpm2.TPMRCSuccess && len(raw) >= 2 {
		cmdTag := tpm2.TPMST(binary.BigEndian.Uint16(raw[0:2]))
		if (cmdTag == tpm2.TPMSTSessions || cmdTag == tpm2.TPMSTNoSessions) && tag != cmdTag {
			return tag, respSize, rc, body, errors.Errorf("TPM response tag 0x%x does not match command tag 0x%x on success", uint16(tag), uint16(cmdTag))
		}
	}
	return tag, respSize, rc, body, nil
}

var tpmCommandNames = map[tpm2.TPMCC]string{
	tpm2.TPMCCStartup:                    "TPM2_Startup",
	tpm2.TPMCCShutdown:                   "TPM2_Shutdown",
	tpm2.TPMCCSelfTest:                   "TPM2_SelfTest",
	tpm2.TPMCCIncrementalSelfTest:        "TPM2_IncrementalSelfTest",
	tpm2.TPMCCGetTestResult:              "TPM2_GetTestResult",
	tpm2.TPMCCStartAuthSession:           "TPM2_StartAuthSession",
	tpm2.TPMCCPolicyRestart:              "TPM2_PolicyRestart",
	tpm2.TPMCCCreate:                     "TPM2_Create",
	tpm2.TPMCCLoad:                       "TPM2_Load",
	tpm2.TPMCCLoadExternal:               "TPM2_LoadExternal",
	tpm2.TPMCCReadPublic:                 "TPM2_ReadPublic",
	tpm2.TPMCCActivateCredential:         "TPM2_ActivateCredential",
	tpm2.TPMCCMakeCredential:             "TPM2_MakeCredential",
	tpm2.TPMCCUnseal:                     "TPM2_Unseal",
	tpm2.TPMCCObjectChangeAuth:           "TPM2_ObjectChangeAuth",
	tpm2.TPMCCCreateLoaded:               "TPM2_CreateLoaded",
	tpm2.TPMCCDuplicate:                  "TPM2_Duplicate",
	tpm2.TPMCCRewrap:                     "TPM2_Rewrap",
	tpm2.TPMCCImport:                     "TPM2_Import",
	tpm2.TPMCCRSAEncrypt:                 "TPM2_RSA_Encrypt",
	tpm2.TPMCCRSADecrypt:                 "TPM2_RSA_Decrypt",
	tpm2.TPMCCECDHKeyGen:                 "TPM2_ECDH_KeyGen",
	tpm2.TPMCCECDHZGen:                   "TPM2_ECDH_ZGen",
	tpm2.TPMCCECCParameters:              "TPM2_ECC_Parameters",
	tpm2.TPMCCZGen2Phase:                 "TPM2_ZGen_2Phase",
	tpm2.TPMCCEncryptDecrypt:             "TPM2_EncryptDecrypt",
	tpm2.TPMCCEncryptDecrypt2:            "TPM2_EncryptDecrypt2",
	tpm2.TPMCCHash:                       "TPM2_Hash",
	tpm2.TPMCCHMAC:                       "TPM2_HMAC",
	tpm2.TPMCCGetRandom:                  "TPM2_GetRandom",
	tpm2.TPMCCStirRandom:                 "TPM2_StirRandom",
	tpm2.TPMCCHMACStart:                  "TPM2_HMAC_Start",
	tpm2.TPMCCHashSequenceStart:          "TPM2_HashSequenceStart",
	tpm2.TPMCCSequenceUpdate:             "TPM2_SequenceUpdate",
	tpm2.TPMCCSequenceComplete:           "TPM2_SequenceComplete",
	tpm2.TPMCCEventSequenceComplete:      "TPM2_EventSequenceComplete",
	tpm2.TPMCCCertify:                    "TPM2_Certify",
	tpm2.TPMCCCertifyCreation:            "TPM2_CertifyCreation",
	tpm2.TPMCCQuote:                      "TPM2_Quote",
	tpm2.TPMCCGetSessionAuditDigest:      "TPM2_GetSessionAuditDigest",
	tpm2.TPMCCGetCommandAuditDigest:      "TPM2_GetCommandAuditDigest",
	tpm2.TPMCCGetTime:                    "TPM2_GetTime",
	tpm2.TPMCCCommit:                     "TPM2_Commit",
	tpm2.TPMCCECEphemeral:                "TPM2_EC_Ephemeral",
	tpm2.TPMCCVerifySignature:            "TPM2_VerifySignature",
	tpm2.TPMCCSign:                       "TPM2_Sign",
	tpm2.TPMCCSetCommandCodeAuditStatus:  "TPM2_SetCommandCodeAuditStatus",
	tpm2.TPMCCPCRExtend:                  "TPM2_PCR_Extend",
	tpm2.TPMCCPCREvent:                   "TPM2_PCR_Event",
	tpm2.TPMCCPCRRead:                    "TPM2_PCR_Read",
	tpm2.TPMCCPCRAllocate:                "TPM2_PCR_Allocate",
	tpm2.TPMCCPCRSetAuthPolicy:           "TPM2_PCR_SetAuthPolicy",
	tpm2.TPMCCPCRSetAuthValue:            "TPM2_PCR_SetAuthValue",
	tpm2.TPMCCPCRReset:                   "TPM2_PCR_Reset",
	tpm2.TPMCCPolicySigned:               "TPM2_PolicySigned",
	tpm2.TPMCCPolicySecret:               "TPM2_PolicySecret",
	tpm2.TPMCCPolicyTicket:               "TPM2_PolicyTicket",
	tpm2.TPMCCPolicyOR:                   "TPM2_PolicyOR",
	tpm2.TPMCCPolicyPCR:                  "TPM2_PolicyPCR",
	tpm2.TPMCCPolicyLocality:             "TPM2_PolicyLocality",
	tpm2.TPMCCPolicyNV:                   "TPM2_PolicyNV",
	tpm2.TPMCCPolicyCounterTimer:         "TPM2_PolicyCounterTimer",
	tpm2.TPMCCPolicyCommandCode:          "TPM2_PolicyCommandCode",
	tpm2.TPMCCPolicyPhysicalPresence:     "TPM2_PolicyPhysicalPresence",
	tpm2.TPMCCPolicyCpHash:               "TPM2_PolicyCpHash",
	tpm2.TPMCCPolicyNameHash:             "TPM2_PolicyNameHash",
	tpm2.TPMCCPolicyDuplicationSelect:    "TPM2_PolicyDuplicationSelect",
	tpm2.TPMCCPolicyAuthorize:            "TPM2_PolicyAuthorize",
	tpm2.TPMCCPolicyAuthValue:            "TPM2_PolicyAuthValue",
	tpm2.TPMCCPolicyPassword:             "TPM2_PolicyPassword",
	tpm2.TPMCCPolicyGetDigest:            "TPM2_PolicyGetDigest",
	tpm2.TPMCCPolicyNvWritten:            "TPM2_PolicyNvWritten",
	tpm2.TPMCCPolicyTemplate:             "TPM2_PolicyTemplate",
	tpm2.TPMCCPolicyAuthorizeNV:          "TPM2_PolicyAuthorizeNV",
	tpm2.TPMCCCreatePrimary:              "TPM2_CreatePrimary",
	tpm2.TPMCCHierarchyControl:           "TPM2_HierarchyControl",
	tpm2.TPMCCSetPrimaryPolicy:           "TPM2_SetPrimaryPolicy",
	tpm2.TPMCCChangePPS:                  "TPM2_ChangePPS",
	tpm2.TPMCCChangeEPS:                  "TPM2_ChangeEPS",
	tpm2.TPMCCClear:                      "TPM2_Clear",
	tpm2.TPMCCClearControl:               "TPM2_ClearControl",
	tpm2.TPMCCHierarchyChanegAuth:        "TPM2_HierarchyChangeAuth",
	tpm2.TPMCCDictionaryAttackLockReset:  "TPM2_DictionaryAttackLockReset",
	tpm2.TPMCCDictionaryAttackParameters: "TPM2_DictionaryAttackParameters",
	tpm2.TPMCCPPCommands:                 "TPM2_PP_Commands",
	tpm2.TPMCCSetAlgorithmSet:            "TPM2_SetAlgorithmSet",
	tpm2.TPMCCContextSave:                "TPM2_ContextSave",
	tpm2.TPMCCContextLoad:                "TPM2_ContextLoad",
	tpm2.TPMCCFlushContext:               "TPM2_FlushContext",
	tpm2.TPMCCEvictControl:               "TPM2_EvictControl",
	tpm2.TPMCCReadClock:                  "TPM2_ReadClock",
	tpm2.TPMCCClockSet:                   "TPM2_ClockSet",
	tpm2.TPMCCClockRateAdjust:            "TPM2_ClockRateAdjust",
	tpm2.TPMCCGetCapability:              "TPM2_GetCapability",
	tpm2.TPMCCTestParms:                  "TPM2_TestParms",
	tpm2.TPMCCNVDefineSpace:              "TPM2_NV_DefineSpace",
	tpm2.TPMCCNVUndefineSpace:            "TPM2_NV_UndefineSpace",
	tpm2.TPMCCNVUndefineSpaceSpecial:     "TPM2_NV_UndefineSpaceSpecial",
	tpm2.TPMCCNVReadPublic:               "TPM2_NV_ReadPublic",
	tpm2.TPMCCNVWrite:                    "TPM2_NV_Write",
	tpm2.TPMCCNVIncrement:                "TPM2_NV_Increment",
	tpm2.TPMCCNVExtend:                   "TPM2_NV_Extend",
	tpm2.TPMCCNVSetBits:                  "TPM2_NV_SetBits",
	tpm2.TPMCCNVWriteLock:                "TPM2_NV_WriteLock",
	tpm2.TPMCCNVGlobalWriteLock:          "TPM2_NV_GlobalWriteLock",
	tpm2.TPMCCNVRead:                     "TPM2_NV_Read",
	tpm2.TPMCCNVReadLock:                 "TPM2_NV_ReadLock",
	tpm2.TPMCCNVChangeAuth:               "TPM2_NV_ChangeAuth",
	tpm2.TPMCCNVCertify:                  "TPM2_NV_Certify",
	tpm2.TPMCCFirmwareRead:               "TPM2_FirmwareRead",
}

func tpmCommandName(cc tpm2.TPMCC) string {
	if name, ok := tpmCommandNames[cc]; ok {
		return name
	}
	return fmt.Sprintf("TPM_CC(0x%x)", uint32(cc))
}

func tpmCommandNameFromRaw(raw []byte) string {
	if len(raw) < 10 {
		return "TPM2_RawCommand"
	}
	cc := tpm2.TPMCC(binary.BigEndian.Uint32(raw[6:10]))
	return tpmCommandName(cc)
}

// SendRawExpectRC transmits a raw command byte slice and verifies that the
// returned TPM response code matches one of expected.
func (t *TpmHandle) SendRawExpectRC(raw []byte, expected ...tpm2.TPMRC) error {
	cmdName := tpmCommandNameFromRaw(raw)
	_, _, rc, _, err := t.SendRaw(raw)
	if err != nil {
		return errors.Wrap(err, cmdName)
	}
	if err := ExpectError(RCErr(rc), expected...); err != nil {
		return errors.Wrap(err, cmdName)
	}
	return nil
}

// ExecSessionCommand sends a TPM_ST_SESSIONS command for cc whose first handle
// is auth (authorized by auth.Auth) followed by the extra handles, and returns
// the response parameters.
func (t *TpmHandle) ExecSessionCommand(cc tpm2.TPMCC, auth tpm2.AuthHandle, params []byte, extra ...tpm2.NamedHandle) ([]byte, error) {
	return t.ExecMultiSessionCommand(cc, []tpm2.AuthHandle{auth}, params, extra...)
}

// ExecMultiSessionCommand sends a TPM_ST_SESSIONS command for cc whose leading
// handles are auths (each authorized by its Auth session) followed by extra
// non-authorized handles, validates all session response HMACs, and returns
// the response parameter bytes.
func (t *TpmHandle) ExecMultiSessionCommand(cc tpm2.TPMCC, auths []tpm2.AuthHandle, params []byte, extra ...tpm2.NamedHandle) (_ []byte, retErr error) {
	if len(auths) == 0 {
		return nil, errors.Errorf("%s: ExecMultiSessionCommand requires at least one AuthHandle", tpmCommandName(cc))
	}
	handles := make([]tpm2.TPMHandle, 0, len(auths)+len(extra))
	names := make([]tpm2.TPM2BName, 0, len(auths)+len(extra))
	sessions := make([]tpm2.Session, len(auths))
	for i, a := range auths {
		handles = append(handles, a.Handle)
		name := a.Name
		if len(name.Buffer) == 0 {
			name = namedOrRawHandle(a.Handle).Name
		}
		names = append(names, name)
		sess := a.Auth
		if sess == nil {
			sess = tpm2.PasswordAuth(nil)
		}
		if sess.IsEncryption() || sess.IsDecryption() {
			return nil, errors.Errorf("%s: ExecSessionCommand does not support parameter-encryption sessions", tpmCommandName(cc))
		}
		sessions[i] = sess
	}
	for _, h := range extra {
		handles = append(handles, h.Handle)
		name := h.Name
		if len(name.Buffer) == 0 {
			name = namedOrRawHandle(h.Handle).Name
		}
		names = append(names, name)
	}

	var inited []tpm2.Session
	defer func() {
		if retErr != nil {
			for _, s := range inited {
				_ = s.CleanupFailure(t)
			}
		}
	}()

	authCmds := make([][]byte, len(sessions))
	for i, sess := range sessions {
		if err := sess.Init(t); err != nil {
			return nil, err
		}
		inited = append(inited, sess)
		if err := sess.NewNonceCaller(); err != nil {
			return nil, err
		}
		ac, err := sess.Authorize(cc, params, nil, names, i)
		if err != nil {
			return nil, err
		}
		authCmds[i] = tpm2.Marshal(*ac)
	}

	raw := BuildRawCommand(tpm2.TPMSTSessions, cc, handles, WrapAuthArea(authCmds...), params)
	tag, _, rc, body, err := t.SendRaw(raw)
	if err != nil {
		return nil, err
	}
	if rc != tpm2.TPMRCSuccess {
		return nil, rc
	}
	cmdName := tpmCommandName(cc)
	if tag != tpm2.TPMSTSessions || len(body) < 4 {
		return nil, errors.Errorf("malformed %s response: tag 0x%x, %d body bytes", cmdName, uint16(tag), len(body))
	}
	paramSize := binary.BigEndian.Uint32(body[0:4])
	if uint64(paramSize)+4 > uint64(len(body)) {
		return nil, errors.Errorf("malformed %s response: parameterSize %d exceeds body (%d bytes)", cmdName, paramSize, len(body))
	}
	rspParams := body[4 : 4+paramSize]
	authRest := body[4+paramSize:]
	for i, sess := range sessions {
		arBytes, rem, err := splitFirstAuthResponse(authRest)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed %s response auth area [%d]", cmdName, i)
		}
		authRest = rem
		ar, err := tpm2.Unmarshal[tpm2.TPMSAuthResponse](arBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "malformed %s response auth area [%d]", cmdName, i)
		}
		if err := sess.Validate(rc, cc, rspParams, names, i, ar); err != nil {
			return nil, err
		}
	}
	if len(authRest) != 0 {
		return nil, errors.Errorf("malformed %s response: %d trailing bytes after auth area", cmdName, len(authRest))
	}
	return rspParams, nil
}

func splitFirstAuthResponse(b []byte) (authResp, rest []byte, err error) {
	nonce, rem, err := SplitFirstTPM2B(b)
	if err != nil {
		return nil, nil, errors.Wrap(err, "nonceTPM")
	}
	if len(rem) < 1 {
		return nil, nil, errors.New("missing sessionAttributes byte")
	}
	hmacBuf, rem2, err := SplitFirstTPM2B(rem[1:])
	if err != nil {
		return nil, nil, errors.Wrap(err, "hmac")
	}
	consumed := 2 + len(nonce) + 1 + 2 + len(hmacBuf)
	return b[:consumed], rem2, nil
}

// ClearControl enables or disables TPM2_Clear execution using auth
// (TPM_RH_LOCKOUT or TPM_RH_PLATFORM).
func (t *TpmHandle) ClearControl(auth tpm2.AuthHandle, disable bool) error {
	var disableByte byte
	if disable {
		disableByte = 1
	}
	if _, err := t.ExecSessionCommand(tpm2.TPMCCClearControl, auth, []byte{disableByte}); err != nil {
		return errors.Wrap(err, "TPM2_ClearControl")
	}
	return nil
}

// SelfTest executes TPM2_SelfTest with the given fullTest flag and returns
// the response code (TPMRCSuccess or TPMRCTesting) or a wrapped error.
func (t *TpmHandle) SelfTest(fullTest bool) (tpm2.TPMRC, error) {
	var flag byte
	if fullTest {
		flag = 1
	}
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCSelfTest, nil, nil, []byte{flag})
	_, _, rc, _, err := t.SendRaw(raw)
	if err != nil {
		return 0, errors.Wrap(err, "TPM2_SelfTest")
	}
	if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess, tpm2.TPMRCTesting); err != nil {
		return rc, errors.Wrap(err, "TPM2_SelfTest")
	}
	return rc, nil
}

// RunSelfTest executes TPM2_SelfTest(fullTest) and polls until testing completes
// with TPM_RC_SUCCESS.
func (t *TpmHandle) RunSelfTest(fullTest bool) error {
	rc, err := t.SelfTest(fullTest)
	if err != nil {
		return err
	}
	for iter := 0; rc == tpm2.TPMRCTesting && iter < 100; iter++ {
		if t.Config != nil && len(t.Config.HashAlgs) > 0 {
			hashCmd := tpm2.Hash{
				Data:      tpm2.TPM2BMaxBuffer{Buffer: []byte{1, 2, 3}},
				HashAlg:   t.Config.DefaultHashAlg(),
				Hierarchy: tpm2.TPMRHNull,
			}
			_, hashErr := hashCmd.Execute(t)
			if hashErr != nil && !IsRC(hashErr, tpm2.TPMRCTesting) {
				return errors.Wrap(hashErr, "TPM2_Hash during SelfTest")
			}
		}
		if t.Ctx != nil {
			select {
			case <-t.Ctx.Done():
				return errors.Wrap(t.Ctx.Err(), "TPM2_SelfTest context expired")
			case <-time.After(10 * time.Millisecond):
			}
		} else {
			<-time.After(10 * time.Millisecond)
		}
		rc, err = t.SelfTest(false)
		if err != nil {
			return err
		}
	}
	if rc != tpm2.TPMRCSuccess {
		return errors.Errorf("TPM2_SelfTest(full=%v) did not complete: last RC=%v", fullTest, rc)
	}
	return nil
}

// IncrementalSelfTest executes TPM2_IncrementalSelfTest for the given algorithm
// list and returns the response code, remaining toDoList, or a wrapped error.
func (t *TpmHandle) IncrementalSelfTest(algs []tpm2.TPMAlgID) (tpm2.TPMRC, []tpm2.TPMAlgID, error) {
	params := tpm2.Marshal(tpm2.TPMLAlg{Algorithms: algs})
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCIncrementalSelfTest, nil, nil, params)
	_, _, rc, body, err := t.SendRaw(raw)
	if err != nil {
		return 0, nil, errors.Wrap(err, "TPM2_IncrementalSelfTest")
	}
	if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess, tpm2.TPMRCTesting); err != nil {
		return rc, nil, errors.Wrap(err, "TPM2_IncrementalSelfTest")
	}
	if rc != tpm2.TPMRCSuccess {
		return rc, nil, nil
	}
	toDo, err := tpm2.Unmarshal[tpm2.TPMLAlg](body)
	if err != nil {
		return rc, nil, errors.Wrap(err, "TPM2_IncrementalSelfTest: failed to unmarshal toDoList")
	}
	return rc, toDo.Algorithms, nil
}

// GetTestResult executes TPM2_GetTestResult and unpacks outData and testResult.
func (t *TpmHandle) GetTestResult() ([]byte, tpm2.TPMRC, error) {
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCGetTestResult, nil, nil, nil)
	_, _, rc, body, err := t.SendRaw(raw)
	if err != nil {
		return nil, 0, errors.Wrap(err, "TPM2_GetTestResult")
	}
	if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess); err != nil {
		return nil, rc, errors.Wrap(err, "TPM2_GetTestResult")
	}
	outData, rest, err := SplitFirstTPM2B(body)
	if err != nil || len(rest) < 4 {
		return nil, 0, errors.Errorf("TPM2_GetTestResult: invalid response body (%d bytes)", len(body))
	}
	testResult := tpm2.TPMRC(binary.BigEndian.Uint32(rest[0:4]))
	return append([]byte(nil), outData...), testResult, nil
}

// StirRandom executes TPM2_StirRandom with the given entropy buffer.
func (t *TpmHandle) StirRandom(inData []byte) error {
	params := tpm2.Marshal(tpm2.TPM2BSensitiveData{Buffer: inData})
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCStirRandom, nil, nil, params)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// DictionaryAttackParameters executes TPM2_DictionaryAttackParameters using
// auth (defaulting to TPM_RH_LOCKOUT if auth.Handle == 0).
func (t *TpmHandle) DictionaryAttackParameters(auth tpm2.AuthHandle, maxTries, recoveryTime, lockoutRecovery uint32) error {
	if auth.Handle == 0 {
		auth.Handle = tpm2.TPMRHLockout
	}
	params := make([]byte, 12)
	binary.BigEndian.PutUint32(params[0:4], maxTries)
	binary.BigEndian.PutUint32(params[4:8], recoveryTime)
	binary.BigEndian.PutUint32(params[8:12], lockoutRecovery)
	if _, err := t.ExecSessionCommand(tpm2.TPMCCDictionaryAttackParameters, auth, params); err != nil {
		return errors.Wrap(err, "TPM2_DictionaryAttackParameters")
	}
	return nil
}

// ClockSet executes TPM2_ClockSet using auth (TPM_RH_OWNER or TPM_RH_PLATFORM).
func (t *TpmHandle) ClockSet(auth tpm2.AuthHandle, newTime uint64) error {
	params := make([]byte, 8)
	binary.BigEndian.PutUint64(params, newTime)
	if _, err := t.ExecSessionCommand(tpm2.TPMCCClockSet, auth, params); err != nil {
		return errors.Wrap(err, "TPM2_ClockSet")
	}
	return nil
}

// SetPrimaryPolicy executes TPM2_SetPrimaryPolicy using auth, policy digest,
// and hashing algorithm.
func (t *TpmHandle) SetPrimaryPolicy(auth tpm2.AuthHandle, authPolicy []byte, hashAlg tpm2.TPMAlgID) error {
	var params bytes.Buffer
	params.Write(tpm2.Marshal(tpm2.TPM2BDigest{Buffer: authPolicy}))
	_ = binary.Write(&params, binary.BigEndian, hashAlg)
	if _, err := t.ExecSessionCommand(tpm2.TPMCCSetPrimaryPolicy, auth, params.Bytes()); err != nil {
		return errors.Wrap(err, "TPM2_SetPrimaryPolicy")
	}
	return nil
}

// HierarchyControl executes TPM2_HierarchyControl using auth to enable or
// disable the specified hierarchy.
func (t *TpmHandle) HierarchyControl(auth tpm2.AuthHandle, enable tpm2.TPMHandle, state bool) error {
	var stateByte byte
	if state {
		stateByte = 1
	}
	params := make([]byte, 5)
	binary.BigEndian.PutUint32(params[0:4], uint32(enable))
	params[4] = stateByte
	if _, err := t.ExecSessionCommand(tpm2.TPMCCHierarchyControl, auth, params); err != nil {
		return errors.Wrap(err, "TPM2_HierarchyControl")
	}
	return nil
}

// ChangePPS executes TPM2_ChangePPS using auth (defaulting to TPM_RH_PLATFORM
// if auth.Handle == 0).
func (t *TpmHandle) ChangePPS(auth tpm2.AuthHandle) error {
	if auth.Handle == 0 {
		auth.Handle = tpm2.TPMRHPlatform
	}
	if _, err := t.ExecSessionCommand(tpm2.TPMCCChangePPS, auth, nil); err != nil {
		return errors.Wrap(err, "TPM2_ChangePPS")
	}
	return nil
}

// ChangeEPS executes TPM2_ChangeEPS using auth (defaulting to TPM_RH_PLATFORM
// if auth.Handle == 0).
func (t *TpmHandle) ChangeEPS(auth tpm2.AuthHandle) error {
	if auth.Handle == 0 {
		auth.Handle = tpm2.TPMRHPlatform
	}
	if _, err := t.ExecSessionCommand(tpm2.TPMCCChangeEPS, auth, nil); err != nil {
		return errors.Wrap(err, "TPM2_ChangeEPS")
	}
	return nil
}

// EventSequenceComplete executes TPM2_EventSequenceComplete using pcrAuth and
// seqAuth and unpacks the returned digest list.
func (t *TpmHandle) EventSequenceComplete(pcrAuth, seqAuth tpm2.AuthHandle, data []byte) (*tpm2.TPMLDigestValues, error) {
	params := tpm2.Marshal(tpm2.TPM2BMaxBuffer{Buffer: data})
	paramBytes, err := t.ExecMultiSessionCommand(tpm2.TPMCCEventSequenceComplete, []tpm2.AuthHandle{pcrAuth, seqAuth}, params)
	if err != nil {
		return nil, errors.Wrap(err, "TPM2_EventSequenceComplete")
	}
	res, err := tpm2.Unmarshal[tpm2.TPMLDigestValues](paramBytes)
	if err != nil {
		return nil, errors.Wrap(err, "TPM2_EventSequenceComplete: failed to unmarshal TPMLDigestValues")
	}
	return res, nil
}

// EncryptDecrypt executes TPM2_EncryptDecrypt2 (when ver2 is true) or raw
// TPM2_EncryptDecrypt (when ver2 is false) and returns outData and ivOut.
func (t *TpmHandle) EncryptDecrypt(keyAuth tpm2.AuthHandle, data []byte, decrypt bool, mode tpm2.TPMAlgID, ivIn []byte, ver2 bool) (outData, ivOut []byte, err error) {
	if ver2 {
		encryptDecrypt2 := tpm2.EncryptDecrypt2{
			KeyHandle: keyAuth,
			Message:   tpm2.TPM2BMaxBuffer{Buffer: data},
			Decrypt:   decrypt,
			Mode:      mode,
			IV:        tpm2.TPM2BIV{Buffer: ivIn},
		}
		resp, err := encryptDecrypt2.Execute(t)
		if err != nil {
			return nil, nil, errors.Wrap(err, "TPM2_EncryptDecrypt2")
		}
		return resp.OutData.Buffer, resp.IV.Buffer, nil
	}

	// go-tpm has no TPM2_EncryptDecrypt (v1), so marshal its parameters by hand.
	var decByte byte
	if decrypt {
		decByte = 1
	}
	var params bytes.Buffer
	params.WriteByte(decByte)
	_ = binary.Write(&params, binary.BigEndian, mode)
	params.Write(tpm2.Marshal(tpm2.TPM2BIV{Buffer: ivIn}))
	params.Write(tpm2.Marshal(tpm2.TPM2BMaxBuffer{Buffer: data}))
	rspParams, err := t.ExecSessionCommand(tpm2.TPMCCEncryptDecrypt, keyAuth, params.Bytes())
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_EncryptDecrypt")
	}
	outSlice, rest, err := SplitFirstTPM2B(rspParams)
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_EncryptDecrypt: invalid outData")
	}
	ivSlice, _, err := SplitFirstTPM2B(rest)
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_EncryptDecrypt: invalid ivOut")
	}
	return append([]byte(nil), outSlice...), append([]byte(nil), ivSlice...), nil
}

// ECCParams holds the unpacked curve parameters returned by TPM2_ECC_Parameters.
type ECCParams struct {
	CurveID tpm2.TPMECCCurve
	KeySize uint16
	P       []byte
	A       []byte
	B       []byte
	GX      []byte
	GY      []byte
	N       []byte
	H       []byte
}

// ECCParameters executes TPM2_ECC_Parameters for curveID and unpacks the
// returned curve parameters.
func (t *TpmHandle) ECCParameters(curveID tpm2.TPMECCCurve) (*ECCParams, error) {
	var params [2]byte
	binary.BigEndian.PutUint16(params[:], uint16(curveID))
	cmd := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCECCParameters, nil, nil, params[:])
	_, _, rc, body, err := t.SendRaw(cmd)
	if err != nil {
		return nil, errors.Wrap(err, "TPM2_ECC_Parameters")
	}
	if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess); err != nil {
		return nil, errors.Wrap(err, "TPM2_ECC_Parameters")
	}
	offset := 0
	next16 := func() (uint16, error) {
		if offset+2 > len(body) {
			return 0, errors.New("TPM2_ECC_Parameters: truncated header")
		}
		v := binary.BigEndian.Uint16(body[offset : offset+2])
		offset += 2
		return v, nil
	}
	out := &ECCParams{}
	curve, err := next16()
	if err != nil {
		return nil, err
	}
	out.CurveID = tpm2.TPMECCCurve(curve)
	if out.KeySize, err = next16(); err != nil {
		return nil, err
	}
	// kdf (TPMT_KDF_SCHEME) and sign (TPMT_ECC_SCHEME): a scheme ID, followed by a
	// hash ID unless the scheme is TPM_ALG_NULL (and a count for ECDAA).
	for _, isSign := range []bool{false, true} {
		alg, err := next16()
		if err != nil {
			return nil, err
		}
		skip := 0
		if tpm2.TPMAlgID(alg) != tpm2.TPMAlgNull {
			skip = 2
			if isSign && tpm2.TPMAlgID(alg) == tpm2.TPMAlgECDAA {
				skip = 4
			}
		}
		if offset+skip > len(body) {
			return nil, errors.New("TPM2_ECC_Parameters: truncated header")
		}
		offset += skip
	}
	rest := body[offset:]
	for _, dst := range []*[]byte{&out.P, &out.A, &out.B, &out.GX, &out.GY, &out.N, &out.H} {
		var inner []byte
		var splitErr error
		inner, rest, splitErr = SplitFirstTPM2B(rest)
		if splitErr != nil {
			return nil, errors.Wrap(splitErr, "TPM2_ECC_Parameters")
		}
		*dst = append([]byte(nil), inner...)
	}
	return out, nil
}

// SplitFirstTPM2B splits a 2-byte big-endian length-prefixed TPM2B buffer from b,
// returning its payload slice and the remaining bytes of b.
func SplitFirstTPM2B(b []byte) (inner, rest []byte, err error) {
	if len(b) < 2 {
		return nil, nil, errors.New("buffer too short for TPM2B size")
	}
	sz := int(binary.BigEndian.Uint16(b[:2]))
	if len(b) < 2+sz {
		return nil, nil, errors.New("buffer too short for TPM2B payload")
	}
	return b[2 : 2+sz], b[2+sz:], nil
}

func parseECDHKeyGenBody(body []byte) (zPoint, pubPoint tpm2.TPMSECCPoint, err error) {
	z2B, rest, err := SplitFirstTPM2B(body)
	if err != nil {
		return zPoint, pubPoint, err
	}
	pub2B, _, err := SplitFirstTPM2B(rest)
	if err != nil {
		return zPoint, pubPoint, err
	}
	zPtr, err := tpm2.Unmarshal[tpm2.TPMSECCPoint](z2B)
	if err != nil {
		return zPoint, pubPoint, err
	}
	pubPtr, err := tpm2.Unmarshal[tpm2.TPMSECCPoint](pub2B)
	if err != nil {
		return zPoint, pubPoint, err
	}
	return *zPtr, *pubPtr, nil
}

// ECDHKeyGen executes TPM2_ECDH_KeyGen on key (without sessions unless key.Auth
// is a non-password session such as an audit session) and returns zPoint and
// pubPoint.
func (t *TpmHandle) ECDHKeyGen(key tpm2.AuthHandle) (zPoint, pubPoint tpm2.TPMSECCPoint, err error) {
	var body []byte
	if key.Auth == nil || key.Auth.Handle() == tpm2.TPMRSPW {
		cmd := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCECDHKeyGen, []tpm2.TPMHandle{key.Handle}, nil, nil)
		var rc tpm2.TPMRC
		_, _, rc, body, err = t.SendRaw(cmd)
		if err != nil {
			return zPoint, pubPoint, errors.Wrap(err, "TPM2_ECDH_KeyGen")
		}
		if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess); err != nil {
			return zPoint, pubPoint, errors.Wrap(err, "TPM2_ECDH_KeyGen")
		}
	} else {
		body, err = t.ExecSessionCommand(tpm2.TPMCCECDHKeyGen, key, nil)
		if err != nil {
			return zPoint, pubPoint, errors.Wrap(err, "TPM2_ECDH_KeyGen")
		}
	}
	return parseECDHKeyGenBody(body)
}

// ECEphemeral executes TPM2_EC_Ephemeral for curveID and returns the ephemeral
// point q and counter.
func (t *TpmHandle) ECEphemeral(curveID tpm2.TPMECCCurve) (q tpm2.TPMSECCPoint, counter uint16, err error) {
	var params [2]byte
	binary.BigEndian.PutUint16(params[:], uint16(curveID))
	cmd := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCECEphemeral, nil, nil, params[:])
	_, _, rc, body, err := t.SendRaw(cmd)
	if err != nil {
		return q, 0, errors.Wrap(err, "TPM2_EC_Ephemeral")
	}
	if err := ExpectError(RCErr(rc), tpm2.TPMRCSuccess); err != nil {
		return q, 0, errors.Wrap(err, "TPM2_EC_Ephemeral")
	}
	q2B, rest, err := SplitFirstTPM2B(body)
	if err != nil {
		return q, 0, errors.Wrap(err, "TPM2_EC_Ephemeral")
	}
	if len(rest) < 2 {
		return q, 0, errors.New("TPM2_EC_Ephemeral: response missing counter")
	}
	qPtr, err := tpm2.Unmarshal[tpm2.TPMSECCPoint](q2B)
	if err != nil {
		return q, 0, errors.Wrap(err, "TPM2_EC_Ephemeral")
	}
	counter = binary.BigEndian.Uint16(rest[:2])
	return *qPtr, counter, nil
}

// ZGen2Phase executes TPM2_ZGen_2Phase using keyA and returns outZ1 and outZ2.
func (t *TpmHandle) ZGen2Phase(keyA tpm2.AuthHandle, inQsB, inQeB tpm2.TPMSECCPoint, scheme tpm2.TPMAlgID, counter uint16) (outZ1, outZ2 tpm2.TPMSECCPoint, err error) {
	var paramBuf bytes.Buffer
	paramBuf.Write(tpm2.Marshal(tpm2.New2B(inQsB)))
	paramBuf.Write(tpm2.Marshal(tpm2.New2B(inQeB)))
	var tail [4]byte
	binary.BigEndian.PutUint16(tail[0:2], uint16(scheme))
	binary.BigEndian.PutUint16(tail[2:4], counter)
	paramBuf.Write(tail[:])

	params, err := t.ExecSessionCommand(tpm2.TPMCCZGen2Phase, keyA, paramBuf.Bytes())
	if err != nil {
		return outZ1, outZ2, errors.Wrap(err, "TPM2_ZGen_2Phase")
	}
	outZ1, outZ2, err = parseECDHKeyGenBody(params)
	if err != nil {
		return outZ1, outZ2, errors.Wrap(err, "TPM2_ZGen_2Phase")
	}
	return outZ1, outZ2, nil
}

// PolicyRestart executes TPM2_PolicyRestart on sessionHandle.
func (t *TpmHandle) PolicyRestart(sessionHandle tpm2.TPMHandle) error {
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyRestart, []tpm2.TPMHandle{sessionHandle}, nil, nil)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyPassword executes TPM2_PolicyPassword on sessionHandle.
func (t *TpmHandle) PolicyPassword(sessionHandle tpm2.TPMHandle) error {
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyPassword, []tpm2.TPMHandle{sessionHandle}, nil, nil)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyPasswordOrAuthValue executes TPM2_PolicyPassword on sessionHandle when
// the TPM supports TPM_CC_PolicyPassword, or falls back to TPM2_PolicyAuthValue
// when TPM_CC_PolicyPassword is not implemented. Pair with
// PolicyPasswordAuthOption when authorizing the policy session.
func (t *TpmHandle) PolicyPasswordOrAuthValue(sessionHandle tpm2.TPMHandle) error {
	cc := tpm2.TPMCCPolicyPassword
	if t.Config != nil && !t.Config.IsCommandSupported(tpm2.TPMCCPolicyPassword) {
		cc = tpm2.TPMCCPolicyAuthValue
	}
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, cc, []tpm2.TPMHandle{sessionHandle}, nil, nil)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyLocality executes TPM2_PolicyLocality on sessionHandle with the given
// locality bitmap.
func (t *TpmHandle) PolicyLocality(sessionHandle tpm2.TPMHandle, locality byte) error {
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyLocality, []tpm2.TPMHandle{sessionHandle}, nil, []byte{locality})
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyPhysicalPresence executes TPM2_PolicyPhysicalPresence on sessionHandle.
func (t *TpmHandle) PolicyPhysicalPresence(sessionHandle tpm2.TPMHandle) error {
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyPhysicalPresence, []tpm2.TPMHandle{sessionHandle}, nil, nil)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyCounterTimer executes TPM2_PolicyCounterTimer on sessionHandle.
func (t *TpmHandle) PolicyCounterTimer(sessionHandle tpm2.TPMHandle, operandB []byte, offset uint16, op tpm2.TPMEO) error {
	var params bytes.Buffer
	params.Write(tpm2.Marshal(tpm2.TPM2BOperand{Buffer: operandB}))
	_ = binary.Write(&params, binary.BigEndian, offset)
	_ = binary.Write(&params, binary.BigEndian, op)
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyCounterTimer, []tpm2.TPMHandle{sessionHandle}, nil, params.Bytes())
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyNameHash executes TPM2_PolicyNameHash on sessionHandle.
func (t *TpmHandle) PolicyNameHash(sessionHandle tpm2.TPMHandle, nameHash []byte) error {
	params := tpm2.Marshal(tpm2.TPM2BDigest{Buffer: nameHash})
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyNameHash, []tpm2.TPMHandle{sessionHandle}, nil, params)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyTemplate executes TPM2_PolicyTemplate on sessionHandle.
func (t *TpmHandle) PolicyTemplate(sessionHandle tpm2.TPMHandle, templateHash []byte) error {
	params := tpm2.Marshal(tpm2.TPM2BDigest{Buffer: templateHash})
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyTemplate, []tpm2.TPMHandle{sessionHandle}, nil, params)
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// PolicyTicket executes TPM2_PolicyTicket on sessionHandle.
func (t *TpmHandle) PolicyTicket(sessionHandle tpm2.TPMHandle, timeout tpm2.TPM2BTimeout, cpHashA tpm2.TPM2BDigest, policyRef tpm2.TPM2BNonce, authName tpm2.TPM2BName, ticket tpm2.TPMTTKAuth) error {
	var params bytes.Buffer
	params.Write(tpm2.Marshal(timeout))
	params.Write(tpm2.Marshal(cpHashA))
	params.Write(tpm2.Marshal(policyRef))
	params.Write(tpm2.Marshal(authName))
	params.Write(tpm2.Marshal(ticket))
	raw := BuildRawCommand(tpm2.TPMSTNoSessions, tpm2.TPMCCPolicyTicket, []tpm2.TPMHandle{sessionHandle}, nil, params.Bytes())
	return t.SendRawExpectRC(raw, tpm2.TPMRCSuccess)
}

// Rewrap executes TPM2_Rewrap using oldParent authorization and returns
// outDuplicate and outSymSeed.
func (t *TpmHandle) Rewrap(oldParent tpm2.AuthHandle, newParent tpm2.TPMHandle, inDuplicate, name, inSymSeed []byte) (outDuplicate, outSymSeed []byte, err error) {
	var paramBuf bytes.Buffer
	paramBuf.Write(tpm2.Marshal(tpm2.TPM2BPrivate{Buffer: inDuplicate}))
	paramBuf.Write(tpm2.Marshal(tpm2.TPM2BName{Buffer: name}))
	paramBuf.Write(tpm2.Marshal(tpm2.TPM2BEncryptedSecret{Buffer: inSymSeed}))

	params, err := t.ExecSessionCommand(tpm2.TPMCCRewrap, oldParent, paramBuf.Bytes(), namedOrRawHandle(newParent))
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_Rewrap")
	}
	dupSlice, rest, err := SplitFirstTPM2B(params)
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_Rewrap: invalid outDuplicate")
	}
	seedSlice, _, err := SplitFirstTPM2B(rest)
	if err != nil {
		return nil, nil, errors.Wrap(err, "TPM2_Rewrap: invalid outSymSeed")
	}
	return append([]byte(nil), dupSlice...), append([]byte(nil), seedSlice...), nil
}

// SetCommandCodeAuditStatus executes TPM2_SetCommandCodeAuditStatus using auth.
func (t *TpmHandle) SetCommandCodeAuditStatus(auth tpm2.AuthHandle, auditAlg tpm2.TPMAlgID, setList, clearList []tpm2.TPMCC) error {
	var params bytes.Buffer
	_ = binary.Write(&params, binary.BigEndian, uint16(auditAlg))
	_, _ = params.Write(tpm2.Marshal(tpm2.TPMLCC{CommandCodes: setList}))
	_, _ = params.Write(tpm2.Marshal(tpm2.TPMLCC{CommandCodes: clearList}))
	if _, err := t.ExecSessionCommand(tpm2.TPMCCSetCommandCodeAuditStatus, auth, params.Bytes()); err != nil {
		return errors.Wrap(err, "TPM2_SetCommandCodeAuditStatus")
	}
	return nil
}
