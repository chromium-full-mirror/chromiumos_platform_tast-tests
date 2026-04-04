// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

const (
	procDevices = "/proc/bus/input/devices" // file describing input devices
	sysfsDir    = "/sys"                    // base sysfs directory
	deviceDir   = "/dev/input"              // directory containing event devices

	evGroup     = "EV"   // event type group in devInfo.bits
	keyGroup    = "KEY"  // keyboard event code group in devInfo.bits
	switchGroup = "SW"   // switch event code group in devInfo.bits
	absGroup    = "ABS"  // absolute type group in devInfo.bits
	propGroup   = "PROP" // property type group in devInfo.bits
)

// These match lines in /proc/bus/input/devices. See readDevices for details.
var infoRegexp = regexp.MustCompile(`^I: Bus=([0-9a-f]{4}) Vendor=([0-9a-f]{4}) Product=([0-9a-f]{4}) Version=([0-9a-f]{4})$`)
var nameRegexp = regexp.MustCompile(`^N: Name="(.+)"$`)
var physRegexp = regexp.MustCompile(`^P: Phys=(.+)$`)
var sysfsRegexp = regexp.MustCompile(`^S: Sysfs=(.+)$`)
var bitsRegexp = regexp.MustCompile(`^B: ([A-Z]+)=([0-9a-f ]+)$`)

// devInfo contains information about a device.
type devInfo struct {
	name  string // descriptive name, e.g. "AT Translated Set 2 keyboard"
	phys  string // physical path, e.g. "isa0060/serio0/input0"
	path  string // path to event device, e.g. "/dev/input/event3"
	sysfs string // full /sys/devices path, e.g. "/devices/platform/AMDI0010:00/i2c-0/i2c-GDIX0000:00/0018:27C6:0ED1.0001/input/input11"

	bits map[string]*big.Int // bitfields keyed by group name, e.g. "EV" or "KEY"

	devID
}

// devID contains information identifying a device.
// Do not change the type or order of fields, as this is used in various kernel structs.
type devID struct{ bustype, vendor, product, version uint16 }

func newDevInfo() *devInfo {
	return &devInfo{bits: make(map[string]*big.Int)}
}

func (di *devInfo) String() string {
	return fmt.Sprintf("%s (%q %04x:%04x:%04x:%04x)",
		di.path, di.name, di.bustype, di.vendor, di.product, di.version)
}

// isAudioJack returns true if this appears to be an audio jack device.
func (di *devInfo) isAudioJack() bool {
	// Check whether the device keys match headphone insert or microphone insert.
	return di.path != "" && ((di.hasBit(evGroup, uint16(EV_SW)) &&
		di.hasBit(switchGroup, uint16(SW_HEADPHONE_INSERT))) || (di.hasBit(evGroup, uint16(EV_SW)) &&
		di.hasBit(switchGroup, uint16(SW_MICROPHONE_INSERT))))
}

// isPowerKey returns true if this appears to be a power key device.
func (di *devInfo) isPowerKey() bool {
	return di.path != "" &&
		(di.hasBit(evGroup, uint16(EV_KEY)) && di.hasBit(keyGroup, uint16(KEY_POWER)))
}

// isKeyboard returns true if this appears to be a keyboard device.
func (di *devInfo) isKeyboard() bool {
	// Just check some arbitrary keys. The choice of Escape, Back, VolumeUP and
	// the Mute Key.
	// https://chromeos.google.com/partner/dlm/docs/hardware-specs/keyboardspec.html

	return di.path != "" && ((di.hasBit(evGroup, uint16(EV_KEY)) &&
		di.hasBit(keyGroup, uint16(KEY_ESC)) && di.hasBit(keyGroup, uint16(KEY_BACK)) &&
		di.hasBit(keyGroup, uint16(KEY_VOLUMEUP)) && di.hasBit(keyGroup, uint16(KEY_MUTE))) ||
		(di.hasBit(keyGroup, uint16(KEY_SPACE)) && di.hasBit(keyGroup, uint16(KEY_ENTER)) &&
			di.hasBit(keyGroup, uint16(KEY_F10)) && di.hasBit(keyGroup, uint16(KEY_H))))
}

