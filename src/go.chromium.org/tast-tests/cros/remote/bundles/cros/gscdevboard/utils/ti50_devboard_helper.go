// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	remoteTi50 "go.chromium.org/tast-tests/cros/remote/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// gpioOutput specifies the output format of OpenTitanTool. Quotes are part of the output
	// after we started passing in --format=json flag. The ? for the quotes could be dropped once
	// the newer docker images are used everywhere
	gpioOutput = regexp.MustCompile("\"?value\"?: (true|false)")
)

// GpioMode represents a mode of a debugger pin.
type GpioMode string

const (
	// GpioModeInput means the pin is used for digital input.
	GpioModeInput GpioMode = "Input"
	// GpioModeOpenDrain means the pin is used for I/O in open drain mode.
	GpioModeOpenDrain GpioMode = "OpenDrain"
	// GpioModePushPull means the pin is used for digital output.
	GpioModePushPull GpioMode = "PushPull"
	// GpioModeAnalogInput means the pin is used to measure an analog voltage.
	GpioModeAnalogInput GpioMode = "AnalogInput"
	// GpioModeAnalogOutput means the pin is used to drive an analog voltage.
	GpioModeAnalogOutput GpioMode = "AnalogOutput"
	// GpioModeAlternate means the pin is used in some special mode (UART, SPI, I2C, ...)
	// the exact alternate functionality supported by each pin depends on the debugger..
	GpioModeAlternate GpioMode = "Alternate"
)

// GpioPullMode represents a weak pull mode of a debugger pin (which will take effect if neither
// the debugger nor the GSC are strongly driving the pin.)
type GpioPullMode string

const (
	// GpioPullNone means no weak pulling of the pin.
	GpioPullNone GpioPullMode = "None"
	// GpioPullUp means a weak pull towards logic high level (often used with
	// GpioModeOpenDrain).
	GpioPullUp GpioPullMode = "PullUp"
	// GpioPullDown means a weak pull towards logic low level (rarely used).
	GpioPullDown GpioPullMode = "PullDown"
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
	// ElapsedUS contains the number of micros elapsed since the gpio monitoring started.
	// All signals can be assumed to have remained stable from their last recorded event
	// until this time.
	ElapsedUS uint64
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

// FindLast returns the last gpio event that matches the specified args
func (e GpioEvents) FindLast(name ti50.GpioName, edge GpioEdge) (match *GpioEvent) {
	for _, event := range e.Events[name] {
		if event.Edge == edge {
			match = &event
		}
	}
	return match
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

// Save stores the sequence of captured GPIO events in the "Value Change Dump" format, which can
// be loaded into logic analyzer programs, such as the open source Pulseview.
func (s GpioMonitorSession) Save(ctx context.Context, e GpioEvents, filename string) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get directory for saving events")
	}
	path := filepath.Join(dir, filename)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to create file `%s`", path)
	}
	defer f.Close()
	fmt.Fprintln(f, "$version")
	fmt.Fprintln(f, "   ti50_devboard_helper.go")
	fmt.Fprintln(f, "$end")
	fmt.Fprintln(f, "$timescale 1000000ps $end")
	fmt.Fprintln(f, "$scope module logic $end")

	revGpio := make(map[ti50.GpioName]int)
	for i := range s.Gpios {
		fmt.Fprintf(f, "$var wire 1 '%d %s $end\n", i, s.Gpios[i])
		revGpio[s.Gpios[i]] = i
	}

	fmt.Fprintln(f, "$upscope $end")
	fmt.Fprintln(f, "$enddefinitions $end")
	fmt.Fprintf(f, "#%d\n", s.AbsoluteTimestampUS)
	for i := range s.Gpios {
		if s.InitialValues[s.Gpios[i]] {
			fmt.Fprintf(f, "1'%d\n", i)
		} else {
			fmt.Fprintf(f, "0'%d\n", i)
		}
	}
	for i := range e.Sorted {
		fmt.Fprintf(f, "#%d\n", e.Sorted[i].TimestampUS+s.AbsoluteTimestampUS)
		switch e.Sorted[i].Edge {
		case GpioEdgeRising:
			fmt.Fprintf(f, "1'%d\n", revGpio[e.Sorted[i].Name])
		case GpioEdgeFalling:
			fmt.Fprintf(f, "0'%d\n", revGpio[e.Sorted[i].Name])
		}
	}
	fmt.Fprintf(f, "#%d\n", s.AbsoluteTimestampUS+e.ElapsedUS)

	testing.ContextLogf(ctx, "Recorded %d GPIO events of %s in `%s`", len(e.Sorted), s.Gpios, filename)
	return nil
}

// DevboardHelper wraps a DevBoard service and adds higher level commands that tests should use
type DevboardHelper struct {
	*remoteTi50.DUTControlAndreiboard
	ti50.SerialChannel
	FirmwareTestingHelperDelegate
	TestbedType ti50.TestbedType
}

// NewDevboardHelper creates a new object from testing state provided by fixture
func NewDevboardHelper(s *testing.State) DevboardHelper {
	f := s.FixtValue().(*fixture.Value)
	b := f.DevBoard()
	gscConsole := b.PhysicalUart(ti50.UartConsole, time.Second)
	return DevboardHelper{b, gscConsole, s, f.TestbedType}
}

