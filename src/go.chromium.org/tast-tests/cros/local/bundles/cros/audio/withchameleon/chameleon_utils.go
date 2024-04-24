// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package withchameleon contains chameleon-related test logic shared by local audio tests.
package withchameleon

import (
	"context"
	"os"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/audio/withchameleon"
	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const chameleonAfterPlugSleep = 5

// CleanupFunc type gives the format of defer functions so chameleon/dut can be put to the original condition
type CleanupFunc func(ctx context.Context) error

// DoFunc is a function that performs audio playback or record.
type DoFunc func(context.Context) (CleanupFunc, error)

// CtxWithCrasChameleondKey is the key used in withchameleon module
// contains "cras" and "chameleond"
type CtxWithCrasChameleondKey string

// Port describe a general port behaviour
type Port interface {
	// ToString prints the string representation of the port, for easier debugging
	ToString() string

	// Type describes the type of the Port: cros/chameleon
	Type() string
}

// OutputPort describe a behaviour of a playable port
type OutputPort interface {
	Port

	// SetupPlayback is called in withchameleon.Setup
	Setup(ctx context.Context) (CleanupFunc, error)

	// VerifySetup is called in after Setup completed, to verify the correctness of setup on the other side.
	VerifySetup(ctx context.Context) error

	// PreparePlayback prepares audio playback for the output port
	// and returns a DoFunc to run the playback.
	PreparePlayback(ctx context.Context, goldenFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error)
}

// InputPort describe a behaviour of a recordable port
type InputPort interface {
	Port

	// SetupPlayback is called in withchameleon.Setup
	Setup(ctx context.Context) (CleanupFunc, error)

	// VerifySetup is called in after Setup completed, to verify the correctness of setup on the other side.
	VerifySetup(ctx context.Context) error

	// PrepareRecord prepares audio record for the input port
	// and returns a DoFunc to run the record.
	PrepareRecord(ctx context.Context, recordingFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error)
}

// wrapCleanups create a new CleanupFunc from a list of CleanupFunc
// it calls these functions in a reverse order.
func wrapCleanups(deferFuncs []CleanupFunc) CleanupFunc {
	return func(cleanupCtx context.Context) error {
		var deferErrors []error
		deferFuncsCount := len(deferFuncs)
		for i := range deferFuncs {
			deferFunc := deferFuncs[deferFuncsCount-i-1]
			if deferFunc == nil {
				continue
			}
			deferErrors = append(deferErrors, deferFunc(cleanupCtx))
		}
		return errors.Join(deferErrors...)
	}
}

// VerifyPortType checks the correctness of in and out ports
func VerifyPortType(ctx context.Context, outPort OutputPort, inPort InputPort) error {
	if outPort.Type() == "chameleon" && inPort.Type() == "chameleon" {
		return errors.New("both port is on chameleon, but expect only one from chameleon")
	}

	// TODO: relaxed regarding portFamily
	// if inPort.ToPortFamily() != "" && inPort.ToPortFamily() != outPort.ToPortFamily() {
	// 	// Error is not raised here because RaspberryPi Analog setup actually violates the rule.
	// 	testing.ContextLogf(ctx, "WARNING: withchameleon.VerifyPortType port family is incompatible (in port family: %s, out port family: %s)", inPort.ToPortFamily(), outPort.ToPortFamily())
	// }
	return nil
}

// Setup function that prepare chameleon and plug the
// suspend and reboot can be called in between and the state of the dut/chameleond
// should remain the same.
func Setup(ctx context.Context, chameleond chameleon.Chameleond, outPort OutputPort, inPort InputPort) (deferFunc CleanupFunc, err error) {
	var deferFuncs []CleanupFunc

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.Setup "+format, args...)
	}

	logger("outPort: %s, inPort: %s", outPort.ToString(), inPort.ToString())

	// Prepare contexts
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to connect to CRAS")
	}

	crasCtx := context.WithValue(ctx, CtxWithCrasChameleondKey("cras"), cras)
	crasChameleondCtx := context.WithValue(crasCtx, CtxWithCrasChameleondKey("chameleond"), chameleond)

	// 1. Setup
	cleanupFunc, err := outPort.Setup(crasChameleondCtx)
	if err != nil {
		return wrapCleanups(deferFuncs), err
	}
	deferFuncs = append(deferFuncs, cleanupFunc)

	cleanupFunc, err = inPort.Setup(crasChameleondCtx)
	if err != nil {
		return wrapCleanups(deferFuncs), err
	}
	deferFuncs = append(deferFuncs, cleanupFunc)

	// GoBigSleepLint plugging require time to appears in CRAS
	testing.Sleep(ctx, time.Second*chameleonAfterPlugSleep)

	// 2. Verify Setup
	if err = outPort.VerifySetup(crasChameleondCtx); err != nil {
		return wrapCleanups(deferFuncs), err
	}

	if err = inPort.VerifySetup(crasChameleondCtx); err != nil {
		return wrapCleanups(deferFuncs), err
	}

	logger("setup complete")
	return wrapCleanups(deferFuncs), nil
}

