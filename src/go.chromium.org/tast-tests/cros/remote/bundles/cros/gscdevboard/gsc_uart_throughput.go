// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUARTThroughput,
		Desc:    "Tests forwarding between GSC UARTs and USB at sustained maximum throughput",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name:      "basic",
			Val:       false,
			ExtraAttr: []string{"gsc_h1_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "endless_crypto",
			// Run crypto operations in the background to validate it doesn't interfere with UART operations.
			Val: true,
		}},
	})
}

const (
	uartThroughputBlockSize            int = 64
	uartThroughputNumWarmupBlocks      int = 16
	uartThroughputNumMeasurementBlocks int = 32768

	// uartBitsPerByte represents how many bit times it takes to transmit a byte on UART.
	uartBitsPerByte int = 10

	uartThroughputNominalBps   float64 = 115200.0
	uartThroughputBpsTolerance float64 = 400.0
)

type consoleChannel struct {
	name  ti50.UartName
	magic byte
	// CCD USB endpoint.
	ccd ti50.SerialChannel
	// UART console.
	uart ti50.SerialChannel
}

func GSCUARTThroughput(ctx context.Context, s *testing.State) {
	const crLf = "\r\n"
	const ecMagic byte = 0xEC
	const apMagic byte = 0xA5
	const fpmcuMagic byte = 0xF5

	var consoles []consoleChannel

	b := utils.NewDevboardHelper(s)
	gscProps := b.GscProperties()
	runEndlessCrypto := s.Param().(bool)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("(Re)starting ti50")
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusI2c, ti50.CcdSuzyQ, ti50.ServoMicroDisconnected)

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Set up the EC console.
	consoles = append(consoles, consoleChannel{
		name:  ti50.UartEC,
		magic: ecMagic,
		ccd:   b.CcdSerialInterface(ti50.UartEC, time.Second),
		uart:  b.PhysicalUart(ti50.UartEC, time.Second),
	})

	// Set up the AP console.
	consoles = append(consoles, consoleChannel{
		name:  ti50.UartAP,
		magic: apMagic,
		ccd:   b.CcdSerialInterface(ti50.UartAP, time.Second),
		uart:  b.PhysicalUart(ti50.UartAP, time.Second),
	})

	// Set up the FPMCU console on chips that support it.
	if gscProps.HasFpmcuUart() {
		consoles = append(consoles, consoleChannel{
			name:  ti50.UartFPMCU,
			magic: fpmcuMagic,
			ccd:   b.CcdSerialInterface(ti50.UartFPMCU, time.Second),
			uart:  b.PhysicalUart(ti50.UartFPMCU, time.Second),
		})
	}

	// Open the CCD and UART interfaces.
	for _, console := range consoles {
		th.MustSucceed(console.ccd.Open(ctx), fmt.Sprintf("Failed to open %s ccd", console.name))
		defer console.ccd.Close(ctx)
		th.MustSucceed(console.uart.Open(ctx), fmt.Sprintf("Failed to open %s uart", console.name))
		defer console.uart.Close(ctx)
	}

	// Flush out any "DATA LOST" message along with other queued-up data.
	for _, console := range consoles {
		console.uart.WriteSerial(ctx, []byte(crLf))
		th.MustSucceed(console.ccd.ClearInput(ctx), "Error clearing buffer")
		th.MustSucceed(console.uart.ClearInput(ctx), "Error clearing buffer")
	}

	// Start out by sending some initial data on all three UARTS, to fill up the buffers.

	var sendBlockNo = 0
	var recvBlockNo = 0

	for ; sendBlockNo < uartThroughputNumWarmupBlocks; sendBlockNo++ {
		// Transmit block on each UART.
		for _, console := range consoles {
			sendIteration(ctx, s, th, console.uart, console.magic, sendBlockNo)
		}
	}

	// Start a separate goroutine, which will repeatedly create RSA keys in order to load the
	// GSC with expensive crypto operations, until told to stop.  (Except on OpenTitan, which
	// does not yet have hardware cryptolib).
	stop := make(chan bool)
	done := make(chan bool)
	if runEndlessCrypto {
		s.Log("Running crypto in the background")
		go endlessCrypto(tpm, s, stop, done)
	}

	// Now, read and verify one 64-byte block of data from each of the three CCD endpoints,
	// followed by transmitting yet another block on each UART.  This way, we ensure that the
	// amount of in-transit data is bounded.
	start := time.Now()
	for ; recvBlockNo < uartThroughputNumWarmupBlocks+uartThroughputNumMeasurementBlocks; recvBlockNo, sendBlockNo = recvBlockNo+1, sendBlockNo+1 {
		// Read a block from each console USB endpoint.
		for _, console := range consoles {
			recvIteration(ctx, s, th, console.ccd, console.magic, recvBlockNo)
		}
		// Transmit block on each UART.
		for _, console := range consoles {
			sendIteration(ctx, s, th, console.uart, console.magic, sendBlockNo)
		}
	}
	elapsed := time.Since(start)

	var bps = float64(uartThroughputNumMeasurementBlocks*uartThroughputBlockSize*uartBitsPerByte) / elapsed.Seconds()
	s.Logf("Effective transfer speed: %.02f kbps", bps/1000)
	if bps > uartThroughputNominalBps+uartThroughputBpsTolerance {
		s.Error("Transfer speed too fast, something is not right")
	}
	if bps < uartThroughputNominalBps-uartThroughputBpsTolerance {
		s.Error("Transfer speed too slow, HyperDebug may not be performing")
	}

	// Tell the endlessCrypto() goroutine to stop, and wait for it to finish any ongoing
	// operation.
	if runEndlessCrypto {
		stop <- true
		<-done
	}
}

