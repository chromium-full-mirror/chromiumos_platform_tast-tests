// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var gpioTimeout = 1 * time.Second
var gpioInterval = 100 * time.Millisecond

var powerButtonInput = ti50.GpioCr50RBOXPwrbLIn
var key0Input = ti50.GpioCr50RBOXKey0In
var key1Input = ti50.GpioCr50RBOXKey1In

// Power Button (All SKUs):
// The power button output follows the power button input. They're both active low.
// Nothing blocks the power button output.
var powerButton = rboxConfig{
	input:             powerButtonInput,
	inputAssertedVal:  false,
	output:            ti50.GpioCr50RBOXPwrbLOut,
	outputAssertedVal: false,
	blocked:           false,
}

// Key0 (Clamshell SKUs):
// The Key0 output follows the Key0 input. They're both active low.
// Key0 output is blocked when the power button is pressed.
var key0Clamshell = rboxConfig{
	input:             key0Input,
	inputAssertedVal:  false,
	output:            ti50.GpioCr50RBOXKey0Out,
	outputAssertedVal: false,
	blocked:           true,
	blockedBy:         powerButton.input,
}

// Key0 (Detachable SKUs):
// The Key0 output follows the Key0 input. They're both active low.
// Key0 output is not blocked.
var key0Detachable = rboxConfig{
	input:             key0Input,
	inputAssertedVal:  false,
	output:            ti50.GpioCr50RBOXKey0Out,
	outputAssertedVal: false,
	blocked:           false,
}

// Key1 (Clamshell SKUs):
// The Key1 input is inverted. It's active low.
// The Key1 output normal.
// Key1 output is blocked when the power button is pressed
var key1Clamshell = rboxConfig{
	input:             key1Input,
	inputAssertedVal:  false,
	output:            ti50.GpioCr50RBOXKey1Out,
	outputAssertedVal: true,
	blocked:           true,
	blockedBy:         powerButton.input,
}

// Key1 (Detachable SKUs):
// The Key0 output follows the Key0 input. They're both active low.
// Key1 input is not inverted.
// Key1 output is not blocked.
var key1Detachable = rboxConfig{
	input:             key1Input,
	inputAssertedVal:  false,
	output:            ti50.GpioCr50RBOXKey1Out,
	outputAssertedVal: false,
	blocked:           false,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Cr50RBOXBufferOutput,
		Desc:    "Verify RBOX handles buffering the rbox inputs",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"mruthven@chromium.org",  // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "power_button",
			Val:  powerButtonInput,
		}, {
			Name: "key0",
			Val:  key0Input,
		}, {
			Name: "key1",
			Val:  key1Input,
		}},
	})
}

type rboxConfig struct {
	name              string
	input             ti50.GpioName
	inputAssertedVal  bool
	output            ti50.GpioName
	outputAssertedVal bool
	blocked           bool
	blockedBy         ti50.GpioName
}

func getRboxConfig(gpio ti50.GpioName, sku ti50.ChipSKU) (rboxConfig, error) {
	switch gpio {
	case powerButton.input:
		return powerButton, nil
	case key0Input:
		if sku == ti50.SKUH1Detachable {
			return key0Detachable, nil
		}
		if sku == ti50.SKUH1Clamshell {
			return key0Clamshell, nil
		}
		return rboxConfig{}, errors.Errorf("Did not find key0 rbox config for %s", sku)
	case key1Input:
		if sku == ti50.SKUH1Detachable {
			return key1Detachable, nil
		}
		if sku == ti50.SKUH1Clamshell {
			return key1Clamshell, nil
		}
		return rboxConfig{}, errors.Errorf("Did not find key1 rbox config for %s", sku)
	default:
		return rboxConfig{}, errors.Errorf("Did not find %s rbox config for %s", gpio, sku)
	}
}

