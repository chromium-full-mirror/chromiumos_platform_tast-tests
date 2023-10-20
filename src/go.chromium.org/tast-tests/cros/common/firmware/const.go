// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

/*
This file defines constants used by both local and remote firmware tests.
*/

// BootMode is a string representing the DUT's firmware boot-mode.
// It is intended to be used with the constants defined below.
type BootMode string

// DUTs have four possible boot modes: Normal, Dev, USBDev, and Recovery.
const (
	BootModeNormal      BootMode = "normal"
	BootModeDev         BootMode = "dev"
	BootModeUSBDev      BootMode = "usbdev"
	BootModeRecovery    BootMode = "rec"
	BootModeUnspecified BootMode = "unspecified"
)

// RWSection refers to one of the two RW firmware sections installed on a Chromebook.
type RWSection string

// There are two RW sections of firmware: "A" and "B". Normally, A is used unless B is required, such as while updating A.
const (
	RWSectionA           RWSection = "A"
	RWSectionB           RWSection = "B"
	RWSectionUnspecified RWSection = ""
)

// FwScreenID denotes the id for an individual firmware screen.
type FwScreenID int

// We have seen the following firmware screens on devices adopting
// Menu UI (MUI). The ids are listed below, as found from the
// 'depthcharge/src/vboot/ui.h' file.
const (
	FirmwareSync                   FwScreenID = 0x100
	RecoveryBroken                 FwScreenID = 0x110
	AdvancedOptions                FwScreenID = 0x120
	LanguageSelect                 FwScreenID = 0x130
	DebugInfo                      FwScreenID = 0x140
	FirmwareLog                    FwScreenID = 0x150
	RecoverySelect                 FwScreenID = 0x200
	RecoveryInvalid                FwScreenID = 0x201
	RecoveryToDev                  FwScreenID = 0x202
	RecoveryDiskStep1              FwScreenID = 0x220
	RecoveryDiskStep2              FwScreenID = 0x221
	RecoveryDiskStep3              FwScreenID = 0x222
	DeveloperMode                  FwScreenID = 0x300
	DeveloperToNorm                FwScreenID = 0x310
	DeveloperBootExternal          FwScreenID = 0x320
	DeveloperInvalidDisk           FwScreenID = 0x330
	DeveloperSelectAltfw           FwScreenID = 0x340
	Diagnostics                    FwScreenID = 0x400
	DiagnosticsStorageHealth       FwScreenID = 0x410
	DiagnosticsStorageTestShort    FwScreenID = 0x411
	DiagnosticsStorageTestExtended FwScreenID = 0x412
	DiagnosticsMemoryQuick         FwScreenID = 0x420
	DiagnosticsMemoryFull          FwScreenID = 0x421
)

// The following firmware screens were found on devices adopting
// Legacy Menu UI (LMUI), and Legacy Clamshell UI (LCUI).
// They are listed below, as found from the
// 'depthcharge/src/drivers/video/display.h' file.
const (
	LegacyBlank                FwScreenID = 0x0
	LegacyDeveloperWarning     FwScreenID = 0x101
	LegacyRecoveryInsert       FwScreenID = 0x202
	LegacyRecoveryNoGood       FwScreenID = 0x203
	LegacyRecoveryToDev        FwScreenID = 0x204
	LegacyDeveloperToNorm      FwScreenID = 0x205
	LegacyScreenWait           FwScreenID = 0x206
	LegacyToNormConfirmed      FwScreenID = 0x207
	LegacyOSBroken             FwScreenID = 0x208
	LegacyDeveloperWarningMenu FwScreenID = 0x20a
	LegacyDeveloperMenu        FwScreenID = 0x20b
	LegacyRecoveryToDevMenu    FwScreenID = 0x20d
	LegacyDeveloperToNormMenu  FwScreenID = 0x20e
	LegacyLanguagesMenu        FwScreenID = 0x20f
	LegacyOptionsMenu          FwScreenID = 0x210
	LegacyAltFwPick            FwScreenID = 0x212
	LegacyAltFwMenu            FwScreenID = 0x213
	LegacyCompleteVendorData   FwScreenID = 0x300
	LegacySetVendorData        FwScreenID = 0x301
	LegacyConfirmVendorData    FwScreenID = 0x302
	LegacyConfirmDiag          FwScreenID = 0x303
)

// FwScreenType represents a firmware screen.
type FwScreenType int

// FwScreenType contains the following firmware screens.
const (
	FwDeveloperScreen FwScreenType = iota
	FwRecoveryScreen
	FwBrokenScreen
)