// isTouchscreen returns true if this appears to be a touchscreen device.
func (di *devInfo) isTouchscreen() bool {
	// Touchscreen reports values in absolute coordinates, and should have the BTN_TOUCH bit set.
	// Multitouch (bit ABS_MT_SLOT) is required to differentiate itself from some stylus devices.
	// Some touchpad devices (like in Kevin) implement all the features needed for a touchscreen
	// device, and luckily more. So, to differentiate a touchpad from a touchscreen, we filter out
	// devices that implements features like DOUBLETAP, which should not be present on a touchscreen.
	return di.path != "" &&
		di.hasBit(evGroup, uint16(EV_KEY)) &&
		di.hasBit(evGroup, uint16(EV_ABS)) &&
		di.hasBit(keyGroup, uint16(BTN_TOUCH)) &&
		!di.hasBit(keyGroup, uint16(BTN_TOOL_DOUBLETAP)) &&
		di.hasBit(absGroup, uint16(ABS_MT_SLOT))
}

// isTrackpad returns true if this appears to be a trackpad device.
func (di *devInfo) isTrackpad() bool {
	// Taken from HasTouchpad() in ui/events/ozone/evdev/event_device_info.cc
	return di.path != "" &&
		di.hasBit(absGroup, uint16(ABS_X)) &&
		di.hasBit(absGroup, uint16(ABS_Y)) &&
		di.hasBit(propGroup, uint16(INPUT_PROP_POINTER)) &&
		!di.hasBit(evGroup, uint16(BTN_TOOL_PEN)) &&
		!di.hasBit(evGroup, uint16(BTN_STYLUS)) &&
		!di.hasBit(evGroup, uint16(BTN_STYLUS2))
}

// isHapticTrackpad returns true if this appears to be a haptic trackpad device.
func (di *devInfo) isHapticTrackpad() bool {
	return di.isTrackpad() && di.hasBit(propGroup, uint16(INPUT_PROP_HAPTIC_TOUCHPAD))
}

// isStylus if this appears to be a stylus device.
func (di *devInfo) isStylus() bool {
	// Taken from HasStylus in ui/events/ozone/evdev/event_device_info.cc
	return di.path != "" &&
		(di.hasBit(keyGroup, uint16(BTN_TOOL_PEN)) ||
			di.hasBit(keyGroup, uint16(BTN_STYLUS)) ||
			di.hasBit(keyGroup, uint16(BTN_STYLUS2)))
}

// hasBit returns true if the n-th bit in di.bits is set.
func (di *devInfo) hasBit(grp string, n uint16) bool {
	bits, ok := di.bits[grp]
	return ok && bits.Bit(int(n)) != 0
}

// parseLine parses a single line from a devices file and incorporates it into di.
// See readDevices for information about the expected format.
func (di *devInfo) parseLine(line, root string) error {
	if ms := infoRegexp.FindStringSubmatch(line); ms != nil {
		id := func(s string) uint16 {
			n, _ := strconv.ParseUint(s, 16, 16)
			return uint16(n)
		}
		di.bustype, di.vendor, di.product, di.version = id(ms[1]), id(ms[2]), id(ms[3]), id(ms[4])
	} else if ms = nameRegexp.FindStringSubmatch(line); ms != nil {
		di.name = ms[1]
	} else if ms = physRegexp.FindStringSubmatch(line); ms != nil {
		di.phys = ms[1]
	} else if ms = sysfsRegexp.FindStringSubmatch(line); ms != nil {
		var err error
		dir := filepath.Join(sysfsDir, ms[1])
		if di.path, err = getDevicePath(dir, root); err != nil {
			return errors.Wrapf(err, "didn't find device in %v", dir)
		}
		di.sysfs = ms[1]
	} else if ms = bitsRegexp.FindStringSubmatch(line); ms != nil {
		var str string
		// Bitfields are specified as space-separated 32- or 64-bit hex values
		// (depending on the userspace arch). Zero-pad if necessary.
		ptrSize := 32 << uintptr(^uintptr(0)>>63) // from https://stackoverflow.com/questions/25741841/
		fullLen := ptrSize / 4
		for _, p := range strings.Fields(ms[2]) {
			if len(p) < fullLen {
				p = strings.Repeat("0", fullLen-len(p)) + p
			}
			str += p
		}
		bits, ok := big.NewInt(0).SetString(str, 16)
		if !ok {
			return errors.Errorf("failed to parse bitfield %q", str)
		}
		di.bits[ms[1]] = bits
	}
	return nil
}