// Cr50RBOXBufferOutput verifies GSC RBOX buffers the outputs of the power
// button, key0, and key1 signals correctly.
func Cr50RBOXBufferOutput(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	testSignal := s.Param().(ti50.GpioName)

	s.Log("(Re)starting GSC")
	b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)

	chipSKU, err := i.ChipSKU(ctx)
	s.Logf("Running %s test for %s", testSignal, chipSKU)
	th.MustSucceed(err, "did not find chip SKU")
	config, err := getRboxConfig(testSignal, chipSKU)
	th.MustSucceed(err, "did not find RBOX config")
	s.Log("config:", config)
	blockedBy, err := getRboxConfig(config.blockedBy, chipSKU)
	if config.blocked {
		th.MustSucceed(err, "did not find RBOX block config")
		s.Log("blockedBy:", blockedBy)
	}

	s.Log("Deasserting Signals")
	// Make sure the signal that blocks buffering is deasserted.
	if config.blocked {
		b.GpioSet(ctx, blockedBy.input, !blockedBy.inputAssertedVal)
	}

	gpioMonitor := b.GpioMonitorStart(ctx, config.output)

	// Set the initial RBOX state to deasserted.
	b.GpioSet(ctx, config.input, !config.inputAssertedVal)

	// Wait to see if the gpio value changes. It may not.
	s.Log("gpio events:", b.GpioMonitorWait(ctx, gpioMonitor, gpioTimeout, gpioInterval))

	inVal := b.GpioGet(ctx, config.input)
	outVal := b.GpioGet(ctx, config.output)
	s.Logf("input signal  - %s - %t", config.input, inVal)
	s.Logf("output signal - %s - %t", config.output, outVal)
	if inVal == config.inputAssertedVal {
		s.Fatalf("Failed to setup %s", config.input)
	}
	if outVal == config.outputAssertedVal {
		s.Fatalf("Failed to setup rbox output. %s not deasserted when %s was deasserted", config.output, config.input)
	}

	// Read from the gpio monitor to clear existing events.
	b.GpioMonitorRead(ctx, gpioMonitor)
	// Verify the RBOX output is asserted after the input is asserted.
	s.Log("Asserting", config.input)
	b.GpioSet(ctx, config.input, config.inputAssertedVal)

	// Wait until the output is asserted
	events := b.GpioMonitorWait(ctx, gpioMonitor, gpioTimeout, gpioInterval)
	if len(events.Sorted) == 0 {
		s.Errorf("Failed to detect %s change when %s was asserted", config.output, config.input)
	}
	s.Log("gpio events:", events.Sorted)

	inVal = b.GpioGet(ctx, config.input)
	outVal = b.GpioGet(ctx, config.output)
	s.Logf("input signal  - %s - %t", config.input, inVal)
	s.Logf("output signal - %s - %t", config.output, outVal)
	if inVal != config.inputAssertedVal {
		s.Fatalf("Failed to assert %s", config.input)
	}
	if outVal != config.outputAssertedVal {
		s.Errorf("RBOX output %s not asserted when %s was asserted", config.output, config.input)
	} else {
		s.Logf("%s was asserted", config.output)
	}

	// Read from the gpio monitor to clear existing events.
	b.GpioMonitorRead(ctx, gpioMonitor)
	// Verify the RBOX output can be deasserted again.
	s.Log("Deasserting", config.input)
	b.GpioSet(ctx, config.input, !config.inputAssertedVal)

	// Wait until the output is asserted
	events = b.GpioMonitorWait(ctx, gpioMonitor, gpioTimeout, gpioInterval)
	if len(events.Sorted) == 0 {
		s.Errorf("Failed to detect %s change", config.output)
	}
	s.Log("gpio events:", events.Sorted)

	inVal = b.GpioGet(ctx, config.input)
	outVal = b.GpioGet(ctx, config.output)
	s.Logf("input signal  - %s - %t", config.input, inVal)
	s.Logf("output signal - %s - %t", config.output, outVal)
	if inVal == config.inputAssertedVal {
		s.Fatalf("Failed to deassert %s", config.input)
	}
	if outVal == config.outputAssertedVal {
		s.Errorf("RBOX output %s not deasserted when %s was deasserted", config.output, config.input)
	} else {
		s.Logf("%s was deasserted", config.output)
		s.Logf("%s output followed the %s input", config.output, config.input)
	}
	if !config.blocked {
		return
	}

	// Read from the gpio monitor to clear existing events.
	b.GpioMonitorRead(ctx, gpioMonitor)
	// Verify the RBOX output is blocked.
	s.Logf("Verifying %s output is blocked by %s", config.output, blockedBy.input)
	b.GpioSet(ctx, blockedBy.input, blockedBy.inputAssertedVal)
	b.GpioSet(ctx, config.input, config.inputAssertedVal)

	// Wait to see if the signal gets asserted.
	events = b.GpioMonitorWait(ctx, gpioMonitor, gpioTimeout, gpioInterval)
	if len(events.Sorted) != 0 {
		s.Errorf("detected %s change when %s was asserted: %v", config.output, blockedBy.input, events.Sorted)
	}
	s.Log("gpio events:", events.Sorted)

	blockVal := b.GpioGet(ctx, blockedBy.input)
	inVal = b.GpioGet(ctx, config.input)
	outVal = b.GpioGet(ctx, config.output)
	s.Logf("blocking signal  - %s - %t", blockedBy.input, blockVal)
	s.Logf("input signal     - %s - %t", config.input, inVal)
	s.Logf("blocked output   - %s - %t", config.output, outVal)
	if blockVal != blockedBy.inputAssertedVal {
		s.Fatalf("Failed to assert %s", blockedBy.input)
	}
	if inVal != config.inputAssertedVal {
		s.Fatalf("Failed to assert %s", config.input)
	}
	if outVal == config.outputAssertedVal {
		s.Errorf("%s did not block %s. %s was asserted", blockedBy.input, config.output, config.output)
	}

	// Read from the gpio monitor to clear existing events.
	b.GpioMonitorRead(ctx, gpioMonitor)
	s.Log("Deasserting", blockedBy.input)
	s.Logf("Verify %s becomes unblocked", config.output)
	b.GpioSet(ctx, blockedBy.input, !blockedBy.inputAssertedVal)
	// Wait to see if the signal gets deasserted.
	events = b.GpioMonitorWait(ctx, gpioMonitor, gpioTimeout, gpioInterval)
	if len(events.Sorted) == 0 {
		s.Errorf("deaserring %s did not unblock %s: %v", blockedBy.input, config.output, events.Sorted)
	}
	s.Log("gpio events:", events.Sorted)

	blockVal = b.GpioGet(ctx, blockedBy.input)
	inVal = b.GpioGet(ctx, config.input)
	outVal = b.GpioGet(ctx, config.output)
	s.Logf("blocking signal  - %s - %t", blockedBy.input, blockVal)
	s.Logf("input signal     - %s - %t", config.input, inVal)
	s.Logf("blocked output   - %s - %t", config.output, outVal)
	if blockVal == blockedBy.inputAssertedVal {
		s.Fatalf("Failed to deassert %s", blockedBy.input)
	}
	if inVal != config.inputAssertedVal {
		s.Fatalf("The input signal %s was deasserted after deaserting %s", config.input, blockedBy.input)
	}
	if outVal != config.outputAssertedVal {
		s.Errorf("%s was not asserted after deasserting %s", config.output, blockedBy.input)
	} else {
		s.Logf("%s blocked the %s output", blockedBy.input, config.output)
	}
}