// Orchestrate used to prepare recording and golden file, manage play and recording
// and compare recorded audio.
// 1. golden/record audio file created from DUT
// 2. if playing on chameleon; golden audio file will be copied to chameleon
// 3. else; after recording on chameleon recorded file will be copied to DUT
func Orchestrate(ctx context.Context, chameleond chameleon.Chameleond, outPort OutputPort, inPort InputPort) (deferFunc CleanupFunc, err error) {
	var deferFuncs []CleanupFunc

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.Orchestrate "+format, args...)
	}

	logger("outPortType: %s, inPortType: %s", outPort.ToString(), inPort.ToString())

	audioFormat := audio.DefaultChameleonUtilGeneratedAudioFormat

	// Prepare contexts
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to connect to CRAS")
	}

	crasCtx := context.WithValue(ctx, CtxWithCrasChameleondKey("cras"), cras)
	crasChameleondCtx := context.WithValue(crasCtx, CtxWithCrasChameleondKey("chameleond"), chameleond)

	// 1. creating recording/golden files
	logger("creating recording and golden file")
	recordingFile, err := os.CreateTemp("", "recorded_*.raw")
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to create temp recording file")
	}
	deferFuncs = append(deferFuncs, func(ctx context.Context) error {
		return os.Remove(recordingFile.Name())
	})

	goldenFile, err := os.CreateTemp("", "golden_*.raw")
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to create temp golden file")
	}
	deferFuncs = append(deferFuncs, func(ctx context.Context) error {
		return os.Remove(goldenFile.Name())
	})

	// 2. generating golden data
	logger("generating golden data")
	testRawDataFile := audio.TestRawData{
		Path:          goldenFile.Name(),
		BitsPerSample: audioFormat.BitsPerSample,
		Channels:      audioFormat.Channels,
		Rate:          audioFormat.Rate,
		Frequencies:   audioFormat.Frequencies,
		Volume:        0.05, // TODO: decide if this volume make sense
		Duration:      audioFormat.Duration,
	}
	if err := audio.GenerateTestRawData(ctx, testRawDataFile); err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to generate golden test data")
	}

	// 3. preparing the input and output ports
	recordFunc, cleanupFunc, err := inPort.PrepareRecord(crasChameleondCtx, recordingFile, audioFormat)
	deferFuncs = append(deferFuncs, cleanupFunc)
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to prepare recording function")
	}

	playbackFunc, cleanupFunc, err := outPort.PreparePlayback(crasChameleondCtx, goldenFile, audioFormat)
	deferFuncs = append(deferFuncs, cleanupFunc)
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to prepare playback function")

	}

	// 4. run play and record in parallel
	var wg sync.WaitGroup
	wg.Add(2)

	var playErr, recordErr error
	var playCleanup, recordCleanup CleanupFunc

	go func() {
		// 4a. play
		defer wg.Done()

		playCleanup, playErr = playbackFunc(crasChameleondCtx)
	}()

	go func() {
		// 4b. record
		defer wg.Done()

		// GoBigSleepLint sleeping before playback happens
		testing.Sleep(ctx, withchameleon.PlaybackInitSleepDuration)

		recordCleanup, recordErr = recordFunc(crasChameleondCtx)
	}()

	logger("starting and waiting for goroutines to finish")
	wg.Wait()
	logger("goroutines completed")

	deferFuncs = append(deferFuncs, playCleanup)
	deferFuncs = append(deferFuncs, recordCleanup)

	logger("logging all goroutines errors:")
	if playErr != nil {
		logger("Playing error ", playErr)
	}
	if recordErr != nil {
		logger("Recording error ", recordErr)
	}

	if playErr != nil || recordErr != nil {
		return wrapCleanups(deferFuncs), errors.Join(playErr, recordErr)
	}

	// 5. check recorded audio file size
	logger("checking if recording file is empty")
	recordingFileStat, err := recordingFile.Stat()
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to check recorded audio file size")
	}

	if recordingFileStat.Size() == 0 {
		return wrapCleanups(deferFuncs), errors.New("no audio recorded")
	}

	// 6. check recorded audio
	logger("checking audio frequencies")
	err = audio.CheckRecordedFrequency(crasChameleondCtx, recordingFile, audioFormat)
	if err != nil {
		return wrapCleanups(deferFuncs), errors.Wrap(err, "failed to check recorded frequency")
	}

	logger("orchestrate complete")
	return wrapCleanups(deferFuncs), nil
}
