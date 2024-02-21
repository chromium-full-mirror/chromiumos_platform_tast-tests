// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
	"time"
)

// I2cBusName represents a I2C bus that Open Titan Tool can act as either host or device on
type I2cBusName string

// GpioStrap represents a certain preset gpio configuration
type GpioStrap string

// GpioName represents a gpio that Open Titan Tool can read or write to
type GpioName string

// GpioEdge represent either rising or falling
type GpioEdge string

// HoldReset represents whether to hold the GSC in reset
type HoldReset bool

const (
	// GpioEdgeRising represents a rising edge
	GpioEdgeRising GpioEdge = "Rising"
	// GpioEdgeFalling represents a falling edge
	GpioEdgeFalling GpioEdge = "Falling"

	// HoldInReset says to hold the GSC in reset
	HoldInReset HoldReset = true

	// DoNotHoldInReset says to *not* hold the GSC in reset,
	// and to ensure it is booted before finishing.
	DoNotHoldInReset HoldReset = false
)

// UartName identifies one of the three UARTs forwarded through CCD
type UartName string

const (
	// UartTimeoutDefault sets the default uart timeout. Timeout if the console doesn't send output for 1 second
	UartTimeoutDefault time.Duration = time.Second
	// UartConsole represents the GSC console
	UartConsole UartName = "console"
	// UartConsoleTimeout sets the GSC console timeout. Timeout if the console doesn't send output for 5 seconds
	UartConsoleTimeout time.Duration = time.Second * 5
	// UartAP represents the AP UART
	UartAP UartName = "AP"
	// UartEC represents the EC UART
	UartEC UartName = "EC"
	// UartFPMCU represents the FPMCU UART
	UartFPMCU UartName = "FPMCU"
)

// GscUsbSpiBridgeOperation defines possible control interactions with the SPI
// bridge.
type GscUsbSpiBridgeOperation int

const (
	// DisableSpiBridge disables any active bridge.
	DisableSpiBridge GscUsbSpiBridgeOperation = iota
	// EnableApSpiBridge sets the bridge destination to AP flash.
	EnableApSpiBridge
	// EnableEcSpiBridge sets the bridge destination to EC flash.
	EnableEcSpiBridge
)

// DevBoard is the generic interface for development boards.
type DevBoard interface {
	// OpenTitanToolCommand runs an arbitrary OpenTitan tool command (without up-/downloading any files).
	OpenTitanToolCommand(ctx context.Context, cmd string, args ...string) (output map[string]interface{}, err error)
	// PlainCommand executes a opentitantool subcommand that uses no file arguments.
	PlainCommand(ctx context.Context, cmd string, args ...string) (output []byte, err error)
	// GSCToolCommand executes gsctool.
	GSCToolCommand(ctx context.Context, image string, args ...string) (output []byte, err error)
	// Executes TCG tests.
	RunTcgTests(ctx context.Context, outdir, testSuite string) error
	// PhysicalUart allows reading/writing data to a physical UART of the GSC under test.
	PhysicalUart(name UartName) SerialChannel
	// CcdSerialInterface allows reading/writing data to a USB interface provided by the GSC under test, which implements the "serial" USB class.
	CcdSerialInterface(name UartName, readTimeout time.Duration) SerialChannel
	// CCDFlashromRead reads the SPI flash chip via CCD.
	CCDFlashromRead(ctx context.Context) (contents []byte, durationMs uint32, err error)
	// CCDFlashromWrite writes the SPI flash chip via CCD.
	CCDFlashromWrite(ctx context.Context, contents []byte) (durationMs uint32, err error)
	// CCDFlashromErase erases the SPI flash chip via CCD.
	CCDFlashromErase(ctx context.Context) (durationMs uint32, err error)
	/// GscUsbI2cInterfaceTransaction write then reads from the I2C USB interface.
	GscUsbI2cInterfaceTransaction(ctx context.Context, request []byte) (response []byte, err error)
	/// GscUsbSpiInterfaceTransaction write then reads from the SPI USB interface.
	GscUsbSpiInterfaceTransaction(ctx context.Context, request []byte) (response []byte, err error)
	/// GscUsbSpiBridge interacts with the GSC USB SPI bridge.
	GscUsbSpiBridge(ctx context.Context, operation GscUsbSpiBridgeOperation) error
}

// SerialChannel is a handle to communicate using a two-way byte stream.
type SerialChannel interface {
	// Open opens the port.
	Open(ctx context.Context) error
	// IsOpen returns true iff the port is open.
	IsOpen() bool
	// Close closes the port.
	Close(ctx context.Context) error
	// ReadSerialSubmatch reads from the port until regex is matched.
	ReadSerialSubmatch(ctx context.Context, re *regexp.Regexp) (output [][]byte, err error)
	// ReadSerialBytes reads from the serial port until number of bytes have been read.
	ReadSerialBytes(ctx context.Context, size int) (output []byte, err error)
	// WriteSerial writes to the port.
	WriteSerial(ctx context.Context, bytes []byte) error
	// ClearInput clears any pending input that hasn't been read yet.
	ClearInput(ctx context.Context) error
}

// ApFlash provides access to the SPI flash chip connected to the devboard.
// Constructed by `DevBoardHelper.WithApFlashAccess`.
type ApFlash interface {
	// FetchApFlashInfo fetches the name and vendor of the SPI flash chip connected to the devboard.
	FetchApFlashInfo(ctx context.Context) (output *ApFlashInfo, err error)

	// EnableApWriteProtect enables HW write protection on the SPI flash chip with the given
	// start address and length.
	EnableApWriteProtect(ctx context.Context, startAddr, len uint32) error

	// WriteApFlash writes `contents` to the SPI flash chip connected to the devboard.
	WriteApFlash(ctx context.Context, contents []byte) error
}

