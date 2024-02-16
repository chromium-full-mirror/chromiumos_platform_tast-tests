// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains recovery reasons defined in Vboot.

package reporters

import (
	"context"
	"strconv"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// RecoveryReasonString represents string for RecoveryReasonValue.
type RecoveryReasonString string

// RecoveryReason represents recovery_reason attributes.
type RecoveryReason int

// RecoveryReason params used by tests
const (
	RecoveryReasonNotRequested                RecoveryReason = 0x00
	RecoveryReasonLegacy                      RecoveryReason = 0x01
	RecoveryReasonROManual                    RecoveryReason = 0x02
	RecoveryReasonROInvalidRW                 RecoveryReason = 0x03
	RecoveryReasonDeprecatedROS3Resume        RecoveryReason = 0x04
	RecoveryReasonDeprecatedROTPMError        RecoveryReason = 0x05
	RecoveryReasonROSharedData                RecoveryReason = 0x06
	RecoveryReasonDeprecatedROTestS3          RecoveryReason = 0x07
	RecoveryReasonDeprecatedROTestLFS         RecoveryReason = 0x08
	RecoveryReasonDeprecatedROTestLF          RecoveryReason = 0x09
	RecoveryReasonDeprecatedRWNotDone         RecoveryReason = 0x10
	RecoveryReasonDeprecatedRWDevFlagMismatch RecoveryReason = 0x11
	RecoveryReasonDeprecatedRWRecFlagMismatch RecoveryReason = 0x12
	RecoveryReasonFWKeyblock                  RecoveryReason = 0x13
	RecoveryReasonFWKeyRollback               RecoveryReason = 0x14
	RecoveryReasonDeprecatedRWDataKeyParse    RecoveryReason = 0x15
	RecoveryReasonFWPreamble                  RecoveryReason = 0x16
	RecoveryReasonFWRollback                  RecoveryReason = 0x17
	RecoveryReasonDeprecatedFWHeaderValid     RecoveryReason = 0x18
	RecoveryReasonFWGetFWBody                 RecoveryReason = 0x19
	RecoveryReasonDeprecatedFWHashWrongSize   RecoveryReason = 0x1a
	RecoveryReasonFWBody                      RecoveryReason = 0x1b
	RecoveryReasonDeprecatedFWValid           RecoveryReason = 0x1c
	RecoveryReasonDeprecatedFWNoRONormal      RecoveryReason = 0x1d
	RecoveryReasonFWVendorBlob                RecoveryReason = 0x1e
	RecoveryReasonROFirmware                  RecoveryReason = 0x20
	RecoveryReasonROTPMReboot                 RecoveryReason = 0x21
	RecoveryReasonECSoftwareSync              RecoveryReason = 0x22
	RecoveryReasonECUnknownImage              RecoveryReason = 0x23
	RecoveryReasonDeprecatedECHash            RecoveryReason = 0x24
	RecoveryReasonDeprecatedECExpectedImage   RecoveryReason = 0x25
	RecoveryReasonECUpdate                    RecoveryReason = 0x26
	RecoveryReasonECJumpRW                    RecoveryReason = 0x27
	RecoveryReasonECProtect                   RecoveryReason = 0x28
	RecoveryReasonECExpectedHash              RecoveryReason = 0x29
	RecoveryReasonDeprecatedECHashMismatch    RecoveryReason = 0x2a
	RecoveryReasonSecdataFirmwareInit         RecoveryReason = 0x2b
	RecoveryReasonGBBHeader                   RecoveryReason = 0x2c
	RecoveryReasonTPMClearOwner               RecoveryReason = 0x2d
	RecoveryReasonDevSwitch                   RecoveryReason = 0x2e
	RecoveryReasonFWSlot                      RecoveryReason = 0x2f
	RecoveryReasonAUXFWUpdate                 RecoveryReason = 0x30
	RecoveryReasonIntelCSELiteSKU             RecoveryReason = 0x31
	RecoveryReasonROUnspecified               RecoveryReason = 0x3f
	RecoveryReasonDeprecatedRWDevScreen       RecoveryReason = 0x41
	RecoveryReasonDeprecatedRWNoOS            RecoveryReason = 0x42
	RecoveryReasonRWInvalidOS                 RecoveryReason = 0x43
	RecoveryReasonDeprecatedRWTPMError        RecoveryReason = 0x44
	RecoveryReasonDeprecatedRWDevMismatch     RecoveryReason = 0x45
	RecoveryReasonRWSharedData                RecoveryReason = 0x46
	RecoveryReasonDeprecatedRWTestLK          RecoveryReason = 0x47
	RecoveryReasonDeprecatedRWNoDisk          RecoveryReason = 0x48
	RecoveryReasonTPMEFail                    RecoveryReason = 0x49
	RecoveryReasonROTPMSError                 RecoveryReason = 0x50
	RecoveryReasonROTPMWError                 RecoveryReason = 0x51
	RecoveryReasonROTPMLError                 RecoveryReason = 0x52
	RecoveryReasonROTPMUError                 RecoveryReason = 0x53
	RecoveryReasonRWTPMRError                 RecoveryReason = 0x54
	RecoveryReasonRWTPMWError                 RecoveryReason = 0x55
	RecoveryReasonRWTPMLError                 RecoveryReason = 0x56
	RecoveryReasonECHashFailed                RecoveryReason = 0x57
	RecoveryReasonECHashSize                  RecoveryReason = 0x58
	RecoveryReasonLKUnspecified               RecoveryReason = 0x59
	RecoveryReasonRWNoDisk                    RecoveryReason = 0x5a
	RecoveryReasonRWNoKernel                  RecoveryReason = 0x5b
	RecoveryReasonDeprecatedRWBCBError        RecoveryReason = 0x5c
	RecoveryReasonSecdataKernelInit           RecoveryReason = 0x5d
	RecoveryReasonDeprecatedFWFastboot        RecoveryReason = 0x5e
	RecoveryReasonROTPMRecHashLError          RecoveryReason = 0x5f
	RecoveryReasonTPMDisableFailed            RecoveryReason = 0x60
	RecoveryReasonALTFWHashMismatch           RecoveryReason = 0x61
	RecoveryReasonSecdataFWMPInit             RecoveryReason = 0x62
	RecoveryReasonGSCBootMode                 RecoveryReason = 0x63
	RecoveryReasonEscapeNoBoot                RecoveryReason = 0x64
	RecoveryReasonWidevinePrepare             RecoveryReason = 0x65
	RecoveryReasonRWUnspecified               RecoveryReason = 0x7f
	RecoveryReasonDeprecatedKEDMVerity        RecoveryReason = 0x81
	RecoveryReasonDeprecatedKEUnspecified     RecoveryReason = 0xbf
	RecoveryReasonUSTest                      RecoveryReason = 0xc1
	RecoveryReasonDeprecatedBCBUserMode       RecoveryReason = 0xc2
	RecoveryReasonDeprecatedUSFastboot        RecoveryReason = 0xc3
	RecoveryReasonTrainAndReboot              RecoveryReason = 0xc4
	RecoveryReasonUSUnspecified               RecoveryReason = 0xff
)

// The strings are copied from vboot_reference/firmware/2lib/2recovery_reasons.c.
var recoveryReasonStringMap = map[RecoveryReason]RecoveryReasonString{
	RecoveryReasonNotRequested:                "Recovery not requested",
	RecoveryReasonLegacy:                      "Recovery requested from legacy utility",
	RecoveryReasonROManual:                    "Recovery button pressed",
	RecoveryReasonROInvalidRW:                 "RW firmware failed signature check",
	RecoveryReasonDeprecatedROS3Resume:        "S3 resume failed",
	RecoveryReasonDeprecatedROTPMError:        "TPM error in read-only firmware",
	RecoveryReasonROSharedData:                "Shared data error in read-only firmware",
	RecoveryReasonDeprecatedROTestS3:          "Test error from S3Resume()",
	RecoveryReasonDeprecatedROTestLFS:         "Test error from LoadFirmwareSetup()",
	RecoveryReasonDeprecatedROTestLF:          "Test error from LoadFirmware()",
	RecoveryReasonDeprecatedRWNotDone:         "RW firmware check not done",
	RecoveryReasonDeprecatedRWDevFlagMismatch: "RW firmware developer flag mismatch",
	RecoveryReasonDeprecatedRWRecFlagMismatch: "RW firmware recovery flag mismatch",
	RecoveryReasonFWKeyblock:                  "RW firmware unable to verify keyblock",
	RecoveryReasonFWKeyRollback:               "RW firmware key version rollback detected",
	RecoveryReasonDeprecatedRWDataKeyParse:    "RW firmware unable to parse data key",
	RecoveryReasonFWPreamble:                  "RW firmware unable to verify preamble",
	RecoveryReasonFWRollback:                  "RW firmware version rollback detected",
	RecoveryReasonDeprecatedFWHeaderValid:     "RW firmware header is valid",
	RecoveryReasonFWGetFWBody:                 "RW firmware error when accessing firmware body",
	RecoveryReasonDeprecatedFWHashWrongSize:   "RW firmware hash is wrong size",
	RecoveryReasonFWBody:                      "RW firmware unable to verify firmware body",
	RecoveryReasonDeprecatedFWValid:           "RW firmware is valid",
	RecoveryReasonDeprecatedFWNoRONormal:      "RW firmware read-only normal path is not supported",
	RecoveryReasonFWVendorBlob:                "RW firmware vendor blob verification failure",
	RecoveryReasonROFirmware:                  "Firmware problem outside of verified boot",
	RecoveryReasonROTPMReboot:                 "TPM requires a system reboot (should be transient)",
	RecoveryReasonECSoftwareSync:              "EC software sync error",
	RecoveryReasonECUnknownImage:              "EC software sync unable to determine active EC image",
	RecoveryReasonDeprecatedECHash:            "EC software sync error obtaining EC image hash",
	RecoveryReasonDeprecatedECExpectedImage:   "EC software sync error obtaining expected EC image from BIOS",
	RecoveryReasonECUpdate:                    "EC software sync error updating EC",
	RecoveryReasonECJumpRW:                    "EC software sync unable to jump to EC-RW",
	RecoveryReasonECProtect:                   "EC software sync protection error",
	RecoveryReasonECExpectedHash:              "EC software sync error obtaining expected EC hash from BIOS",
	RecoveryReasonDeprecatedECHashMismatch:    "EC software sync error comparing expected EC hash and image",
	RecoveryReasonSecdataFirmwareInit:         "Firmware secure NVRAM (TPM) initialization error",
	RecoveryReasonGBBHeader:                   "Error parsing GBB header",
	RecoveryReasonTPMClearOwner:               "Error trying to clear TPM owner",
	RecoveryReasonDevSwitch:                   "Error reading or updating developer switch",
	RecoveryReasonFWSlot:                      "Error selecting RW firmware slot",
	RecoveryReasonAUXFWUpdate:                 "Error updating auxiliary firmware",
	RecoveryReasonIntelCSELiteSKU:             "Intel CSE Lite SKU firmware failure",
	RecoveryReasonROUnspecified:               "Unspecified/unknown error in RO firmware",
	RecoveryReasonDeprecatedRWDevScreen:       "User requested recovery from dev-mode warning screen",
	RecoveryReasonDeprecatedRWNoOS:            "No OS kernel detected (or kernel rollback attempt?)",
	RecoveryReasonRWInvalidOS:                 "OS kernel or rootfs failed signature check",
	RecoveryReasonDeprecatedRWTPMError:        "TPM error in rewritable firmware",
	RecoveryReasonDeprecatedRWDevMismatch:     "RW firmware in dev mode, but dev switch is off",
	RecoveryReasonRWSharedData:                "Shared data error in rewritable firmware",
	RecoveryReasonDeprecatedRWTestLK:          "Test error from vb2api_load_kernel()",
	RecoveryReasonDeprecatedRWNoDisk:          "No bootable storage device in system",
	RecoveryReasonTPMEFail:                    "TPM error that was not fixed by reboot",
	RecoveryReasonROTPMSError:                 "TPM setup error in read-only firmware",
	RecoveryReasonROTPMWError:                 "TPM write error in read-only firmware",
	RecoveryReasonROTPMLError:                 "TPM lock error in read-only firmware",
	RecoveryReasonROTPMUError:                 "TPM update error in read-only firmware",
	RecoveryReasonRWTPMRError:                 "TPM read error in rewritable firmware",
	RecoveryReasonRWTPMWError:                 "TPM write error in rewritable firmware",
	RecoveryReasonRWTPMLError:                 "TPM lock error in rewritable firmware",
	RecoveryReasonECHashFailed:                "EC software sync unable to get EC image hash",
	RecoveryReasonECHashSize:                  "EC software sync invalid image hash size",
	RecoveryReasonLKUnspecified:               "Unspecified error while trying to load kernel",
	RecoveryReasonRWNoDisk:                    "No bootable storage device in system",
	RecoveryReasonRWNoKernel:                  "No bootable kernel found on disk",
	RecoveryReasonDeprecatedRWBCBError:        "BCB partition error on disk",
	RecoveryReasonSecdataKernelInit:           "Kernel secure NVRAM (TPM) initialization error",
	RecoveryReasonDeprecatedFWFastboot:        "Fastboot-mode requested in firmware",
	RecoveryReasonROTPMRecHashLError:          "Recovery hash space lock error in RO firmware",
	RecoveryReasonTPMDisableFailed:            "Failed to disable TPM before running untrusted code",
	RecoveryReasonALTFWHashMismatch:           "Verification of alternate bootloader payload failed",
	RecoveryReasonSecdataFWMPInit:             "FWMP secure NVRAM (TPM) initialization error",
	RecoveryReasonGSCBootMode:                 "Failed to get boot mode from GSC",
	RecoveryReasonEscapeNoBoot:                "Attempt to escape from NO_BOOT mode was detected",
	RecoveryReasonWidevinePrepare:             "Failed to prepare widevine",
	RecoveryReasonRWUnspecified:               "Unspecified/unknown error in RW firmware",
	RecoveryReasonDeprecatedKEDMVerity:        "DM-verity error",
	RecoveryReasonDeprecatedKEUnspecified:     "Unspecified/unknown error in kernel",
	RecoveryReasonUSTest:                      "Recovery mode test from user-mode",
	RecoveryReasonDeprecatedBCBUserMode:       "User-mode requested recovery via BCB",
	RecoveryReasonDeprecatedUSFastboot:        "User-mode requested fastboot mode",
	RecoveryReasonTrainAndReboot:              "User-mode requested DRAM train and reboot",
	RecoveryReasonUSUnspecified:               "Unspecified/unknown error in user-mode",
}

// GetRecoveryReasonString returns the recovery reason string.
func (r *Reporter) GetRecoveryReasonString(ctx context.Context, recoveryReasonInt RecoveryReason) (RecoveryReasonString, error) {
	return recoveryReasonStringMap[RecoveryReason(recoveryReasonInt)], nil
}

// ContainsRecoveryReason determines if a recovery_reason crossystem value contains one of expectedReasons.
func (r *Reporter) ContainsRecoveryReason(ctx context.Context, expectedReasons []RecoveryReason) (bool, error) {
	csRecReasonVal, err := r.CrossystemParam(ctx, CrossystemParamRecoveryReason)
	if err != nil {
		return false, errors.Wrap(err, "failed to get recovery reason")
	}
	csRecReasonInt, err := strconv.ParseInt(csRecReasonVal, 10, 64)
	if err != nil {
		return false, errors.Wrap(err, "failed to convert the value to int")
	}
	testing.ContextLogf(ctx, "Current recovery reason is %s (%v)", recoveryReasonStringMap[RecoveryReason(csRecReasonInt)], csRecReasonInt)
	for _, expReason := range expectedReasons {
		if RecoveryReason(csRecReasonInt) == expReason {
			return true, nil
		}
	}
	return false, nil
}
