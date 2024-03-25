// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PowerButtonHelper is a helper interface that can press the power button
// on the device.
type PowerButtonHelper interface {
	PressAndRelease(ctx context.Context) error
}

// TimerHelper is a helper interface that exposes time-related utilities.
type TimerHelper interface {
	// After calling this function, |duration| should have passed in the hwsec timer.
	Sleep(ctx context.Context, duration time.Duration) error
}

// SimulatorController includes helpers to control the TPM simulator.
type SimulatorController interface {
	PowerButtonHelper() *PowerButtonHelper
	TimerHelper() *TimerHelper
}

// Ti50EmulatorController implements SimulatorController for ti50-emulator.
// This is currently only available on VMs running ti50-emulator (dependencies
// "tpm2-simulator" + "gsc").
type Ti50EmulatorController struct {
	powerButtonHelper SimulatedPowerButtonHelper
	timerHelper       SimulatedTimerHelper
}

// SimulatedPowerButtonHelper presses the power button by sending bytes to
// the GPIO power button socket.
type SimulatedPowerButtonHelper struct {
	cmd CmdRunner
}

// SimulatedTimerHelper controls the time by sending data to the timer_control
// socket.
type SimulatedTimerHelper struct {
	cmd CmdRunner
}

// NewTi50EmulatorController creates a new Ti50EmulatorController.
func NewTi50EmulatorController(cmd CmdRunner) Ti50EmulatorController {
	return Ti50EmulatorController{
		powerButtonHelper: SimulatedPowerButtonHelper{cmd},
		timerHelper:       SimulatedTimerHelper{cmd},
	}
}

// PowerButtonHelper exposes the power button helper of Ti50EmulatorController.
func (c *Ti50EmulatorController) PowerButtonHelper() *SimulatedPowerButtonHelper {
	return &c.powerButtonHelper
}

// TimerHelper exposes the timer button helper of Ti50EmulatorController.
func (c *Ti50EmulatorController) TimerHelper() *SimulatedTimerHelper {
	return &c.timerHelper
}

// PressAndRelease implements PowerButtonHelper.PressAndRelease.
func (helper SimulatedPowerButtonHelper) PressAndRelease(ctx context.Context) error {
	const (
		socketCommandTempl string = "echo -e %s | socat -t1 unix-connect:/run/tpm2-simulator/sockets/gpioPwrBtn -"
		zero               string = "0"
		one                string = "1"
	)

	// Sending the character zero to the socket triggers a power button pressed
	// signal, while sending the character one triggers a power button released
	// signal.
	if _, err := helper.cmd.Run(ctx, "sh", "-c", fmt.Sprintf(socketCommandTempl, one)); err != nil {
		return errors.Wrap(err, "failed to press power button")
	}
	// GoBigSleepLint: We want to simulate a real button press, so sleep for a short interval.
	testing.Sleep(ctx, 500*time.Millisecond)
	if _, err := helper.cmd.Run(ctx, "sh", "-c", fmt.Sprintf(socketCommandTempl, zero)); err != nil {
		return errors.Wrap(err, "failed to release power button")
	}
	return nil
}

// Sleep implements TimerHelper.Sleep. Instead of actually sleeping, it sends
// control signal to the emulator to fast forward the timer.
func (helper SimulatedTimerHelper) Sleep(ctx context.Context, duration time.Duration) error {
	const (
		socketCommandTempl string = "echo '%s' | socat -t1 unix-connect:/run/tpm2-simulator/sockets/timer -"
		getTime            string = `{"commands":[{"target":"TimerRTC","command":"GetTime"}]}`
		setTimeTo          string = `{"commands":[{"target":"TimerRTC","command":{"SetTime":%d}}]}`
	)

	// Sending the getTime command to get the current ticks (in ms).
	reply, err := helper.cmd.Run(ctx, "sh", "-c", fmt.Sprintf(socketCommandTempl, getTime))
	// TODO(b/331120135): The ti50-emulator currently has some problems with
	// gracefully shutting down the socket connection. Ignore the errors and
	// continue as long as we receive some parseable reply.
	if err != nil {
		testing.ContextLog(ctx, "GetTime command failed: ", err)
	}
	timerMs, err := strconv.Atoi(string(reply))
	if err != nil {
		return errors.Wrap(err, "failed to parse time value")
	}
	setTime := fmt.Sprintf(setTimeTo, timerMs+int(duration.Milliseconds()))
	_, err = helper.cmd.Run(ctx, "sh", "-c", fmt.Sprintf(socketCommandTempl, setTime))
	if err != nil {
		testing.ContextLog(ctx, "SetTime command failed: ", err)
	}
	return nil
}