// readDevices reads /proc/bus/input/devices and returns device information.
// Unit tests may specify an alternate root directory via root.
//
// The file should contain stanzas similar to the following, separated by blank lines:
//
//	I: Bus=0011 Vendor=0001 Product=0001 Version=ab83
//	N: Name="AT Translated Set 2 keyboard"
//	P: Phys=isa0060/serio0/input0
//	S: Sysfs=/devices/platform/i8042/serio0/input/input3
//	U: Uniq=
//	H: Handlers=sysrq event3
//	B: PROP=0
//	B: EV=120013
//	B: KEY=402000000 3803078f800d001 feffffdfffefffff fffffffffffffffe
//	B: MSC=10
//	B: LED=7
//
// "B" entries are hexadecimal bitfields. For example, in the "EV" bitfield, the i-th bit corresponds to the EventType with value i.
func readDevices(root string) (infos []*devInfo, err error) {
	f, err := os.Open(filepath.Join(root, procDevices))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	inDev := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		// End the current device when we see a blank line.
		if len(line) == 0 {
			inDev = false
			continue
		}

		if !inDev {
			infos = append(infos, newDevInfo())
			inDev = true
		}
		if err := infos[len(infos)-1].parseLine(line, root); err != nil {
			return nil, errors.Wrapf(err, "failed parsing %q from %v", line, procDevices)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, errors.Wrapf(err, "failed reading %v", procDevices)
	}
	return infos, nil
}

// DevInfo is a public version of devInfo to allow other packages to also look
// up what input devices are available. The Name field contains the device's
// name, and the Path field contains the filesystem path to the device that can
// be opened and read from.
type DevInfo struct {
	Name string
	Path string
}

// ReadInputDevices is a public version of readDevices to allow other packages
// to get a list of all the input devices and some basic information for each.
func ReadInputDevices(root string) ([]DevInfo, error) {
	devInfos, err := readDevices(root)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't readDevices")
	}
	var infos []DevInfo
	for _, di := range devInfos {
		infos = append(infos, DevInfo{Name: di.name, Path: di.path})
	}
	return infos, nil
}

// getDevicePath iterates over the entries in sysdir, a sysfs device dir (e.g.
// "/sys/devices/platform/i8042/serio1/input/input3"), looking for a event dir (e.g. "event3"), and returns the
// corresponding device in /dev/input (e.g. "/dev/input/event3").
// Unit tests may specify an alternate root directory via root.
func getDevicePath(sysdir, root string) (string, error) {
	fis, err := os.ReadDir(filepath.Join(root, sysdir))
	if err != nil {
		return "", err
	}
	for _, fi := range fis {
		if !strings.HasPrefix(fi.Name(), "event") || !fi.Type().IsDir() {
			continue
		}
		dev := filepath.Join(deviceDir, fi.Name())
		if _, err := os.Stat(filepath.Join(root, dev)); err == nil {
			return dev, nil
		}
	}
	return "", errors.Errorf("no event dirs in %v", sysdir)
}