// ApFlashInfo contains info about the AP flash that the DUT would have access to.
type ApFlashInfo struct {
	Name   string
	Vendor string
}

// TestbedType represents a kind of testbed, including which GSC devboard, debugger and wiring.
type TestbedType string

const (
	// GscDTAndreiboard is a traditional AndreiBoard with dozens of wires to a
	// HyperDebug according to:
	// https://docs.google.com/spreadsheets/d/1youX_Yh2A6-Zd2T98ShjH_O8M9CZexDB9DCHpegNMvE
	GscDTAndreiboard TestbedType = "gsc_dt_ab"

	// GscDTShield is a small DT board on top of HyperDebug.
	GscDTShield TestbedType = "gsc_dt_shield"

	// GscOpentitanCw310Fpga is a ChipWhisperer 310 FPGA board connected via ribbon cables to
	// a "swizzle board" on top of HyperDebug.
	GscOpentitanCw310Fpga TestbedType = "gsc_ot_fpga_cw310"

	// GscHostEmulation is not a physical testbed, but an emulation on a Linux host computer.
	GscHostEmulation TestbedType = "gsc_he"

	// GscH1Shield is a small H1 board on top of HyperDebug.
	GscH1Shield TestbedType = "gsc_h1_shield"

	// GscOTShield is a small OpenTitan board on top of HyperDebug.
	GscOTShield TestbedType = "gsc_ot_shield"
)

// APROResultCode represents the status of AP RO verification.
type APROResultCode byte

const (
	// ApRoNotRun is a V1 code that means AP RO verification did not run.
	ApRoNotRun APROResultCode = 0
	// ApRoPassUnverifiedGbb is a V1 code that mean everything passed but GBB.
	ApRoPassUnverifiedGbb APROResultCode = 1
	// ApRoFail is a V1 code that means verification failed .
	ApRoFail APROResultCode = 2
	// ApRoUnsupportedUnknown is a V1 code that mean an unknown error occurred.
	ApRoUnsupportedUnknown APROResultCode = 3
	// ApRoUnsupportedNotTriggered is a V1 code that means verification was not
	// triggered.
	ApRoUnsupportedNotTriggered APROResultCode = 4
	// ApRoUnsupportedTriggered is a V1 code that means verification was not run.
	ApRoUnsupportedTriggered APROResultCode = 5
	// ApRoPass is a V1 code that means verification passed.
	ApRoPass APROResultCode = 6
	// ApRoInProgress is a V1 code that means verification is on going.
	ApRoInProgress APROResultCode = 7
	// ApRoV2Success is a V2 code meaning success.
	ApRoV2Success APROResultCode = 20
	// ApRoV2FailedVerification is a V2 code meaning failure.
	ApRoV2FailedVerification APROResultCode = 21
	// ApRoV2InconsistentGscvd is a V2 code meaning GSCVD section was bad.
	ApRoV2InconsistentGscvd APROResultCode = 22
	// ApRoV2InconsistentKeyblock is a V2 code meaning key block was bad.
	ApRoV2InconsistentKeyblock APROResultCode = 23
	// ApRoV2InconsistentKey is a V2 code meaning signing key was wrong.
	ApRoV2InconsistentKey APROResultCode = 24
	// ApRoV2SpiRead is a V2 code meaning SPI read failed.
	ApRoV2SpiRead APROResultCode = 25
	// ApRoV2UnsupportedCryptoAlgorithm is a V2 code meaning bad crypto algorithm.
	ApRoV2UnsupportedCryptoAlgorithm APROResultCode = 26
	// ApRoV2VersionMismatch is a V2 code meaning version did not match.
	ApRoV2VersionMismatch APROResultCode = 27
	// ApRoV2OutOfMemory is a V2 code meaning there wasn't enough memory.
	ApRoV2OutOfMemory APROResultCode = 28
	// ApRoV2Internal is a V2 code meaning there was an internal error.
	ApRoV2Internal APROResultCode = 29
	// ApRoV2TooBig is a V2 code meaning there was an internal size error.
	ApRoV2TooBig APROResultCode = 30
	// ApRoV2MissingGscvd is a V2 code meaning there was no GSCVD section.
	ApRoV2MissingGscvd APROResultCode = 31
	// ApRoV2BoardIDMismatch is a V2 code meaning the board id did not match.
	ApRoV2BoardIDMismatch APROResultCode = 32
	// ApRoV2SettingNotProvisioned is a V2 code meaning setting were not
	// provisioned.
	ApRoV2SettingNotProvisioned APROResultCode = 33
	// ApRoV2NonZeroGbbFlags is a V2 code meaning GBB flags are non-zero.
	ApRoV2NonZeroGbbFlags APROResultCode = 36
	// ApRoV2WrongRootKey is a V2 code meaning the wrong key was used.
	ApRoV2WrongRootKey APROResultCode = 37
	// ApRoV2Unknown is a V2 code meaning there was an unknown error.
	ApRoV2Unknown APROResultCode = 255
)

// IsV2Code returns true if this is a version 2 return code
func (c APROResultCode) IsV2Code() bool {
	return byte(c) >= byte(ApRoV2Success)
}
