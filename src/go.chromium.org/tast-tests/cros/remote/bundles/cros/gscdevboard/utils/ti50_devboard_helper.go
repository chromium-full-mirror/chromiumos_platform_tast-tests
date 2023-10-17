// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-tpm/legacy/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

var (
	// gpioOutput specifies the output format of OpenTitanTool. Quotes are part of the output
	// after we started passing in --format=json flag. The ? for the quotes could be dropped once
	// the newer docker images are used everywhere
	gpioOutput = regexp.MustCompile("\"?value\"?: (true|false)")
)

// GpioEdge represent either rising or falling
type GpioEdge string

const (
	// GpioEdgeRising represents a rising edge
	GpioEdgeRising GpioEdge = "Rising"
	// GpioEdgeFalling represents a falling edge
	GpioEdgeFalling GpioEdge = "Falling"
)

// GpioEvent expresses a rising or falling edge with its corresponding timestamp
type GpioEvent struct {
	// Name of the gpio
	Name ti50.GpioName
	// Edge specifies which edge type the event is
	Edge GpioEdge
	// TimestampUS contains the number of micros elapsed since the gpio monitoring started
	TimestampUS uint64
}

// GpioMonitorSession represents a list of GpioNames that are currently being monitoring.
type GpioMonitorSession struct {
	// Gpios is the list of signals being monitored in this session
	Gpios []ti50.GpioName
	// InitialValues contains the initial value of each GpioName that is being monitored
	InitialValues map[ti50.GpioName]bool
	// AbsoluteTimestampUS specified the absolute time measurement started
	AbsoluteTimestampUS uint64
}

// GpioEvents stores all of the GpioEvent objects that were recorded via gpio monitoring
type GpioEvents struct {
	// Events stores the events by GpioName
	Events map[ti50.GpioName][]GpioEvent
	// Sorted stores events in the time order that they occurred
	Sorted []GpioEvent
}

// FindFirst returns the first gpio event that matches the specified args
func (e GpioEvents) FindFirst(name ti50.GpioName, edge GpioEdge) *GpioEvent {
	for _, event := range e.Events[name] {
		if event.Edge == edge {
			return &event
		}
	}
	return nil
}

// FindLastBefore returns the last gpio event that matches the specified args that happens before
// the passed in event
func (e GpioEvents) FindLastBefore(target GpioEvent, name ti50.GpioName) (match *GpioEvent) {
	for _, event := range e.Events[name] {
		if event.TimestampUS < target.TimestampUS {
			var copy = event
			match = &copy
		}
	}
	return match
}

// FindFirstAfter returns the first gpio event that matches the specified args that happens after
// the passed in event
func (e GpioEvents) FindFirstAfter(target GpioEvent, name ti50.GpioName) *GpioEvent {
	for _, event := range e.Events[name] {
		if event.TimestampUS > target.TimestampUS {
			return &event
		}
	}
	return nil
}

// String prints the gpio events in a table for logging
func (e GpioEvents) String() string {
	events := make([]string, len(e.Sorted))
	for i := range e.Sorted {
		events[i] = fmt.Sprintf("\t%-15s\t%s\t%dms", e.Sorted[i].Name, e.Sorted[i].Edge, e.Sorted[i].TimestampUS/1000)
	}
	return "\n" + strings.Join(events, "\n")
}

// DevboardHelper wraps a DevBoard service and adds higher level commands that tests should use
type DevboardHelper struct {
	ti50.DevBoard
	FirmwareTestingHelperDelegate
	TestbedType ti50.TestbedType
}

// NewDevboardHelper creates a new object from a DevBoard and testing state
func NewDevboardHelper(f *fixture.Value, state FirmwareTestingHelperDelegate) DevboardHelper {
	return DevboardHelper{f.DevBoard(), state, f.TestbedType}
}

