// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package withchameleon

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/audio/withchameleon"
	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ChameleonPort describe a playable/recordable port that is on Chameleon
type ChameleonPort interface {
	ToChameleonPortID(ctx context.Context) (chameleon.PortID, error)
}

// ChameleonOutputPort describe a playable port that is on Chameleon
type ChameleonOutputPort struct {
	OutputPort
	ChameleonPort
	ChamPortType chameleon.PortType
}

// ChameleonInputPort describe a recordable port that is on Chameleon
type ChameleonInputPort struct {
	InputPort
	ChameleonPort
	ChamPortType chameleon.PortType
}

// ToString shows the string representation of the ChameleonOutputPort
func (outputPort ChameleonOutputPort) ToString() string {
	return fmt.Sprintf("ChameleonOutputPort{ ChamPortType: %s }", outputPort.ChamPortType)
}

// ToString shows the string representation of the ChameleonInputPort
func (inputPort ChameleonInputPort) ToString() string {
	return fmt.Sprintf("ChameleonInputPort{ ChamPortType: %s }", inputPort.ChamPortType)
}

// Type describes the type of the Port which is "chameleon"
func (outputPort ChameleonOutputPort) Type() string {
	return "chameleon"
}

// Type describes the type of the Port which is "chameleon"
func (inputPort ChameleonInputPort) Type() string {
	return "chameleon"
}

// PreparePlayback for ChameleonOutputPort prepares audio playback for chameleon ports.
func (outputPort ChameleonOutputPort) PreparePlayback(ctx context.Context, goldenFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error) {
	var deferFuncs []CleanupFunc

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.ChameleonOutputPort.PreparePlayback "+format, args...)
	}

	chameleond, ok := ctx.Value(CtxWithCrasChameleondKey("chameleond")).(chameleon.Chameleond)
	if !ok {
		return nil, wrapCleanups(deferFuncs), errors.New("missing chameleond in context values")
	}

	logger("converting to chameleon portID (chamPortType: %s)", outputPort.ChamPortType)
	chamPortID, err := outputPort.ToChameleonPortID(ctx)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), err
	}

	playToken, err := audio.CopyPlayFileToChameleon(ctx, chameleond, goldenFile.Name())
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrap(err, "failed to copy golden test data to chameleon")
	}
	deferFuncs = append(deferFuncs, func(ctx context.Context) error {
		err := chameleond.DeleteFileInChameleon(ctx, playToken)
		return err
	})

	playbackDuration := time.Duration(audioFormat.Duration) * time.Second

	// description a chameleon playback procedure
	return func(ctx context.Context) (CleanupFunc, error) {
		var playbackCleanupFuncs []CleanupFunc
		var playbackErr error

		logger("playing: on chameleon (chamPortID: %d)", chamPortID)

		if playToken == "" {
			playbackErr = errors.New("missing play token on chameleon")
			return wrapCleanups(playbackCleanupFuncs), playbackErr
		}

		// play with chameleon
		chameleonAudioFormat := &chameleon.AudioDataFormat{
			FileType:     chameleon.AudioFileTypeRaw,
			SampleFormat: chameleon.AudioSampleFormatS16LE,
			Channel:      audioFormat.Channels,
			Rate:         audioFormat.Rate,
		}
		logger("playing: start playing on chameleon")
		playbackErr = chameleond.StartPlayingAudioWithToken(ctx, chamPortID, playToken, chameleonAudioFormat)
		if playbackErr != nil {
			return wrapCleanups(playbackCleanupFuncs), errors.Wrap(playbackErr, "failed to start playing audio on chameleon")
		}

		// GoBigSleepLint stop playing after duration.
		testing.Sleep(ctx, playbackDuration)

		logger("playing: stop playing on chameleon")
		playbackErr = chameleond.StopPlayingAudio(ctx, chamPortID)
		if playbackErr != nil {
			return wrapCleanups(playbackCleanupFuncs), errors.Wrap(playbackErr, "failed to stop playing audio on chameleon")
		}

		logger("playing: completed on chameleon")

		return wrapCleanups(playbackCleanupFuncs), nil
	}, wrapCleanups(deferFuncs), nil
}