// recvIteration receives a block of data from one specific UART, verifying that it was as expected.
func recvIteration(ctx context.Context, s *testing.State, th utils.FirmwareTestingHelper, ccd ti50.SerialChannel, magic byte, iteration int) {
	databuf, err := ccd.ReadSerialBytes(ctx, uartThroughputBlockSize)
	th.MustSucceed(err, "Read error")

	// All validation errors reported as "Fatal", in order to avoid thousands of lines of
	// error messages, as all future data would fail validation in case of a dropped sequence.
	if databuf[0] != byte(iteration) ||
		databuf[1] != byte(iteration>>8) ||
		databuf[2] != byte(iteration>>16) ||
		databuf[3] != byte(iteration>>24) {
		s.Fatal("Incorrect sequence number")
	}
	if databuf[4] != magic {
		s.Fatal("Incorrect magic")
	}
	var idx = 5
	for idx < uartThroughputBlockSize {
		if databuf[idx] != byte(idx) {
			s.Fatal("Incorrect data contents")
		}
		idx = idx + 1
	}
}

// sendIteration sends a block of data on one specific UART.
func sendIteration(ctx context.Context, s *testing.State, th utils.FirmwareTestingHelper, uart ti50.SerialChannel, magic byte, iteration int) {
	databuf := make([]byte, uartThroughputBlockSize)
	var idx = 0
	for idx < uartThroughputBlockSize {
		databuf[idx] = byte(idx)
		idx = idx + 1
	}
	databuf[0] = byte(iteration)
	databuf[1] = byte(iteration >> 8)
	databuf[2] = byte(iteration >> 16)
	databuf[3] = byte(iteration >> 24)

	databuf[4] = magic
	th.MustSucceed(uart.WriteSerial(ctx, databuf), "Write error")
}

func endlessCrypto(tpm *utils.TpmHelper, s *testing.State, stop, done chan bool) {
	cp := tpm2.CreatePrimary{
		PrimaryHandle: tpm2.TPMRHOwner,
		InPublic: tpm2.New2B(
			tpm2.TPMTPublic{
				Type:    tpm2.TPMAlgRSA,
				NameAlg: tpm2.TPMAlgSHA1,
				ObjectAttributes: tpm2.TPMAObject{
					Decrypt:             true,
					Restricted:          true,
					FixedTPM:            true,
					FixedParent:         true,
					SensitiveDataOrigin: true,
					UserWithAuth:        true,
				},
				Parameters: tpm2.NewTPMUPublicParms(tpm2.TPMAlgRSA,
					&tpm2.TPMSRSAParms{
						Symmetric: tpm2.TPMTSymDefObject{
							Algorithm: tpm2.TPMAlgAES,
							KeyBits: tpm2.NewTPMUSymKeyBits(
								tpm2.TPMAlgAES,
								tpm2.TPMKeyBits(128),
							),
							Mode: tpm2.NewTPMUSymMode(
								tpm2.TPMAlgAES,
								tpm2.TPMAlgCFB,
							),
						},
						KeyBits:  2048,
						Exponent: 1<<16 + 1,
					}),
			}),
	}

	for {
		select {
		case <-stop:
			done <- true
			return
		default:
			// Generate RSA key pair.
			resp, err := cp.Execute(tpm)
			if err != nil {
				s.Fatalf("Error creating RSA key: %s", err)
			}

			// Delete newly generated key, in order to not overflow TPM storage.
			flush := tpm2.FlushContext{
				FlushHandle: resp.ObjectHandle,
			}
			flush.Execute(tpm)
		}
	}
}