// GpioSet sets a well-defined gpio to a value, and if there are any errors, set a fatal
// condition on the test state
func (h DevboardHelper) GpioSet(ctx context.Context, g ti50.GpioName, val bool) {
	if _, err := h.PlainCommand(ctx, "gpio", "write", string(g), strconv.FormatBool(val)); err != nil {
		h.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

// GpioGet gets a well-defined gpio value, and if there are any errors, set a fatal
// condition on the test state
func (h DevboardHelper) GpioGet(ctx context.Context, g ti50.GpioName) bool {
	output, err := h.PlainCommand(ctx, "gpio", "read", string(g))
	if err != nil {
		h.Fatalf("failed to get gpio %s: %s", g, err)
	}
	matches := gpioOutput.FindSubmatch(output)
	if len(matches) != 2 {
		h.Fatalf("invalid gpio value: %s", string(output))
	}
	val, _ := strconv.ParseBool(string(matches[1]))
	return val
}

// GpioApplyStrap applies one or more known gpio strap setting
func (h DevboardHelper) GpioApplyStrap(ctx context.Context, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := h.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
			h.Fatalf("failed to apply gpio strap %s: %s", strap, err)
		}
	}
}

type initialLevels struct {
	Name  ti50.GpioName `json:"signal_name"`
	Value bool          `json:"value"`
}
type monitorStartOutput struct {
	InitialLevels []initialLevels `json:"initial_levels"`
	Time          uint64          `json:"timestamp"`
}

// GpioMonitorStart starts monitoring the specified gpio values
func (h DevboardHelper) GpioMonitorStart(ctx context.Context, gpios ...ti50.GpioName) (session GpioMonitorSession) {
	args := make([]string, len(gpios)+2)
	args[0] = "monitoring"
	args[1] = "start"
	for i := range gpios {
		args[i+2] = string(gpios[i])
	}
	startOutput, err := h.PlainCommand(ctx, "gpio", args...)
	if err != nil {
		h.Fatalf("failed to start gpio monitoring: %s. %s", args, err)
	}

	output := monitorStartOutput{}
	if err := json.Unmarshal(startOutput, &output); err != nil {
		h.Fatalf("failed to parse start gpio monitoring output: %s. %s", string(startOutput), err)
	}

	// Set timestamp so we can use it to calculate relative timestamps later
	session.AbsoluteTimestampUS = output.Time

	session.Gpios = gpios
	session.InitialValues = make(map[ti50.GpioName]bool, len(gpios))
	for _, signal := range output.InitialLevels {
		session.InitialValues[signal.Name] = signal.Value
	}

	// If we don't have an initial value for everything, the we never actually started monitoring
	if len(session.InitialValues) != len(gpios) {
		h.Fatalf("Monitoring did not start. Check /var/log/dev*")
	}

	return session
}

type gpioEvent struct {
	Name ti50.GpioName `json:"signal_name"`
	Edge GpioEdge      `json:"edge"`
	Time uint64        `json:"timestamp"`
}
type monitorFinishOutput struct {
	Events []gpioEvent `json:"events"`
}

// GpioMonitorRead retrieves the list of events so far the specified gpio monitoring session, the
// monitoring continues, and must be eventually stopped by a call to GpioMonitorFinish.
func (h DevboardHelper) GpioMonitorRead(ctx context.Context, session GpioMonitorSession) (events GpioEvents) {
	return h.gpioMonitorRead(ctx, session, false)
}

// GpioMonitorFinish finishes gpio monitoring for the specified session
func (h DevboardHelper) GpioMonitorFinish(ctx context.Context, session GpioMonitorSession) (events GpioEvents) {
	return h.gpioMonitorRead(ctx, session, true)
}

func (h DevboardHelper) gpioMonitorRead(ctx context.Context, session GpioMonitorSession, finish bool) (events GpioEvents) {
	args := make([]string, 2)
	args[0] = "monitoring"
	args[1] = "read"
	if !finish {
		args = append(args, "--continue-monitoring")
	}
	// opentitantool requires that the pin names be given in exactly the same order as they
	// were to `monitoring start`.
	for i := range session.Gpios {
		args = append(args, string(session.Gpios[i]))
	}
	readOutput, err := h.PlainCommand(ctx, "gpio", args...)
	if err != nil {
		h.Fatalf("gpio monitoring error: %s", err)
	}

	output := monitorFinishOutput{}
	if err := json.Unmarshal(readOutput, &output); err != nil {
		h.Fatalf("failed to parse gpio monitoring output: %s. %s", string(readOutput), err)
	}

	events.Events = make(map[ti50.GpioName][]GpioEvent)
	for _, event := range output.Events {
		event := GpioEvent{
			Name:        event.Name,
			Edge:        event.Edge,
			TimestampUS: event.Time - session.AbsoluteTimestampUS,
		}
		events.Sorted = append(events.Sorted, event)
		events.Events[event.Name] = append(events.Events[event.Name], event)
	}

	return events
}

