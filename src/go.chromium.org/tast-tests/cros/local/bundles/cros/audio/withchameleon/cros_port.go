// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package withchameleon contains chameleon-related test logic shared by local audio tests.
package withchameleon

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/audio/withchameleon"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CrosPort describe a playable/recordable port that is on Cras
type CrosPort interface {
	ToCrasNode(ctx context.Context) (*audio.CrasNode, error)
}

// CrosOutputPort describe a playable port that is on Cras
type CrosOutputPort struct {
	OutputPort
	CrosPort
	CrasNodeType string
}

// CrosInputPort describe a recordable port that is on Cras
type CrosInputPort struct {
	InputPort
	CrosPort
	CrasNodeType string
}

// ToString shows the string representation of the CrosOutputPort
func (outputPort CrosOutputPort) ToString() string {
	return fmt.Sprintf("CrosOutputPort{ CrasNodeType: %s }", outputPort.CrasNodeType)
}

// ToString shows the string representation of the CrosInputPort
func (inputPort CrosInputPort) ToString() string {
	return fmt.Sprintf("CrosInputPort{ CrasNodeType: %s }", inputPort.CrasNodeType)
}

// Type describes the type of the Port which is "cros"
func (outputPort CrosOutputPort) Type() string {
	return "cros"
}

// Type describes the type of the Port which is "cros"
func (inputPort CrosInputPort) Type() string {
	return "cros"
}

// PreparePlayback for CrosOutputPort prepares audio playback for cros port
func (outputPort CrosOutputPort) PreparePlayback(ctx context.Context, goldenFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error) {
	var deferFuncs []CleanupFunc
	var err error

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.CrosOutputPort.PrepareRecord "+format, args...)
	}

	cras, ok := ctx.Value(CtxWithCrasChameleondKey("cras")).(*audio.Cras)
	if !ok {
		return nil, wrapCleanups(deferFuncs), errors.New("missing cras in context values")
	}

	crasNode, err := outputPort.ToCrasNode(ctx)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrapf(err, "failed check and get cras node (crasNodeType: %s)", outputPort.CrasNodeType)
	}

	logger("setting active cras node", *crasNode)
	err = cras.SetActiveNode(ctx, *crasNode)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrapf(err, "failed to set active node (cras_node: %v)", *crasNode)
	}

	logger("setting output node volume", *crasNode)
	err = cras.SetOutputNodeVolume(ctx, *crasNode, 75) // TODO: decide if this volume make sense
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrapf(err, "failed to set output node volume through cras (cras_node: %v)", *crasNode)
	}
	// TODO: missing reverting of audio volume

	playbackDuration := time.Duration(audioFormat.Duration) * time.Second

	// description a chameleon playback procedure
	return func(ctx context.Context) (CleanupFunc, error) {
		var playbackCleanupFuncs []CleanupFunc
		var playbackErr error

		logger("playing: on dut")

		tempPlaybackCmd := crastestclient.PlaybackFileCommand(ctx, goldenFile.Name(), int(playbackDuration.Seconds()), audioFormat.Channels, audioFormat.Rate)

		logger("playing: with command: %v", tempPlaybackCmd.Args)
		playbackErr = tempPlaybackCmd.Run(testexec.DumpLogOnError)
		if playbackErr != nil {
			playbackErr = errors.Wrap(playbackErr, "failed to start playing audio on dut")
			return wrapCleanups(playbackCleanupFuncs), playbackErr
		}

		logger("playing: completed on dut")
		return wrapCleanups(playbackCleanupFuncs), nil
	}, wrapCleanups(deferFuncs), nil
}

