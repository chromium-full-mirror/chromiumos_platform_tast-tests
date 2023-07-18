// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"unsafe"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// TopRowLayout represents the top row layout of the Chromebook keyboard.
// Each entry represents the key it is mapped to. An empty string means that is is not mapped.
type TopRowLayout struct {
	BrowserBack    string
	BrowserForward string
	BrowserRefresh string
	ZoomToggle     string
	SelectTask     string
	Screenshot     string
	BrightnessDown string
	BrightnessUp   string
	MediaPlayPause string
	VolumeMute     string
	VolumeDown     string
	VolumeUp       string
	MediaLaunchApp string
}

// TopRowLayoutType represents the top row keyboard type.
type TopRowLayoutType int

// Following constants represent different top row keyboard layouts taken from here:
// https://source.chromium.org/chromium/chromium/src/+/main:ui/chromeos/events/event_rewriter_chromeos.h;l=54;drc=f54415214d7d3ed03d2e9a1382d633abb3a44f47
const (
	// LayoutUnknown must be 0 so KeyboardEventWriter is initialized correctly
	LayoutUnknown TopRowLayoutType = iota
	Layout1
	Layout2
	LayoutWilco
	LayoutDrallion
	LayoutCustom
)

// KeyboardTopRowLayout returns the layout of the top row (function keys) for a given keyboard.
// This is because not all Chromebook keyboards have the same functionality associated to the functions keys.
// As an example, the Toggle Zoom key could be mapped to F3 or F4 depending on the Chromebook model.
// NOTE: If the given keyboard is generated through `input.Keyboard` and can be either an internal or external keyboard,
// this function can yield inaccurate results. `input.VirtualKeyboard` is guaranteed to always give the correct result from
// this KeyboardTopRowLayout. `input.Keyboard` will only always give the correct result from this function if the `InternalKeyboard`
// hardware dependency is applied to the test.
// TODO(b/286466129): Improve internal vs external detection of keyboards to better deduce the default.
func KeyboardTopRowLayout(ctx context.Context, ew *KeyboardEventWriter) (*TopRowLayout, error) {
	// "mapping 1" and "mapping 2" taken from:
	// https://cs.chromium.org/chromium/src/ui/chromeos/events/event_rewriter_chromeos.cc?l=1143&rcl=3028a8be77afd57282d664b6bb07f6d4d01edc55
	mapping1 := TopRowLayout{
		BrowserBack:    "F1",
		BrowserForward: "F2",
		BrowserRefresh: "F3",
		ZoomToggle:     "F4",
		SelectTask:     "F5",
		BrightnessDown: "F6",
		BrightnessUp:   "F7",
		VolumeMute:     "F8",
		VolumeDown:     "F9",
		VolumeUp:       "F10",
	}

	// External keyboards receive top row layouts equivalent to mapping1, but with search prepended.
	externalMapping1 := TopRowLayout{
		BrowserBack:    "Search+F1",
		BrowserForward: "Search+F2",
		BrowserRefresh: "Search+F3",
		ZoomToggle:     "Search+F4",
		SelectTask:     "Search+F5",
		BrightnessDown: "Search+F6",
		BrightnessUp:   "Search+F7",
		VolumeMute:     "Search+F8",
		VolumeDown:     "Search+F9",
		VolumeUp:       "Search+F10",
	}

	// BROWSER_FORWARD removed, MEDIA_PLAY_PAUSE added.
	mapping2 := TopRowLayout{
		BrowserBack:    "F1",
		BrowserRefresh: "F2",
		ZoomToggle:     "F3",
		SelectTask:     "F4",
		BrightnessDown: "F5",
		BrightnessUp:   "F6",
		MediaPlayPause: "F7",
		VolumeMute:     "F8",
		VolumeDown:     "F9",
		VolumeUp:       "F10",
	}

	// Wilco mappings taken from:
	// https://source.chromium.org/chromium/chromium/src/+/HEAD:ui/chromeos/events/event_rewriter_chromeos.cc;drc=3e2b7d89ce6261e00e6e723c13c52d0d41bcc69e;l=1599
	// MEDIA_PLAY_PAUSE removed, MEDIA_LAUNCH_APP2 added.
	mappingWilcoAndDrallion := TopRowLayout{
		BrowserBack:    "search+F1",
		BrowserRefresh: "search+F2",
		ZoomToggle:     "search+F3",
		SelectTask:     "search+F4",
		BrightnessDown: "search+F5",
		BrightnessUp:   "search+F6",
		VolumeMute:     "search+F7",
		VolumeDown:     "search+F8",
		VolumeUp:       "search+F9",
		MediaLaunchApp: "search+F12",
	}

	// This is the minimal set of required top row keys for custom top row
	// layouts.
	mappingCustom := TopRowLayout{
		BrowserBack:    "back",
		BrowserRefresh: "refresh",
		ZoomToggle:     "fullscreen",
		SelectTask:     "scale",
		Screenshot:     "sysrq",
		BrightnessDown: "brightnessdown",
		BrightnessUp:   "brightnessup",
		VolumeMute:     "mute",
		VolumeDown:     "volumedown",
		VolumeUp:       "volumeup",
	}

	layout, err := GetTopRowLayoutType(ctx, ew)
	if err != nil {
		return nil, err
	}

	switch layout {
	case Layout1:
		// If its a virtual device, it will be considered an external keyboard.
		if ew.virt != nil {
			return &externalMapping1, nil
		}
		return &mapping1, nil
	case Layout2:
		return &mapping2, nil
	case LayoutWilco:
		return &mappingWilcoAndDrallion, nil
	case LayoutDrallion:
		return &mappingWilcoAndDrallion, nil
	case LayoutCustom:
		return &mappingCustom, nil
	// If for some reason our layout does not match any of our known layouts, return mapping for Layout1.
	default:
		return &mapping1, nil
	}
}

