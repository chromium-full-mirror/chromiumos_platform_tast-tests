// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pmpb "chromiumos/system_api/power_manager_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TransitionToTabletMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tablet mode: Transition to tablet mode",
		Contacts:     []string{"intel-chrome-system-automation-team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"volumeUp.txt", "volumeDown.txt"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Touchpad(), hwdep.Keyboard(), hwdep.FormFactor(hwdep.Convertible)),
		Attr:         []string{"group:intel-convertible"},
		Timeout:      5 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func TransitionToTabletMode(ctx context.Context, s *testing.State) {
	// TransitionToTabletMode function verifies Touch navigation, On-board keyboard, External peripherals, Power, volume
	// button works or not when dut is transitioned from laptop mode.
	cr := s.FixtValue().(*chrome.Chrome)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanUp, err := ash.EnsureTabletModeEnabledWithKeyboardDisabled(ctx)
	if err != nil {
		s.Fatal("Failed to ensure DUT in tablet mode: ", err)
	}

	defer func(ctx context.Context) {
		if err := cleanUp(ctx); err != nil {
			s.Fatal("Failed to transition back to laptop mode: ", err)
		}
	}(cleanupCtx)

	cui := uiauto.New(tconn)
	searchTabElement := nodewith.NameStartingWith("Search your").Role(role.TextField)

	if err := cui.WaitForLocation(searchTabElement)(ctx); err != nil {
		s.Fatal("Failed to wait for search tab element: ", err)
	}

	if err := cui.LeftClick(searchTabElement)(ctx); err != nil {
		s.Fatal("Failed to left click search tab element: ", err)
	}

	const (
		hideKeyboardName       = "hide keyboard"
		switchEmojiName        = "switch to emoji"
		backspaceElementName   = "backspace"
		handwritingElementName = "switch to handwriting, not compatible with ChromeVox"
	)

	onscreenKeyboardElements := []string{hideKeyboardName, switchEmojiName,
		backspaceElementName, handwritingElementName}

	for _, element := range onscreenKeyboardElements {
		nodeElement := nodewith.Name(element).Role(role.Button)
		if err := cui.WaitUntilExists(nodeElement)(ctx); err != nil {
			s.Fatalf("Failed to wait for %s element on on-screen keyboard: %v", element, err)
		}
	}

	if err := apps.Launch(ctx, tconn, apps.Chrome.ID); err != nil {
		s.Fatal("Failed to launch chrome app: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Chrome.ID, time.Minute); err != nil {
		s.Fatal("Failed to launch Chrome browser after launch: ", err)
	}

	maximizeElement := nodewith.Name("Maximize").Role(role.Button)
	found, err := cui.IsNodeFound(ctx, maximizeElement)
	if err != nil {
		s.Fatal("Failed to wait for Maximize element on on-screen keyboard: ", err)
	}
	if found {
		s.Fatal("Failed to check UI element exist status")
	}

	keyboardAvailable, keyboardDevPath, err := input.FindPhysicalKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard event: ", err)
	}
	if !keyboardAvailable {
		s.Fatal("Failed to find keyboard input device")
	}

	const devRoot = "/dev"
	onboardKeyboardEventPath, err := filepath.Rel(devRoot, keyboardDevPath)
	if err != nil {
		s.Fatal("Failed to get keyboard event relative path: ", err)
	}

	onboardKeyboardStatus := "disabled"
	if err := inputDeviceDetectionCheck(ctx, onboardKeyboardEventPath, onboardKeyboardStatus); err != nil {
		s.Fatal("Failed to verify keyboard event in evtest in tablet mode: ", err)
	}

	const (
		usbDeviceClassName = "Mass Storage"
		usbSpeed           = "5000M"
	)
	usbDevicesList, err := usbutils.ListDevicesInfo(ctx, nil)
	if err != nil {
		s.Fatal("Failed to get USB devices list: ", err)
	}
	got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
	if want := 1; got != want {
		s.Fatalf("Unexpected number of USB devices connected: got %d, want %d", got, want)
	}

	// Long press the power button.
	emitter, err := power.NewPowerManagerEmitter(ctx)
	if err != nil {
		s.Fatal("Unable to create power manager emitter: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := emitter.Stop(cleanupCtx); err != nil {
			s.Log("Unable to stop emitter: ", err)
		}
	}(cleanupCtx)

	eventType := pmpb.InputEvent_POWER_BUTTON_DOWN
	if err := emitter.EmitInputEvent(ctx, &pmpb.InputEvent{Type: &eventType}); err != nil {
		s.Fatal("Send POWER_BUTTON_DOWN failed: ", err)
	}
	eventType = pmpb.InputEvent_POWER_BUTTON_UP
	if err := emitter.EmitInputEvent(ctx, &pmpb.InputEvent{Type: &eventType}); err != nil {
		s.Fatal("Send POWER_BUTTON_UP failed: ", err)
	}

	const (
		usbVolumeUpFile   = "volumeUp.txt"
		usbVolumeDownFile = "volumeDown.txt"
	)

	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create new volume helper: ", err)
	}

	preVolume, err := vh.GetVolume(ctx)
	if err != nil {
		s.Fatal("Failed to get present volume info: ", err)
	}
	if err := volumeButtonEvents(ctx, s.DataPath(usbVolumeUpFile)); err != nil {
		s.Fatal("Failed to press volume button: ", err)
	}
	infoAfterVolumeUp, err := vh.GetVolume(ctx)
	if err != nil {
		s.Fatal("Failed to get present volume info: ", err)
	}
	if preVolume == infoAfterVolumeUp {
		s.Fatalf("Failed to increase volume want %d got %d ", infoAfterVolumeUp, preVolume)
	}

	if err := volumeButtonEvents(ctx, s.DataPath(usbVolumeDownFile)); err != nil {
		s.Fatal("Failed to press volume button: ", err)
	}
	infoAfterVolumeDown, err := vh.GetVolume(ctx)
	if err != nil {
		s.Fatal("Failed to get present volume info: ", err)
	}
	if infoAfterVolumeUp == infoAfterVolumeDown {
		s.Fatalf("Failed to increase volume want %d got %d ", infoAfterVolumeDown, infoAfterVolumeUp)
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	internalDisplayID := infos[0].ID

	rotationAngles := []display.RotationAngle{display.Rotate90, display.Rotate180, display.Rotate270, display.Rotate0}
	for _, angle := range rotationAngles {
		if err := display.SetDisplayRotationSync(ctx, tconn, internalDisplayID, angle); err != nil {
			s.Fatalf("Failed to rotate to %v angle: %v", angle, err)
		}
	}
}

// inputDeviceDetectionCheck verifies input device eventPath has expectedDetectionStatus.
func inputDeviceDetectionCheck(ctx context.Context, eventPath, expectedDetectionStatus string) error {
	wakeSourceFile := fmt.Sprintf("/sys/class/%s/device/device/power/wakeup", eventPath)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		sourceOut, err := ioutil.ReadFile(wakeSourceFile)
		if err != nil {
			return errors.Wrapf(err, "failed to read %q file", wakeSourceFile)
		}
		got := strings.TrimSpace(string(sourceOut))
		if !strings.Contains(got, expectedDetectionStatus) {
			return errors.Errorf("unexpected detection status: got %q; want %q", got, expectedDetectionStatus)
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify input device detection check")
	}
	return nil
}

// volumeButtonEvents verifies whether volume button events are performed or not.
func volumeButtonEvents(ctx context.Context, volumeKeyFile string) error {
	usbKeyPlay := "evemu-play --insert-slot0 /dev/input/event"
	out, _ := exec.Command("evtest").CombinedOutput()
	re := regexp.MustCompile(fmt.Sprintf(`(?i)/dev/input/event([0-9]+):.*(%s).*`, regexp.QuoteMeta("cros_ec_buttons")))
	result := re.FindStringSubmatch(string(out))
	usbHSEventNum := ""
	if len(result) == 0 {
		return errors.New("Volume button not found in evtest command output")
	}
	usbHSEventNum = result[1]

	if err := testexec.CommandContext(ctx, "bash", "-c", usbKeyPlay+usbHSEventNum+" < "+volumeKeyFile).Run(); err != nil {
		return errors.Wrap(err, "failed to play volume button events")
	}
	return nil
}