// PrepareRecord for CrosInputPort prepares audio record for cros port
func (inputPort CrosInputPort) PrepareRecord(ctx context.Context, recordingFile *os.File, audioFormat *audio.TestRawData) (DoFunc, CleanupFunc, error) {
	var deferFuncs []CleanupFunc
	var err error

	logger := func(format string, args ...interface{}) {
		testing.ContextLogf(ctx, "withchameleon.CrosInputPort.PrepareRecord "+format, args...)
	}

	cras, ok := ctx.Value(CtxWithCrasChameleondKey("cras")).(*audio.Cras)
	if !ok {
		return nil, wrapCleanups(deferFuncs), errors.New("missing cras in context values")
	}

	logger("converting to cras node (crasNodeType: %s)", inputPort.CrasNodeType)
	crasNode, err := inputPort.ToCrasNode(ctx)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrapf(err, "failed check and get cras node (crasNodeType: %s)", inputPort.CrasNodeType)
	}

	logger("setting active cras node")
	err = cras.SetActiveNode(ctx, *crasNode)
	if err != nil {
		return nil, wrapCleanups(deferFuncs), errors.Wrapf(err, "failed to set active node: %v ", *crasNode)
	}

	playbackDuration := time.Duration(audioFormat.Duration) * time.Second
	recordDuration := withchameleon.CalculateRecordDuration(playbackDuration)

	// description a chameleon playback procedure
	return func(ctx context.Context) (CleanupFunc, error) {
		var recordCleanupFuncs []CleanupFunc
		var recordErr error

		logger("recording: on dut")

		tempCaptureCmd := crastestclient.CaptureFileCommand(
			ctx, recordingFile.Name(),
			int(recordDuration.Seconds()),
			audioFormat.Channels,
			audioFormat.Rate)

		logger("recording: with command: %v", tempCaptureCmd.Args)
		recordErr = tempCaptureCmd.Run(testexec.DumpLogOnError)
		if recordErr != nil {
			recordErr = errors.Wrap(recordErr, "failed to record audio on dut")
			return wrapCleanups(recordCleanupFuncs), recordErr
		}

		logger("recording: completed on dut")

		return wrapCleanups(recordCleanupFuncs), nil
	}, wrapCleanups(deferFuncs), nil
}

// ToCrasNode convert crasNodeType to CrasNode.
func ToCrasNode(ctx context.Context, crasNodeType string, isInput bool) (*audio.CrasNode, error) {
	cras, ok := ctx.Value(CtxWithCrasChameleondKey("cras")).(*audio.Cras)
	if !ok {
		return nil, errors.New("missing cras in context values")
	}

	var crasNodeStreamType audio.StreamType
	if isInput {
		crasNodeStreamType = audio.InputStream
	} else {
		crasNodeStreamType = audio.OutputStream
	}

	crasNodes, err := cras.GetNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get all cras nodes")
	}

	crasNode, err := cras.GetNodeByMatcher(ctx, &audio.MatchNodeTypeDirection{Type: crasNodeType, Direction: crasNodeStreamType})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot find cras node with node_type: %s, node_stream_type: %s, all_nodes: %v", crasNodeType, crasNodeStreamType.String(), crasNodes)
	}

	return crasNode, nil
}

// ToCrasNode convert crasNodeType to CrasNode
func (outputPort CrosOutputPort) ToCrasNode(ctx context.Context) (*audio.CrasNode, error) {
	return ToCrasNode(ctx, outputPort.CrasNodeType, false)
}

// ToCrasNode convert crasNodeType to CrasNode
func (inputPort CrosInputPort) ToCrasNode(ctx context.Context) (*audio.CrasNode, error) {
	return ToCrasNode(ctx, inputPort.CrasNodeType, true)
}

// Setup do nothing for CrosOutputPort
func (outputPort CrosOutputPort) Setup(ctx context.Context) (CleanupFunc, error) {
	return nil, nil
}

// Setup do nothing for CrosInputPort
func (inputPort CrosInputPort) Setup(ctx context.Context) (CleanupFunc, error) {
	return nil, nil
}

// VerifySetup check if a cras node can be obtained from cras
func (outputPort CrosOutputPort) VerifySetup(ctx context.Context) error {
	if _, err := outputPort.ToCrasNode(ctx); err != nil {
		return errors.Wrapf(err, "failed check and get cras node (crasNodeType: %s)", outputPort.CrasNodeType)
	}
	return nil
}

// VerifySetup check if a cras node can be obtained from cras
func (inputPort CrosInputPort) VerifySetup(ctx context.Context) error {
	if _, err := inputPort.ToCrasNode(ctx); err != nil {
		return errors.Wrapf(err, "failed check and get cras node (crasNodeType: %s)", inputPort.CrasNodeType)
	}
	return nil
}