// GscProperties returns an object that can be queried about varios aspects of the GSC currently
// under test.
func (h DevboardHelper) GscProperties() GscProperties {
	if h.TestbedType == ti50.GscH1Shield {
		return &gscCr50{}
	}
	return &gscTi50{}
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

// GpioMultiSet configures a gpio pin in a particular logic level, drive mode, and weak pull
// mode.  If there are any errors, set a fatal condition on the test state
func (h DevboardHelper) GpioMultiSet(ctx context.Context, g ti50.GpioName, val bool, m GpioMode, p GpioPullMode) {
	args := []string{"set", string(g)}
	args = append(args, "--mode", string(m))
	if m == GpioModePushPull || m == GpioModeOpenDrain {
		args = append(args, "--value", strconv.FormatBool(val))
	}
	args = append(args, "--pull", string(p))
	if _, err := h.PlainCommand(ctx, "gpio", args...); err != nil {
		h.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

// GpioApplyStrap applies one or more known gpio strap setting
func (h DevboardHelper) GpioApplyStrap(ctx context.Context, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := h.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
			h.Fatalf("failed to apply gpio strap %s: %s", strap, err)
		}
	}
}

// GpioRemoveStrap removes one or more known gpio strap setting
func (h DevboardHelper) GpioRemoveStrap(ctx context.Context, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := h.PlainCommand(ctx, "gpio", "remove", string(strap)); err != nil {
			h.Fatalf("failed to remove gpio strap %s: %s", strap, err)
		}
	}
}

// ResetWithStraps applies straps while resetting the chip
func (h DevboardHelper) ResetWithStraps(ctx context.Context, straps ...ti50.GpioStrap) {
	h.GpioApplyStrap(ctx, ti50.StrapReset)
	for _, strap := range straps {
		h.GpioApplyStrap(ctx, strap)
	}
	h.GpioRemoveStrap(ctx, ti50.StrapReset)
}

// Reset resets the chip
func (h DevboardHelper) Reset(ctx context.Context) {
	h.ResetWithStraps(ctx)
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
	Events  []gpioEvent `json:"events"`
	EndTime uint64      `json:"timestamp"`
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
	events.ElapsedUS = output.EndTime - session.AbsoluteTimestampUS
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

	// Ensure the GSC UART is open and collecting results before we reset GSC to ensure that
	// the UART messages right after GSC reset are captured.
	testing.ContextLogf(ctx, "Restarting Ti50 for %s TPM", bus)
	h.ResetWithStraps(ctx, straps...)
	// TODO(b/305814102): check board properties on H1 to verify SPI vs I2C
	if h.TestbedType != ti50.GscH1Shield {
		// Ti50 prints "I2C" or "SPI" based on the TPM Bus type.
		m, err := h.ReadSerialSubmatch(ctx, regexp.MustCompile(`Strap config: .* TPM Bus: ([^;]+);`))
		if err != nil {
			h.Fatalf("Could not find TPM strap: %s", err)
		} else if strings.ToLower(string(m[1])) != string(bus) {
			h.Fatalf("Wrong TPM strap: got %s, wanted %s", m[1], bus)
		}
	}
	th := FirmwareTestingHelper{FirmwareTestingHelperDelegate: h}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	h.GpioApplyStrap(ctx, busConfig)

	tpmHandle := h.Tpm(ctx, bus)

	// Try reading DidVid a few times until Ti50 is ready.
	const maxDidVidAttempts = 3
	expectedDidVidValue := h.GscProperties().ExpectedDidVidValue()
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

// WithApFlashAccess runs `f` with the proper setup and teardown to access the SPI flash chip.
// This function asserts the SuzyQ strapping and leaves it in that state, so `gsctool` should work immediately.
func (h DevboardHelper) WithApFlashAccess(ctx context.Context, i *ti50.CrOSImage, holdReset ti50.HoldReset, f func(ti50.ApFlash)) {
	h.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	h.WaitUntilCCDConnected(ctx)

	flash := remoteTi50.NewApFlash(h.DUTControlAndreiboard)
	if _, err := flash.FetchApFlashInfo(ctx); err != nil {
		h.Fatalf("Could not get ap flash info: %s", err)
	}
	// TODO(kupiakos): consider warning/erroring if an unrecognized chip is seen
	// on the andreishield, or pass this info to `f` so it can check itself.
	f(flash)
	// Reset the GSC
	h.GpioApplyStrap(ctx, ti50.StrapReset)
	if !holdReset {
		h.GpioRemoveStrap(ctx, ti50.StrapReset)
		if err := i.WaitUntilBooted(ctx); err != nil {
			h.Fatalf("GSC did not restart: %s", err)
		}
	}
}

// WaitUntilCCDConnected waits until CCD is connected, using gsctool to check.
func (h DevboardHelper) WaitUntilCCDConnected(ctx context.Context) {
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := h.GSCToolCommand(ctx, "", "--fwver")
		return err
	}, &pOpts)
	if err != nil {
		h.Fatalf("CCD did not connect: %s", err)
	}
}

// CCDMustNotBeConnected verifies there is no CCD connection for the specified duration
func (h DevboardHelper) CCDMustNotBeConnected(ctx context.Context, duration time.Duration) {
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: duration}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := h.GSCToolCommand(ctx, "", "--fwver")
		return err
	}, &pOpts)
	if err == nil {
		h.Fatalf("CCD connect unexpectedly")
	}
}
