// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Provides keyboard mappings.

package firmware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// Important logical keys.
const (
	LogicalKeyBrightnessDown = "KEY_BRIGHTNESSDOWN"
	LogicalKeyBrightnessUp   = "KEY_BRIGHTNESSUP"
	LogicalKeyF5             = "KEY_F5"
	LogicalKeyF6             = "KEY_F6"
	LogicalKeyF7             = "KEY_F7"
)

// GetKeyboardMappings returns some keyboard mappings for the current DUT.
// The returned map is logical key -> servo-key, for example "KEY_BRIGHTNESSDOWN" -> "<f5>"
// and only includes important keys that are used in tests, not every key.
// reverseMappingExceptions are for the keys that don't return the expected key codes in getevent.
func GetKeyboardMappings(ctx context.Context, dut *dut.DUT, model string, hwFeatures *api.HardwareFeatures) (forwardMapping, reverseMappingExceptions map[string]string, err error) {
	reverseMappingExceptions = map[string]string{}
	forwardMapping = map[string]string{
		"KEY_0":        "0",
		"KEY_B":        "b",
		"KEY_E":        "e",
		"KEY_O":        "o",
		"KEY_R":        "r",
		"KEY_S":        "s",
		"KEY_T":        "t",
		"KEY_ENTER":    "<enter>",
		"KEY_LEFTCTRL": "<ctrl_l>",
		"KEY_LEFTALT":  "<alt_l>",
		"KEY_ESC":      "<esc>",
		"KEY_TAB":      "<tab>",
		"KEY_SPACE":    " ",
	}

	// There are 3 kinds of DUTs.
	// 1) Vivaldi -   The EC sends key mappings, and the OS sees logical keys like KEY_BRIGHTNESSDOWN
	//                and ectool kbgetconfig can tell you what those logical key mappings are.
	// 2) Key Codes - The EC sends key mappings, and the OS sees logical keys like KEY_BRIGHTNESSDOWN
	//                but there is no key map to get at runtime. These are the special cases below.
	// 3) Legacy -    The EC sends raw keys to the OS. So F5 -> KEY_F5 and not KEY_BRIGHTNESSDOWN.

	// (1) Run ectool to get vivaldi keys (requires OS >= R123-15775.0.0)
	out, err := dut.Conn().CommandContext(ctx, "ectool", "kbgetconfig").CombinedOutput()
	if err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		re := regexp.MustCompile(`^\s*(\d+): (\S[^\(]*) \(`)
		for sc.Scan() {
			m := re.FindStringSubmatch(sc.Text())
			if m != nil {
				idx, err := strconv.Atoi(m[1])
				if err != nil {
					return nil, nil, errors.Wrapf(err, "failed to parse %q in %q", m[1], sc.Text())
				}
				key := fmt.Sprintf("<f%d>", idx+1)
				// From src/third_party/coreboot/src/acpi/acpigen_ps2_keybd.c
				switch m[2] {
				case "Brightness Down":
					forwardMapping[LogicalKeyBrightnessDown] = key
				case "Brightness Up":
					forwardMapping[LogicalKeyBrightnessUp] = key
				}
			}
		}
		return forwardMapping, reverseMappingExceptions, nil
	}
	// Host command not implemented: EC result 1 (INVALID_COMMAND)
	// Vivaldi keyboard not enabled: EC result 2 (ERROR)
	if !strings.Contains(string(out), "EC result 1 (INVALID_COMMAND)") && !strings.Contains(string(out), "EC result 2 (ERROR)") {
		return nil, nil, errors.Wrapf(err, "ectool kbgetconfig failed: %s", string(out))
	}

	// (2) Non-vivaldi devices that have key mappings
	// 15194 corsola/steelix:    F5=KEY_BRIGHTNESSDOWN F6=KEY_BRIGHTNESSUP F7=KEY_MICMUTE
	if model == "steelix" || model == "rusty" {
		forwardMapping[LogicalKeyBrightnessDown] = "<f5>"
		forwardMapping[LogicalKeyBrightnessUp] = "<f6>"
		return forwardMapping, reverseMappingExceptions, nil
	}
	if model == "hayato" {
		forwardMapping[LogicalKeyBrightnessDown] = "<f6>"
		forwardMapping[LogicalKeyBrightnessUp] = "<f7>"
		return forwardMapping, reverseMappingExceptions, nil
	}
	// These all have normal key mappings
	// 14454 cherry/tomato:      F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
	// 13885 asurada/spherion:   F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
	// 13577 trogdor/pazquel360: F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
	if hwFeatures.FwConfig.FwRoVersion.MajorVersion >= 13885 || model == "pompom" || model == "kingoftown" || model == "pazquel" || model == "pazquel360" {
		forwardMapping[LogicalKeyBrightnessDown] = "<f6>"
		forwardMapping[LogicalKeyBrightnessUp] = "<f7>"
		return forwardMapping, reverseMappingExceptions, nil
	}

	// (3) Legacy devices that use KEY_Fn key mappings.
	if model == "atlas" || model == "eve" {
		forwardMapping[LogicalKeyBrightnessDown] = "<f5>"
		forwardMapping[LogicalKeyBrightnessUp] = "<f6>"
		reverseMappingExceptions["<f5>"] = LogicalKeyF5
		reverseMappingExceptions["<f6>"] = LogicalKeyF6
		return forwardMapping, reverseMappingExceptions, nil
	}
	// All other legacy devices
	// 13577 trogdor/lazor
	forwardMapping[LogicalKeyBrightnessDown] = "<f6>"
	forwardMapping[LogicalKeyBrightnessUp] = "<f7>"
	reverseMappingExceptions["<f6>"] = LogicalKeyF6
	reverseMappingExceptions["<f7>"] = LogicalKeyF7
	return forwardMapping, reverseMappingExceptions, nil
}
