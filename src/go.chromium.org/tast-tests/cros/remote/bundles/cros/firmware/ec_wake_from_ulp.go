// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	wakeDUTWithPwrBtn = iota
	wakeDUTWithLidSwitch
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECWakeFromULP,
		Desc: "Verify waking the EC from suspend by lid switch and pressing the power button",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		SoftwareDeps: []string{"ec_hibernate"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name: "lid_switch",
			ExtraHardwareDeps: hwdep.D(hwdep.Lid(), hwdep.SkipOnModel(
				// The lid_switch variant of this test requires servo_micro. Please skip on C2D2 devices.
				// If the board is not one of Brask, Brya, Cherry, Dedede, Guybrush, Nissa, Rex, or Skyrim, it's probably not a C2D2 device.
				// Brask
				"gladios",
				"lisbon",
				// Brya
				"anahera",
				"banshee",
				"calus",
				"dochi",
				"felwinter",
				"gimble",
				"kano",
				"mithrax",
				"omnigul",
				"omniknight",
				"osiris",
				"redrix",
				"taeko",
				"taniks",
				"vell",
				"volmar",
				"xol",
				"zydron",
				// Cherry
				"cherry",
				"dojo",
				"tomato",
				// Dedede
				"awasuki",
				"awadoron",
				"beetley",
				"blipper",
				"bookem",
				"boten",
				"bugzzy",
				"cret",
				"cret360",
				"drawcia",
				"drawlat",
				"drawman",
				"drawper",
				"kracko",
				"kracko360",
				"landia",
				"landrid",
				"lantis",
				"madoo",
				"magister",
				"maglet",
				"maglia",
				"maglith",
				"magma",
				"magneto",
				"magolor",
				"magpie",
				"metaknight",
				"palutena",
				"pasara",
				"peezer",
				"pirette",
				"pirika",
				"sasukette",
				"storo",
				// Guybrush
				"dewatt",
				"nipperkin",
				//Nirva,
				"craaskyu2",
				"ruken",
				"rynar",
				"telithn50",
				"telticn50",
				// Nissa
				"anraggar",
				"anraggar360",
				"craask",
				"craaskana",
				"craaskbowl",
				"craaskino",
				"craaskov",
				"craaskvin",
				"craasneto",
				"craaswell",
				"domika",
				"domilly",
				"domiso",
				"epic",
				"epiccs",
				"gallida360",
				"gana",
				"glassway",
				"gothrax",
				"guren",
				"guren360",
				"joxer",
				"meliks",
				"nirwen",
				"pujjoga",
				"pujjogatwin",
				"pujjoniru",
				"quandiso",
				"quandiso360",
				"quandiso2",
				"quandiso3602",
				"riven",
				"roric",
				"rudriks",
				"ruke",
				"rull",
				"rynax",
				"sundance",
				"teliks",
				"teliks360",
				"telith",
				"teltic",
				"uldren",
				"uldren360",
				"xivu",
				"xivu360",
				"yahiko",
				"yavijo",
				"yaviks",
				"yavikso",
				"yavilla",
				"yavilly",
				// Rex
				"kanix",
				"karis",
				"screebo",
				"screebo4es",
				// Skyrim
				"crystaldrift",
				"frostflow",
				"markarth",
				"skyrim15w",
				"whiterun",
			)),
			Val: wakeDUTWithLidSwitch,
		}, {
			Name: "power_button",
			Val:  wakeDUTWithPwrBtn,
		}},
	})
}

func ECWakeFromULP(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	wakeSrc := s.Param().(int)
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	servoType, err := h.Servo.GetServoType(ctx)
	if err != nil {
		s.Fatal("Failed to find servo type: ", err)
	}
	s.Log("Servo type: ", servoType)

	// To wake DUT up from hibernate by power button, we need either
	// servo_micro or C2D2. On the other hand, we can only wake DUT up
	// by lid switch with servo_micro.
	hasMicroOrC2D2, err := h.Servo.PreferDebugHeader(ctx)
	if err != nil {
		s.Fatal("Failed to check for servo micro or C2D2 connected: ", err)
	}

	hasC2D2, err := h.Servo.HasC2D2(ctx)
	if err != nil {
		s.Fatal("Failed to check c2d2: ", err)
	}

	if !hasMicroOrC2D2 {
		s.Fatal("There is no servo micro or C2D2 connected")
	} else if wakeSrc == wakeDUTWithLidSwitch && hasC2D2 {
		s.Fatal("A servo_micro to wake DUT by lid switch, please add this model to the skip list for lid_switch test")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := h.Servo.OpenLid(ctx); err != nil {
			s.Fatal("Failed to set lid state: ", err)
		}
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT booted: ", err)
		}
	}(cleanupCtx)

	s.Log("Stopping AC Power")
	if err := powercontrol.PlugUnplugCharger(ctx, h, false); err != nil {
		s.Fatal("Failed to stop power supply: ", err)
	}

	if err := firmware.ShutdownDUT(ctx, h); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}

	s.Log("Putting DUT in hibernation with EC console command")
	if err := h.Servo.ECHibernate(ctx, h.Model, servo.UseConsole); err != nil {
		s.Log("Failed to hibernate: ", err)
	}

	if err := wakeDUTFromULP(ctx, h, wakeSrc); err != nil {
		s.Fatal("Failed to wake DUT: ", err)
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
}

func wakeDUTFromULP(ctx context.Context, h *firmware.Helper, wakeSrc int) error {
	switch wakeSrc {
	case wakeDUTWithPwrBtn:
		testing.ContextLog(ctx, "Wake DUT by a short press on the power button")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurShortPress); err != nil {
			errors.Wrap(err, "failed to press power button")
		}
	case wakeDUTWithLidSwitch:
		testing.ContextLog(ctx, "Wake DUT by lid switch")
		if err := lidSwitch(ctx, h); err != nil {
			errors.Wrap(err, "failed to do lid switch")
		}
	default:
		errors.Errorf("unexpected wake source: %d", wakeSrc)
	}
	return nil
}

func lidSwitch(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		errors.Wrap(err, "failed to close lid")
	}
	// GoBigSleepLint: Sleeping 1 sec to wait for lid close complete
	testing.Sleep(ctx, 1*time.Second)
	if err := h.Servo.OpenLid(ctx); err != nil {
		errors.Wrap(err, "failed to open lid")
	}
	return nil
}