// Tpm returns an object that can be used with the go-tpm library to execute TPM commands via SPI
// or I2C.  See firmware.Ti50Tpm for an example.
// go-tpm documentation: https://pkg.go.dev/github.com/google/go-tpm@v0.3.3/tpm2
func (h DevboardHelper) Tpm(ctx context.Context, bus ti50.TpmBus) *TpmHelper {
	return &TpmHelper{ti50.NewTpmHandle(ctx, h, bus), h}
}

// ExpectedDidVidValue returns the value expected when reading the TPM DID_VID register (differs
// between Cr50 and Ti50).
func (h DevboardHelper) ExpectedDidVidValue(ctx context.Context) []byte {
	if h.TestbedType == ti50.GscHavenShield {
		return ti50.TpmCr50DidVidValue
	}
	return ti50.TpmTi50DidVidValue
}

// ResetAndTpmStartup resets the board with I2C or SPI TPM strap, reads TpmRegDidVid, then sends
// tpm2.Startup command.
func (h DevboardHelper) ResetAndTpmStartup(ctx context.Context, i *ti50.CrOSImage, bus ti50.TpmBus, straps ...ti50.GpioStrap) *TpmHelper {
	var busConfig ti50.GpioStrap
	switch bus {
	case ti50.TpmBusSpi:
		straps = append(straps, ti50.TpmSpi)
		busConfig = ti50.ApOnSpi
	case ti50.TpmBusI2c:
		straps = append(straps, ti50.TpmI2c)
		busConfig = ti50.ApOnI2c
	}
	// Turn off the AP while reading the straps.
	straps = append(straps, ti50.ApOff)

	testing.ContextLogf(ctx, "Restarting Ti50 for %s TPM", bus)
	h.GpioApplyStrap(ctx, straps...)

	th := FirmwareTestingHelper{FirmwareTestingHelperDelegate: h}
	th.MustSucceed(h.Reset(ctx), "Reset board")
	m, err := h.ReadSerialSubmatch(ctx, regexp.MustCompile(`Strap config: .* TPM Bus: ([^;]+);`))
	if err != nil || strings.ToLower(string(m[1])) != string(bus) {
		h.Fatalf("Wrong TPM strap")
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	h.GpioApplyStrap(ctx, busConfig)

	tpmHandle := h.Tpm(ctx, bus)

	// Try reading DidVid a few times until Ti50 is ready.
	const maxDidVidAttempts = 3
	expectedDidVidValue := h.ExpectedDidVidValue(ctx)
	for r := 1; r <= maxDidVidAttempts; r++ {
		// Avoid using tpmHandle.ReadRegister(), as doing so would instantly fail the test
		// in case of timeout or other errors, instead directly call lower-level method.
		didVid, err := tpmHandle.OpenTitanToolTpmCommand("read-register", string(ti50.TpmRegDidVid))
		if err != nil {
			if r == maxDidVidAttempts {
				h.Fatalf("Repeated errors reading TPM DID_VID: %s", err)
			}
			continue
		}
		if bytes.Equal(didVid, expectedDidVidValue) {
			break
		}
		if r == maxDidVidAttempts {
			h.Fatalf("Repeated unexpected TPM DID_VID: %v", didVid)
		}
		if bytes.Equal(didVid, []byte{0xff, 0xff, 0xff, 0xff}) {
			// Common result, in case the GSC is completely unresponsive on the SPI
			// bus at the time of the request.
			continue
		}
		if bytes.Equal(didVid, []byte{0xff, 0x01, 0xe0, 0x1a}) {
			// Common way for Cr50 DID_VID to be corrupted on the SPI bus, if Cr50 not
			// fully initialized at the time of the request.
			continue
		}
		h.Fatalf("Unexpected TPM DID_VID: %v", didVid)
	}

	if err := tpm2.Startup(tpmHandle, tpm2.StartupClear); err != nil {
		h.Fatalf("TPM startup error: %v", err)
	}

	return tpmHandle
}
