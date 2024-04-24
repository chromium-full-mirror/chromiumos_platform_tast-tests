// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

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
	gscConsole := b.PhysicalUart(ti50.UartConsole)
	return DevboardHelper{b, gscConsole, s, f.TestbedProperties.TestbedType}
}

// GscProperties returns an object that can be queried about various aspects of the GSC currently
// under test.
func (h DevboardHelper) GscProperties() GscProperties {
	switch h.TestbedType {
	case ti50.GscH1Shield:
		return &gscCr50{}
	case ti50.GscOpentitanCw310Fpga:
		return &gscOT{}
	case ti50.GscOTShield:
		return &gscOT{}
	case ti50.GscHostEmulation:
		return &gscHE{}
	default:
		return &gscDT{}
	}
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

type bitbangingOutput struct {
	Samples map[string]string `json:"samples"`
	All     []string          `json:"all"`
}

// GpioBitbang controls given set of GPIO pins with precise timing
func (h DevboardHelper) GpioBitbang(ctx context.Context, clock, waveform string, collectAll bool, gpios ...ti50.GpioName) BitbangOutput {
	args := make([]string, len(gpios)+5)
	args[0] = "bitbang"
	args[1] = "--clock"
	args[2] = clock
	args[3] = "-s"
	args[4] = waveform
	for i := range gpios {
		args[i+5] = string(gpios[i])
	}
	if collectAll {
		args = append(args, "--all")
	}
	startOutput, err := h.PlainCommand(ctx, "gpio", args...)
	if err != nil {
		h.Fatalf("failed to preform gpio bit-banging: %s. %s", args, err)
	}

	output := bitbangingOutput{}
	if err := json.Unmarshal(startOutput, &output); err != nil {
		h.Fatalf("failed to parse start gpio monitoring output: %s. %s", string(startOutput), err)
	}
	result := BitbangOutput{
		Samples: make(map[string]map[ti50.GpioName]bool, len(output.Samples)),
		All:     make([]map[ti50.GpioName]bool, len(output.All)),
	}
	for label, levels := range output.Samples {
		sample := make(map[ti50.GpioName]bool, len(gpios))
		for i := 0; i < len(gpios); i++ {
			sample[gpios[i]] = levels[i] == '1'
		}
		result.Samples[label] = sample
	}
	for i, levels := range output.All {
		sample := make(map[ti50.GpioName]bool, len(gpios))
		for i := 0; i < len(gpios); i++ {
			sample[gpios[i]] = levels[i] == '1'
		}
		result.All[i] = sample
	}
	return result
}

// BitbangOutput holds the output of a bit bang operation.
type BitbangOutput struct {
	Samples map[string]map[ti50.GpioName]bool
	All     []map[ti50.GpioName]bool
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

// GpioMonitorWait waits until a GPIO event happens.
func (h DevboardHelper) GpioMonitorWait(ctx context.Context, session GpioMonitorSession, timeout, interval time.Duration) GpioEvents {
	events := GpioEvents{}
	pOpts := testing.PollOptions{Interval: interval, Timeout: timeout}
	testing.Poll(ctx, func(ctx context.Context) error {
		events = h.GpioMonitorRead(ctx, session)
		if len(events.Sorted) != 0 {
			return nil
		}
		return errors.New("failed to detect gpio event")
	}, &pOpts)
	return events
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

// ResetAndTpmStartup resets the board with preferred TPM bus, then reads TpmRegDidVid, then sends
// tpm2.Startup command.
func (h DevboardHelper) ResetAndTpmStartup(ctx context.Context, i *ti50.CrOSImage, straps ...ti50.GpioStrap) *TpmHelper {
	return h.ResetAndTpmStartupForBus(ctx, i, h.GscProperties().PreferredTPMBus(), straps...)
}

// WaitForTpm wait until GSC responds with the correct DID VID.
func (h DevboardHelper) WaitForTpm(ctx context.Context, tpmHandle *TpmHelper) {
	// Try reading DidVid a few times until Ti50 or Cr50 is ready.
	const maxDidVidAttempts = 6
	expectedDidVidValue := h.GscProperties().ExpectedDidVidValue()
	for r := 1; r <= maxDidVidAttempts; r++ {
		// Avoid using tpmHandle.ReadRegister(), as doing so would instantly fail the test
		// in case of timeout or other errors, instead directly call lower-level method.
		testing.ContextLogf(ctx, "TPM DID_VID verification attempt %d/%d", r, maxDidVidAttempts)
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
		// If the test reads the DID_VID before Cr50 is ready, it'll
		// respond with 0xff in the first couple of bytes. Ignore H1
		// reads until the data is valid.
		if h.TestbedType == ti50.GscH1Shield && len(didVid) > 0 && didVid[0] == 0xff {
			testing.ContextLog(ctx, "Cr50 not ready - DID_VID ", didVid)
			continue
		}
		h.Fatalf("Unexpected TPM DID_VID: %v", didVid)
	}
}

// ResetAndTpmStartupForBus resets the board specified TPM bus strap, then reads TpmRegDidVid, then
// sends tpm2.Startup command.
func (h DevboardHelper) ResetAndTpmStartupForBus(ctx context.Context, i *ti50.CrOSImage, bus ti50.TpmBus, straps ...ti50.GpioStrap) *TpmHelper {
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
	th := FirmwareTestingHelper{FirmwareTestingHelperDelegate: h}
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	actualBus, err := i.GetBoardPropertiesTPMBus(ctx)
	if err != nil {
		h.Fatalf("Error running brdprop: %v", err)
	}
	if actualBus != bus {
		h.Fatalf("Unexpected TPM bus: board reported %s, expected %s", actualBus, bus)
	}

	// Tell Ti50 that the AP came out of reset.  This will cause Ti50 to start responding to
	// TPM commands.
	h.GpioApplyStrap(ctx, busConfig)

	tpmHandle := h.Tpm(ctx, bus)
	h.WaitForTpm(ctx, tpmHandle)

	startup := tpm2.Startup{
		StartupType: tpm2.TPMSUClear,
	}
	if _, err := startup.Execute(tpmHandle); err != nil {
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

	// Further, wait until Ti50 drives EC UART TX high, indicating that CCD UART forwarding is
	// active.
	startTime := time.Now()
	for h.GpioGet(ctx, ti50.GpioTi50UartDbgTxEcRx) != true {
		if time.Since(startTime) > 5*time.Second {
			h.Fatalf("GSC not driving EC UART high on CCD connection")
			return
		}
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

// I2CHostMode switches an I2C port of the debugger to act as host (this is the default).
func (h DevboardHelper) I2CHostMode(ctx context.Context, bus ti50.I2cBusName) {
	_, err := h.PlainCommand(ctx, "i2c", "--bus", string(bus), "set-mode", "host")
	if err != nil {
		h.Fatalf("failed to switch to I2C device mode: %s", err)
	}
}

// I2CDeviceMode switches an I2C port of the debugger to act as device with the given address.
func (h DevboardHelper) I2CDeviceMode(ctx context.Context, bus ti50.I2cBusName, addr byte) {
	_, err := h.PlainCommand(ctx, "i2c", "--bus", string(bus), "set-mode", "device", "--addr", "0x"+strconv.FormatInt(int64(addr), 16))
	if err != nil {
		h.Fatalf("failed to switch to I2C device mode: %s", err)
	}
}

// I2CDevicePrepareRead instructs the debugger what data to respond with, should the I2C host
// initiate a read transfer in the future (or in case the host has already initiated one, which is
// currently being stalled by the debugger through "clock stretching").
func (h DevboardHelper) I2CDevicePrepareRead(ctx context.Context, bus ti50.I2cBusName, data []byte) {
	_, err := h.PlainCommand(ctx, "i2c", "--bus", string(bus), "prepare-read", "--hexdata", hex.EncodeToString(data), "--sticky")
	if err != nil {
		h.Fatalf("failed to prepare I2C data: %s", err)
	}
}

// I2CDeviceReadStatus represents the current state of I2C bus, with respect to a host attempting
// to read data from the debugger, acting as I2C device.
type I2CDeviceReadStatus string

const (
	// I2CDeviceWaitingForData means that the debugger is currently stretching the clock, as
	// the I2C host is attempting to read data.  `I2CDevicePrepareRead()` needs to be urgently
	// called with data to respond with.  It is uncertain how many milliseconds the I2C host
	// is willing to wait.
	I2CDeviceWaitingForData I2CDeviceReadStatus = "WaitingForData"

	// I2CDeviceIdle means that the I2C host is not currently attempting to read data, but
	// also that the debugger does not have a prepared response, should the host device
	// initiate a read transfer.
	I2CDeviceIdle I2CDeviceReadStatus = "Idle"

	// I2CDeviceDataPrepared means that the debugger has one or more bytes prepared, for when
	// the I2C initiates a read transfer.
	I2CDeviceDataPrepared I2CDeviceReadStatus = "DataPrepared"
)

type i2cDeviceStatusJSON struct {
	Transfers []struct {
		Write struct {
			Addr    byte   `json:"addr"`
			Hexdata string `json:"hexdata"`
		} `json:"Write,omitempty"`
		Read struct {
			Addr    byte `json:"addr"`
			Timeout bool `json:"timeout"`
			Len     int  `json:"len"`
		} `json:"Read,omitempty"`
	} `json:"transfers"`
	ReadStatus I2CDeviceReadStatus `json:"read_status"`
}

// I2CDirection represents the direction of an I2C transfer (read or write), from the perspective
// of the I2C host.
type I2CDirection string

const (
	// I2CRead represents the I2C host receiving data from the I2C device.
	I2CRead I2CDirection = "read"
	// I2CWrite represents the I2C host sending data to the I2C device.
	I2CWrite I2CDirection = "write"
)

// I2CDeviceTransfer is a record of a I2C transfer as it happened on a debugger bus.
type I2CDeviceTransfer struct {
	// Addr is the I2C address to which the debugger responded.
	Addr byte
	// Direction is the direction of an I2C transfer (read or write), from the perspective of
	// the I2C host.
	Direction I2CDirection
	// Len is the number of bytes transferred.
	Len int
	// Data is populated only for `I2CWrite` direction, with `Len` bytes from the I2C host.
	Data []byte
	// Timeout is true if the debugger did not have data, and the I2C host gave up waiting
	// (make sense only for `I2CRead`, for writes Timeout is always false.)
	Timeout bool
}

// I2CDeviceStatus contains a transcript of transactions having happened since last time, as well
// as an indication whether the I2C host is waiting to read data, or whether the debugger has
// prepared data ready, should the host want to read later.
type I2CDeviceStatus struct {
	Transfers  []I2CDeviceTransfer
	ReadStatus I2CDeviceReadStatus
}

// I2CDeviceGetStatus returns a transcript of transactions having happened since last time, as
// well as an indication whether the I2C host is waiting to read data, or whether the debugger has
// prepared data ready, should the host want to read later.
func (h DevboardHelper) I2CDeviceGetStatus(ctx context.Context, bus ti50.I2cBusName) I2CDeviceStatus {
	args := []string{"--bus", string(bus), "get-device-status"}
	outputText, err := h.PlainCommand(ctx, "i2c", args...)
	if err != nil {
		h.Fatalf("failed to retrieve I2C device data: %s", err)
	}
	outputJSON := i2cDeviceStatusJSON{}
	if err := json.Unmarshal(outputText, &outputJSON); err != nil {
		h.Fatalf("failed to parse I2C device data: %q: %s", string(outputText), err)
	}
	output := I2CDeviceStatus{}

	// Convert JSON structs into slightly more convenient structs
	for _, transfer := range outputJSON.Transfers {
		if transfer.Write.Addr != 0 {
			data, err := hex.DecodeString(transfer.Write.Hexdata)
			if err != nil {
				h.Fatalf("failed to parse I2C device hex data: %q: %s", transfer.Write.Hexdata, err)
			}
			output.Transfers = append(output.Transfers, I2CDeviceTransfer{
				Addr:      transfer.Write.Addr,
				Direction: I2CWrite,
				Data:      data,
				Len:       len(data),
				Timeout:   false,
			})
		} else if transfer.Read.Addr != 0 {
			output.Transfers = append(output.Transfers, I2CDeviceTransfer{
				Addr:      transfer.Read.Addr,
				Direction: I2CRead,
				Len:       transfer.Read.Len,
				Timeout:   transfer.Read.Timeout,
			})
		} else {
			h.Fatalf("unrecognized I2C JSON data")
		}
	}
	output.ReadStatus = outputJSON.ReadStatus
	return output
}

const (
	// Ina231ShuntLsbVolts represents "one count" of the least significant bit of the shunt
	// measuring port of INA231.  See https://www.ti.com/lit/ds/symlink/ina231.pdf
	Ina231ShuntLsbVolts = 0.0000025

	// DauntlessShieldShuntOhms is the resistance of the shunt resistor.
	DauntlessShieldShuntOhms = 0.5

	// Pac195xShuntLsbVolts represents "one count" of the least significant bit of the shunt
	// measuring port of PAC195x.  See https://www.microchip.com/en-us/product/pac1954
	Pac195xShuntLsbVolts = 0.0000015

	// OpenTitanShieldShuntOhms is the resistance of the shunt resistor.
	OpenTitanShieldShuntOhms = 0.5

	// H1ShieldShuntOhms is the resistance of the shunt resistor.
	H1ShieldShuntOhms = 0.5
)

// PACAddr is the hex string of the 8-bit PAC195x address.
type PACAddr string

const (
	// PACVSense1 is the address for Vsense1.
	PACVSense1 PACAddr = "0b"

	// PACVSense2 is the address for Vsense2.
	PACVSense2 PACAddr = "0c"

	// PACVSense3 is the address for Vsense3.
	PACVSense3 PACAddr = "0d"

	// PACVSense4 is the address for Vsense4.
	PACVSense4 PACAddr = "0e"

	// OpenTitanVCC The Open Titan Shield uses Vsense3 to measure VCC power.
	OpenTitanVCC PACAddr = PACVSense3

	// H1VDDIOM The H1 Shield uses Vsense3 to measure VDDIOM power.
	H1VDDIOM PACAddr = PACVSense3
)

// ReadGscTotalMilliAmps measures the current consumed by the GSC across all power rails.
func (h DevboardHelper) ReadGscTotalMilliAmps(ctx context.Context) float32 {
	if h.TestbedType == ti50.GscDTShield {
		return h.readGscTotalMilliAmpsDtShield(ctx)
	} else if h.TestbedType == ti50.GscOTShield {
		return h.readGscTotalMilliAmpsOtShield(ctx)
	} else if h.TestbedType == ti50.GscH1Shield {
		return h.readGscTotalMilliAmpsH1Shield(ctx)
	} else {
		h.Fatalf("current measurement not implemented for: %s", h.TestbedType)
		return 0.0
	}
}

func (h DevboardHelper) readGscTotalMilliAmpsDtShield(ctx context.Context) float32 {
	output, err := h.OpenTitanToolCommand(ctx, "i2c", "--bus", "INA", "raw-write-read", "--hexdata=01", "-n2")
	if err != nil {
		h.Fatalf("i2c error: %s", err)
	}
	reading, err := strconv.ParseUint(output["hexdata"].(string), 16, 64)
	if err != nil {
		h.Fatalf("decoding response '%s' from INA231: %v", output["hexdata"].(string), err)
	}
	return float32(reading) * Ina231ShuntLsbVolts / DauntlessShieldShuntOhms * 1000.0
}

func (h DevboardHelper) readGscTotalMilliAmpsPac195x(ctx context.Context, bus ti50.I2cBusName, addr PACAddr, shuntOhms float32) float32 {
	// Tell the chip to present snapshot of measurement values in its registers.
	_, err := h.OpenTitanToolCommand(ctx, "i2c", "--bus", string(bus), "raw-write", "--hexdata=1f")
	if err != nil {
		h.Fatalf("i2c error: %s", err)
	}
	// Read the specified Vsense address.
	output, err := h.OpenTitanToolCommand(ctx, "i2c", "--bus", string(bus), "raw-write-read", "--hexdata="+string(addr), "-n2")
	if err != nil {
		h.Fatalf("i2c error: %s", err)
	}
	reading, err := strconv.ParseUint(output["hexdata"].(string), 16, 64)
	if err != nil {
		h.Fatalf("decoding response '%s' from INA231: %v", output["hexdata"].(string), err)
	}
	return float32(reading) * Pac195xShuntLsbVolts / shuntOhms * 1000.0
}

func (h DevboardHelper) readGscTotalMilliAmpsOtShield(ctx context.Context) float32 {
	return h.readGscTotalMilliAmpsPac195x(ctx, ti50.I2cPAC1Bus, OpenTitanVCC, OpenTitanShieldShuntOhms)
}

func (h DevboardHelper) readGscTotalMilliAmpsH1Shield(ctx context.Context) float32 {
	return h.readGscTotalMilliAmpsPac195x(ctx, ti50.I2cPAC1Bus, H1VDDIOM, H1ShieldShuntOhms)
}

// CurrentThreshold is the current threshold in milliAmps for different chip power states.
type CurrentThreshold float32

const (
	// H1MaxDeepSleepCurrent is the maximum power allowed for deep sleep.
	H1MaxDeepSleepCurrent CurrentThreshold = 0.2

	// H1MaxNormalSleepCurrent is the maximum power allowed for regular sleep.
	H1MaxNormalSleepCurrent CurrentThreshold = 1.5
)

// WaitForPowerRise waits until the current reading is greater than the given threshold
func (h DevboardHelper) WaitForPowerRise(ctx context.Context, threshold CurrentThreshold, timeout time.Duration) error {
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: timeout}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		c := h.ReadGscTotalMilliAmps(ctx)
		testing.ContextLogf(ctx, "%f: %f mA", threshold, c)
		if c > float32(threshold) {
			return nil
		}
		return errors.Errorf("%f is below %f", c, threshold)
	}, &pOpts)
	if err != nil {
		return errors.Errorf("GSC did not rise over %f: %s", threshold, err)
	}
	return nil
}

// WaitForPowerDrop waits until the current reading is less than the given threshold
func (h DevboardHelper) WaitForPowerDrop(ctx context.Context, threshold CurrentThreshold, timeout time.Duration) error {
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: timeout}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		c := h.ReadGscTotalMilliAmps(ctx)
		testing.ContextLogf(ctx, "%f: %f mA", threshold, c)
		if c < float32(threshold) {
			return nil
		}
		return errors.Errorf("%f is above %f", c, threshold)
	}, &pOpts)
	if err != nil {
		return errors.Errorf("GSC did not drop below %f: %s", threshold, err)
	}
	return nil
}