// GetTopRowLayoutType returns the TopRowLayoutType for a given KeyboardEventWriter.
// If the type had been discovered before, this function uses the cached value within the KeyboardEventWriter.
func GetTopRowLayoutType(ctx context.Context, ew *KeyboardEventWriter) (TopRowLayoutType, error) {
	getTopRowLayoutTypeHelper := func(ctx context.Context, ew *KeyboardEventWriter) (TopRowLayoutType, error) {
		props, err := udevProperties(ctx, ew.Device())
		if err != nil {
			return Layout1, err
		}
		attrs, err := udevAttributes(ctx, ew.Device())
		if err != nil {
			return Layout1, err
		}

		// Logic taken from here:
		// https://source.chromium.org/chromium/chromium/src/+/HEAD:ui/chromeos/events/event_rewriter_chromeos.h;l=56;drc=3e2b7d89ce6261e00e6e723c13c52d0d41bcc69e
		if val, ok := attrs["function_row_physmap"]; ok {
			if err = parseCustomScanCodes(ctx, ew, val); err != nil {
				return Layout1, err
			}

			return LayoutCustom, nil
		}
		if val, ok := props["CROS_KEYBOARD_TOP_ROW_LAYOUT"]; ok {
			switch val {
			case "1":
				return Layout1, nil
			case "2":
				return Layout2, nil
			case "3":
				return LayoutWilco, nil
			case "4":
				return LayoutDrallion, nil
			default:
				return Layout1, errors.Errorf("unexpected CROS_KEYBOARD_ROW_LAYOUT: got %s, want [1-4]", val)
			}
		}
		// If keyboard cannot be identified, return Layout1 as defined here:
		// https://source.chromium.org/chromium/chromium/src/+/HEAD:ui/chromeos/events/event_rewriter_chromeos.h;l=172;drc=c537d05a0cc7b74258fe1474260094923b1e4f68
		return Layout1, nil
	}

	if ew.topRowLayoutType != LayoutUnknown {
		return ew.topRowLayoutType, nil
	}

	layout, err := getTopRowLayoutTypeHelper(ctx, ew)
	ew.topRowLayoutType = layout
	return layout, err
}