// PrepareRecord for chameleonInputPort prepares audio record for chameleon ports.
func (inputPort ChameleonInputPort) PrepareRecord(ctx context.Context, recordingFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error) {
	var deferFuncs []CleanupFunc

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.ChameleonInputPort.PrepareRecord "+format, args...)
	}

	chameleond, ok := ctx.Value(CtxWithCrasChameleondKey("chameleond")).(chameleon.Chameleond)
	if !ok {
		return nil, wrapCleanups(deferFuncs), errors.New("missing chameleond in context values")
	}

	logger("converting to chameleon portID (chamPortType: %s)", inputPort.ChamPortType.String())
	chamPortID, err := inputPort.ToChameleonPortID(ctx)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), err
	}

	playbackDuration := time.Duration(audioFormat.Duration) * time.Second
	recordDuration := withchameleon.CalculateRecordDuration(playbackDuration)

	// description a chameleon recording procedure
	return func(ctx context.Context) (CleanupFunc, error) {
		var recordCleanupFuncs []CleanupFunc
		var recordErr error

		logger("recording: on chameleon (chamPortID: %d)", chamPortID)

		logger("recording: start recording on chameleon")
		recordErr = chameleond.StartCapturingAudio(ctx, chamPortID, true)
		if recordErr != nil {
			recordErr = errors.Wrap(recordErr, "failed to start recording audio on chameleon")
			return wrapCleanups(recordCleanupFuncs), recordErr
		}

		// GoBigSleepLint wait for playback to complete.
		testing.Sleep(ctx, recordDuration)

		logger("recording: stop recording on chameleon")
		tempCapturingDetails, recordErr := chameleond.StopCapturingAudioAndGetToken(ctx, chamPortID)
		if recordErr != nil {
			recordErr = errors.Wrap(recordErr, "failed to stop recording audio on chameleon")
			return wrapCleanups(recordCleanupFuncs), recordErr
		}

		recordToken := tempCapturingDetails["token"]
		recordCleanupFuncs = append(recordCleanupFuncs, func(ctx context.Context) error {
			return chameleond.DeleteFileInChameleon(ctx, recordToken)
		})

		logger("recording: copying recorded file from chameleon")
		recordErr = audio.CopyRecordFileFromChameleon(ctx, chameleond, recordToken, recordingFile.Name())
		if recordErr != nil {
			recordErr = errors.Wrap(err, "failed to copy recorded file on chameleon")
			return wrapCleanups(recordCleanupFuncs), recordErr
		}

		logger("recording: complete on chameleon")

		return wrapCleanups(recordCleanupFuncs), nil
	}, wrapCleanups(deferFuncs), nil
}

// ToChameleonPortID converts chameleon.PortType to chameleond XMLRPC PortID format
func ToChameleonPortID(ctx context.Context, chamPortType chameleon.PortType) (chameleon.PortID, error) {
	chameleond, ok := ctx.Value(CtxWithCrasChameleondKey("chameleond")).(chameleon.Chameleond)
	if !ok {
		return 0, errors.New("missing chameleond in context values")
	}

	// PortType to PortID
	chamPortID, err := chameleond.FetchSupportedPortIDByType(ctx, chamPortType, 0)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to translate chameleon portType to portID (chamPortType: %s)", chamPortType.String())
	}
	return chamPortID, nil
}

// ToChameleonPortID converts chameleon.PortType to chameleond XMLRPC PortID format
func (outputPort ChameleonOutputPort) ToChameleonPortID(ctx context.Context) (chameleon.PortID, error) {
	return ToChameleonPortID(ctx, outputPort.ChamPortType)
}

// ToChameleonPortID converts chameleon.PortType to chameleond XMLRPC PortID format
func (inputPort ChameleonInputPort) ToChameleonPortID(ctx context.Context) (chameleon.PortID, error) {
	return ToChameleonPortID(ctx, inputPort.ChamPortType)
}

func plugChameleonPort(ctx context.Context, chamPort ChameleonPort) (CleanupFunc, error) {
	var err error
	chameleond, ok := ctx.Value(CtxWithCrasChameleondKey("chameleond")).(chameleon.Chameleond)
	if !ok {
		return nil, errors.New("missing chameleond in context values")
	}

	chamPortID, err := chamPort.ToChameleonPortID(ctx)
	if err != nil {
		return nil, err
	}

	if err = chameleond.Plug(ctx, chamPortID); err != nil {
		return nil, errors.Wrapf(err, "failed to chameleon plug (chamPortID: %d)", chamPortID.Int())
	}
	return func(ctx context.Context) error {
		return chameleond.Unplug(ctx, chamPortID)
	}, nil
}

// Setup plugs chameleon port with chameleond
func (outputPort ChameleonOutputPort) Setup(ctx context.Context) (CleanupFunc, error) {
	return plugChameleonPort(ctx, outputPort)
}

// Setup plugs chameleon port with chameleond
func (inputPort ChameleonInputPort) Setup(ctx context.Context) (CleanupFunc, error) {

	return plugChameleonPort(ctx, inputPort)
}

// VerifySetup do nothing on chameleonOutputPort
func (outputPort ChameleonOutputPort) VerifySetup(ctx context.Context) error {
	return nil
}

// VerifySetup do nothing on chameleonInputPort
func (inputPort ChameleonInputPort) VerifySetup(ctx context.Context) error {
	return nil
}
