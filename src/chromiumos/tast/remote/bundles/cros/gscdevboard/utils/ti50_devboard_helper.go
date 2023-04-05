// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"chromiumos/tast/common/firmware/ti50"
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
}

// NewDevboardHelper creates a new object from a DevBoard and testing state
func NewDevboardHelper(board ti50.DevBoard, state FirmwareTestingHelperDelegate) DevboardHelper {
	return DevboardHelper{board, state}
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

// GpioApplyStrap applies a known gpio strap setting
func (h DevboardHelper) GpioApplyStrap(ctx context.Context, strap ti50.GpioStrap) {
	if _, err := h.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
		h.Fatalf("failed to apply gpio strap %s: %s", strap, err)
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

// GpioMonitorFinish finishes gpio monitoring for the specified session
func (h DevboardHelper) GpioMonitorFinish(ctx context.Context, session GpioMonitorSession) (events GpioEvents) {
	args := make([]string, 2)
	args[0] = "monitoring"
	args[1] = "read"
	for k := range session.InitialValues {
		args = append(args, string(k))
	}
	readOutput, err := h.PlainCommand(ctx, "gpio", args...)
	if err != nil {
		h.Fatalf("failed to stop gpio monitoring: %s. %s", args, err)
	}

	output := monitorFinishOutput{}
	if err := json.Unmarshal(readOutput, &output); err != nil {
		h.Fatalf("failed to parse stop gpio monitoring output: %s. %s", string(readOutput), err)
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

// TpmReadRegister retrieves the value of a TPM register by communicating via SPI or I2C.
func (h DevboardHelper) TpmReadRegister(ctx context.Context, bus ti50.TpmBus, register ti50.TpmRegister) string {
	data, err := h.OpenTitanToolCommand(ctx,
		string(bus), "tpm", "read-register", string(register))
	if err != nil {
		h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return data["hexdata"].(string)
}

// TpmExecuteHex sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply.
func (h DevboardHelper) TpmExecuteHex(ctx context.Context, bus ti50.TpmBus, request string) string {
	response, err := h.OpenTitanToolCommand(ctx,
		string(bus), "tpm", "execute-command", "--hexdata", request)
	if err != nil {
		h.Fatalf("failed to execute TPM command: %s", err)
	}
	return response["hexdata"].(string)
}

// TpmExecute sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply.
func (h DevboardHelper) TpmExecute(ctx context.Context, bus ti50.TpmBus, request []byte) []byte {
	response, err := h.OpenTitanToolCommand(ctx,
		string(bus), "tpm", "execute-command", "--hexdata", hex.EncodeToString(request))
	if err != nil {
		h.Fatalf("failed to execute TPM command: %s", err)
	}
	binary, err := hex.DecodeString(response["hexdata"].(string))
	if err != nil {
		h.Fatalf("malformed hexdata \"%s\": %s", response["hexdata"].(string), err)
	}
	return binary
}