// udevAttributes returns the attributes associated to a certain Linux udev device.
func udevAttributes(ctx context.Context, devicePath string) (map[string]string, error) {
	cmd := testexec.CommandContext(ctx, "udevadm", "info", "--attribute-walk", "property", "--name", devicePath)
	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return nil, errors.Errorf("failed running %q", strings.Join(cmd.Args, " "))
	}
	return parseUdev(out, `^ATTRS{([a-z0-9_]+)}=="(.*)"$`)
}

// udevProperties returns the properties associated to a certain Linux udev device.
func udevProperties(ctx context.Context, devicePath string) (map[string]string, error) {
	cmd := testexec.CommandContext(ctx, "udevadm", "info", "--query", "property", "--name", devicePath)
	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return nil, errors.Errorf("failed running %q", strings.Join(cmd.Args, " "))
	}
	return parseUdev(out, `^([A-Z0-9_]+)\s*=\s*(.*)$`)
}

// parseUdev parses the raw output from udevadm and converts it into a key-value map.
func parseUdev(r []byte, matchPattern string) (map[string]string, error) {
	// lineRe matches a key-value line.
	var lineRe = regexp.MustCompile(matchPattern)

	kvs := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(r))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kvs[m[1]] = m[2]
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return kvs, nil
}

// evIOCGKeyCodeV2 is equivalent to EVIOCGKEYCODE_V2 defined in include/uapi/linux/input.h.
func evIOCGKeyCodeV2() uint {
	const sizeofInputKeymapEntry = 0x28
	return ior('E', 0x04, sizeofInputKeymapEntry)
}

// inputKeyMapEntry is equivalent to input_keymap_entry in include/uapi/linux/input.h.
type inputKeyMapEntry struct {
	flags    uint8
	len      uint8
	index    uint16
	keycode  uint32
	scancode [32]byte
}

// parseCustomScanCodes converts the topRowScanCodesStr which is a string of hex values to the equivalent linux
// keycodes for each scancode and stores them in a map within the KeyboardEventWriter.
func parseCustomScanCodes(ctx context.Context, ew *KeyboardEventWriter, topRowScanCodesStr string) error {
	scanCodeStrs := strings.Split(topRowScanCodesStr, " ")

	f, err := os.Open(ew.Device())
	if err != nil {
		return err
	}
	defer f.Close()

	ew.topRowScanCodeMap = make(map[EventCode]int32)
	for _, scanCodeStr := range scanCodeStrs {
		// Decode each scancode and convert it to a uint32.
		scanCodeArray, err := hex.DecodeString(scanCodeStr)
		if err != nil {
			return err
		}
		var scanCodeUint32Array [unsafe.Sizeof(uint32(0))]byte
		copy(scanCodeUint32Array[:], scanCodeArray)
		scanCode := kernelByteOrder.Uint32(scanCodeUint32Array[:])

		// Initialize inputKeyMapEntry struct with the scancode value and length.
		inputKeymapEntry := inputKeyMapEntry{}
		inputKeymapEntry.len = uint8(len(scanCodeArray))
		copy(inputKeymapEntry.scancode[:], scanCodeArray)

		// Make EVIOCGKEYCODE_V2 ioctl call to retrieve the equivalent keycode for the given scancode.
		if err := ioctl(int(f.Fd()), evIOCGKeyCodeV2(), uintptr(unsafe.Pointer(&inputKeymapEntry))); err != nil {
			return err
		}

		ew.topRowScanCodeMap[EventCode(inputKeymapEntry.keycode)] = int32(scanCode)
	}

	return nil
}
