// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/internal"
	"go.chromium.org/tast/core/testing"
)

// AloopLoaded is a ParameterizedFixture which sets up the snd-aloop device.
type AloopLoaded struct {
	// Channels of the aloop device. 0 to not change the existing configuration.
	Channels int
	// Name of the parent fixture.
	Parent string
}

var _ ParameterizedFixture = AloopLoaded{}

// Instance returns the instance name of the parameterized AloopLoaded fixture.
func (pf AloopLoaded) Instance() string {
	name := "aloopLoaded"
	if pf.Channels != 0 {
		name += fmt.Sprintf("%dch", pf.Channels)
	}
	if pf.Parent != "" {
		name += fmt.Sprintf("With%s%s", strings.ToUpper(pf.Parent[:1]), pf.Parent[1:])
	}
	return maybeRegisterFixture(&testing.Fixture{
		Name:     name,
		Desc:     fmt.Sprintf("Configure the ALSA loopback device with %v", pf),
		Contacts: []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl: &AloopLoadedFixture{
			Channels: pf.Channels,
		},
		Parent:          pf.Parent,
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
		PreTestTimeout:  20 * time.Second,
	})
}

const aloopUCMPath = "/usr/share/alsa/ucm/Loopback/HiFi.conf"

const aloopUCMTemplate = `SectionVerb {
	Value {
		FullySpecifiedUCM "1"
	}

	EnableSequence [
	]

	DisableSequence [
	]
}

SectionDevice."Loopback Playback".0 {
	Value {
		PlaybackPCM "hw:Loopback,0"
		PlaybackChannels "{{.Channels}}"
	}
}

SectionDevice."Loopback Capture".0 {
	Value {
		CapturePCM "hw:Loopback,1"
		CaptureChannels "{{.Channels}}"
	}
}
`

// AloopLoadedFixture is a fixture to load snd-aloop kernel module.
// Take note that this fixture doesn't select the output/input node.
// We need to call audio.SelectIODevices to select the output/input node
// via D-Bus, or SetupLoopback to select the output/input node via Quick Settings UI.
type AloopLoadedFixture struct {
	// Channels of the aloop device. 0 to not change the existing configuration.
	Channels int

	originalUCM []byte
}

// SetUp the AloopLoadedFixture
func (f *AloopLoadedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if f.Channels != 0 {
		s.Logf("Replacing %s with channels=%d", aloopUCMPath, f.Channels)
		var ucmContent bytes.Buffer
		if err := template.Must(template.New("HiFi.conf").Parse(aloopUCMTemplate)).Execute(&ucmContent, f); err != nil {
			s.Fatal("Cannot generate aloop HiFi.conf: ", err)
		}
		overrideUCMPath := filepath.Join(s.OutDir(), "LoopbackOverrideHiFi.conf")
		if err := os.WriteFile(overrideUCMPath, ucmContent.Bytes(), 0644); err != nil {
			s.Fatalf("Cannot write to %s: %v", overrideUCMPath, err)
		}
		if err := testexec.CommandContext(ctx, "mount", "--bind", overrideUCMPath, aloopUCMPath).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Cannot mount loopback UCM override: ", err)
		}
	}

	if _, err := internal.LoadAloop(ctx); err != nil {
		s.Fatal("Cannot load aloop: ", err)
	}

	// Provides pass-through for the value yielded by the parent fixture.
	return s.ParentValue()
}

// TearDown the AloopLoadedFixture
func (f *AloopLoadedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.Channels != 0 {
		s.Log("Restoring ", aloopUCMPath)
		if err := testexec.CommandContext(ctx, "umount", aloopUCMPath).Run(testexec.DumpLogOnError); err != nil {
			s.Errorf("Cannot restore %s: %v", aloopUCMPath, err)
		}
	}

	// Unload aloop, which also restarts CRAS.
	if err := internal.UnloadAloop(ctx); err != nil {
		s.Error("Cannot unload aloop: ", err)
	}
}

// Reset the AloopLoadedFixture
func (AloopLoadedFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest the AloopLoadedFixture by restarting CRAS and wait until loopback node is available
func (AloopLoadedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Restart CRAS to prevent CRAS state leakage between tests.
	if _, err := audio.RestartCras(ctx); err != nil {
		s.Fatal("Cannot restart CRAS: ", err)
	}

	// Wait for the aloop device to be actually available in CRAS.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}
	if err := testing.Poll(ctx,
		func(ctx context.Context) error {
			_, err := cras.GetNodeByType(ctx, "ALSA_LOOPBACK")
			return err
		},
		&testing.PollOptions{
			Timeout:  10 * time.Second,
			Interval: 1 * time.Second,
		},
	); err != nil {
		s.Error("CRAS alsa loopback device not found: ", err)
	}
}

// PostTest the AloopLoadedFixture
func (AloopLoadedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
