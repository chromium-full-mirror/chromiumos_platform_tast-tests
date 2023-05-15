// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.AloopLoaded,
		Desc:            "Configure the ALSA loopback device for CRAS",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            &aloopLoadedFixture{},
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
		PreTestTimeout:  20 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.StereoAloopLoaded,
		Desc:            "Configure the ALSA loopback device as a stereo device for CRAS",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            &aloopLoadedFixture{Channels: 2},
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
		PreTestTimeout:  20 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.AloopLoadedWithoutUI,
		Desc:            "Configure the ALSA loopback device for CRAS and stop UI",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            uiStoppedFixture{},
		Parent:          fixture.AloopLoaded,
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
		PreTestTimeout:  20 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.StereoAloopLoadedWithoutUI,
		Desc:            "Configure the ALSA loopback device as a stereo device for CRAS and stop UI",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            uiStoppedFixture{},
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
		PreTestTimeout:  20 * time.Second,
	})
}

const aloopModuleName = "snd-aloop"

// LoadAloop loads snd-aloop module on kernel. A deferred call to the returned
// unloadAloop function to unload snd-aloop should be scheduled by the caller if
// err is non-nil.
//
// Deprecated: The unloadAloop function returned by LoadAloop does not handle
// errors, but just log them. Use the fixture.AloopLoaded fixture instead.
func LoadAloop(ctx context.Context) (func(context.Context), error) {
	if err := testexec.CommandContext(ctx, "modprobe", aloopModuleName).Run(testexec.DumpLogOnError); err != nil {
		return nil, err
	}

	// For compatibility, return a cleanup function which does not expose the errors.
	return func(ctx context.Context) {
		if err := unloadAloop(ctx); err != nil {
			testing.ContextLog(ctx, "unloadAloop() failed: ", err)
		}
	}, nil
}

func unloadAloop(ctx context.Context) error {
	// Process cras should be stopped first, otherwise snd-aloop would not be unloaded successfully.
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		return errors.Wrap(err, "failed to stop cras")
	}
	var modprobeError error
	if err := testexec.CommandContext(ctx, "modprobe", "-r", aloopModuleName).Run(testexec.DumpLogOnError); err != nil {
		modprobeError = errors.Wrapf(err, "failed to unload %s", aloopModuleName)
		testing.ContextLog(ctx, "unloadAloop(): ", modprobeError)
	}
	if err := upstart.EnsureJobRunning(ctx, "cras"); err != nil {
		return errors.Wrap(err, "failed to start cras")
	}
	return modprobeError
}

// SetupLoopback sets the playback and capture nodes to the ALSA loopback via the Quick Settings UI .
func SetupLoopback(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	timeForCleanUp := 5 * time.Second
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanUp)
	defer cancel()

	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show the quicksettings to select playback node")
	}
	defer func() {
		if err := quicksettings.Hide(ctxForCleanUp, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to hide the quicksettings on defer: ", err)
		}
	}()
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Playback"); err != nil {
		return errors.Wrap(err, "failed to select ALSA loopback output")
	}

	// After selecting Loopback Playback, SelectAudioOption() sometimes detected that audio setting
	// is still opened while it is actually fading out, and failed to select Loopback Capture.
	// Call Hide() and Show() to reset the quicksettings menu first.
	if err := quicksettings.Hide(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to hide the quicksettings before show")
	}
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show the quicksettings to select capture node")
	}
	if err := quicksettings.SelectAudioOption(ctx, tconn, "Loopback Capture"); err != nil {
		return errors.Wrap(err, "failed to select ALSA loopback input")
	}

	return nil
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

type aloopLoadedFixture struct {
	// Channels of the aloop device. 0 to not change the existing configuration.
	Channels int

	originalUCM []byte
}

func (f *aloopLoadedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
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

	if _, err := LoadAloop(ctx); err != nil {
		s.Fatal("Cannot load aloop: ", err)
	}

	return nil
}

func (f *aloopLoadedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.Channels != 0 {
		s.Log("Restoring ", aloopUCMPath)
		if err := testexec.CommandContext(ctx, "umount", aloopUCMPath).Run(testexec.DumpLogOnError); err != nil {
			s.Errorf("Cannot restore %s: %v", aloopUCMPath, err)
		}
	}

	// Unload aloop, which also restarts CRAS.
	if err := unloadAloop(ctx); err != nil {
		s.Error("Cannot unload aloop: ", err)
	}
}

func (aloopLoadedFixture) Reset(ctx context.Context) error {
	return nil
}

func (aloopLoadedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Restart CRAS to prevent CRAS state leakage between tests.
	if _, err := RestartCras(ctx); err != nil {
		s.Fatal("Cannot restart CRAS: ", err)
	}

	// Wait for the aloop device to be actually available in CRAS.
	cras, err := NewCras(ctx)
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

func (aloopLoadedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

type uiStoppedFixture struct{}

func (uiStoppedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Cannot stop ui: ", err)
	}
	return nil
}

func (uiStoppedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := upstart.EnsureJobRunning(ctx, "ui"); err != nil {
		s.Fatal("Cannot start ui: ", err)
	}
}

func (uiStoppedFixture) Reset(ctx context.Context) error {
	return nil
}

func (uiStoppedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (uiStoppedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
