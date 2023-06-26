// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
)

// GpioStrap represents a certain preset gpio configuration
type GpioStrap string

// GpioName represents a gpio that Open Titan Tool can read or write to
type GpioName string

// GpioEdge represent either rising or falling
type GpioEdge string

const (
	// GpioEdgeRising represents a rising edge
	GpioEdgeRising GpioEdge = "Rising"
	// GpioEdgeFalling represents a falling edge
	GpioEdgeFalling GpioEdge = "Falling"
)

// DevBoard is the generic interface for development boards.
type DevBoard interface {
	// Open opens the ti50 console and EC consoles.
	Open(ctx context.Context) error
	// ReadSerialSubmatch reads gsc console output from port until regex is matched.
	ReadSerialSubmatch(ctx context.Context, re *regexp.Regexp) (output [][]byte, err error)
	// WriteSerial writes to gsc console.
	WriteSerial(ctx context.Context, bytes []byte) error
	// ClearInput clears any pending input that hasn't been read yet.
	ClearInput(ctx context.Context) error
	// OpenTitanToolCommand runs an arbitrary OpenTitan tool command (without up-/downloading any files).
	OpenTitanToolCommand(ctx context.Context, cmd string, args ...string) (output map[string]interface{}, err error)
	// PlainCommand executes a opentitantool subcommand that uses no file arguments.
	PlainCommand(ctx context.Context, cmd string, args ...string) (output []byte, err error)
	// Reset the DevBoard.
	Reset(ctx context.Context) error
	// Close closes all open consoles.
	Close(ctx context.Context) error
	// GSCToolCommand executes gsctool.
	GSCToolCommand(ctx context.Context, image string, args ...string) (output []byte, err error)
	// Executes TCG tests.
	RunTcgTests(ctx context.Context, outdir, testSuite string) error
	// ECSerialWrite writes the specified bytes to the EC console. This also clears any pending
	// incoming EC console data that hasn't been read yet as this is the most common pattern to
	// interact with EC console.
	ECSerialWrite(ctx context.Context, bytes []byte) error
	// ECSerialRead reads the specified number of bytes from the EC console.
	ECSerialRead(ctx context.Context, size int) ([]byte, error)
}

// TestbedType represents a kind of testbed, including which GSC devboard, debugger and wiring.
// Please update AllTestbedTypes() after editing.
type TestbedType string

const (
	// GscDauntlessAndreiboard is a traditional AndreiBoard with dozens of wires to a
	// HyperDebug according to:
	// https://docs.google.com/spreadsheets/d/1youX_Yh2A6-Zd2T98ShjH_O8M9CZexDB9DCHpegNMvE
	GscDauntlessAndreiboard TestbedType = "gsc_dt_ab"

	// GscOpentitanCw310Fpga is a ChipWhisperer 310 FPGA board connected via ribbon cables to
	// a "swizzle board" on top of HyperDebug.
	GscOpentitanCw310Fpga TestbedType = "gsc_ot_fpga_cw310"

	// GscHostEmulation is not a physical testbed, but an emulation on a Linux host computer.
	GscHostEmulation TestbedType = "gsc_he"
)

// AllTestbedTypes returns all the possible testbed types.
func AllTestbedTypes() []TestbedType {
	return []TestbedType{GscDauntlessAndreiboard, GscOpentitanCw310Fpga, GscHostEmulation}
}