// WaitUntilNormalSleep waits until gsc goes into deep sleep via monitoring print statement or waiting for the power to drop.
func (h DevboardHelper) WaitUntilNormalSleep(ctx context.Context, i *ti50.CrOSImage, timeout time.Duration) error {
	// H1 does not support checking sleep from the console. Use a power drop to detect sleep.
	if h.TestbedType == ti50.GscH1Shield {
		return h.WaitForPowerDrop(ctx, H1MaxNormalSleepCurrent, timeout)
	}
	return i.WaitUntilNormalSleep(ctx, timeout)
}

// WaitUntilDeepSleep waits until gsc goes into deep sleep via monitoring print statement or waiting for the power to drop.
func (h DevboardHelper) WaitUntilDeepSleep(ctx context.Context, i *ti50.CrOSImage, timeout time.Duration) error {
	// H1 does not support checking deep sleep from the console. Use a power drop to detect deep sleep.
	if h.TestbedType == ti50.GscH1Shield {
		return h.WaitForPowerDrop(ctx, H1MaxDeepSleepCurrent, timeout)
	}
	return i.WaitUntilDeepSleep(ctx, timeout)
}

// WaitUntilAnySleep waits until gsc goes into deep or normal sleep via monitoring print statement or waiting for the power to drop.
func (h DevboardHelper) WaitUntilAnySleep(ctx context.Context, i *ti50.CrOSImage, timeout time.Duration) error {
	// H1 does not support checking sleep from the console. Use a power drop to detect sleep.
	if h.TestbedType == ti50.GscH1Shield {
		return h.WaitForPowerDrop(ctx, H1MaxNormalSleepCurrent, timeout)
	}
	return i.WaitUntilAnySleep(ctx, timeout)
}
