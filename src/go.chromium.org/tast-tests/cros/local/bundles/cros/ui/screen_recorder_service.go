// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	var screenRecorderService ScreenRecorderService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			screenRecorderService = ScreenRecorderService{sharedObject: common.SharedObjectsForServiceSingleton}
			pb.RegisterScreenRecorderServiceServer(srv, &screenRecorderService)
		},
		GuaranteeCompatibility: true,
	})
}

const (
	// Default interval in milliseconds for streaming messages
	intervalStreamMessage = 1000
)

// ScreenRecorderService implements tast.cros.ui.ScreenRecorderService
// It provides functionalities to perform screen recording.
type ScreenRecorderService struct {
	sharedObject *common.SharedObjectsForService
	mutex        sync.Mutex
	// For screen recording on DUT
	screenRecorder *uiauto.ScreenRecorder
	fileName       string
	// For streaming screen recording
	streamScreenRecorder *streamScreenRecorder
}

// Start creates a new media recorder and starts to record the screen.
// There can be only a single recording in progress at a time.
// If user does not specify the file name, the service will generate a
// temporary location for the recording and return that to the user in Stop().
func (svc *ScreenRecorderService) Start(ctx context.Context, req *pb.StartRequest) (*empty.Empty, error) {
	//Follow the same locking sequence from svc lock to shared object lock to avoid deadlock
	if !svc.mutex.TryLock() {
		return nil, errors.New("the service is being used and locked now")
	}
	defer svc.mutex.Unlock()
	if svc.isRecording(ctx) {
		return nil, errors.New("cannot start again when recording is in progress")
	}

	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	cr := svc.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// TODO(b/219091883): The underlying screen recording API leverages on screen navigation to start
	// the Screen recording. Screen recording is limited to the time when the user is logged
	// in and does not go beyond the boundary of a chrome session. Research to see recording
	// can span across chrome sessions.
	svc.screenRecorder, err = uiauto.NewScreenRecorder(ctx, tconn)
	if err != nil || svc.screenRecorder == nil {
		return nil, errors.Wrap(err, "failed to create ScreenRecorder")
	}
	if req.FileName != "" {
		svc.fileName = req.FileName
	} else {
		svc.fileName = ""
	}
	if err := svc.screenRecorder.Start(ctx, tconn); err != nil {
		svc.cleanup(ctx)
		return nil, errors.Wrap(err, "failed to start screen recording")
	}

	return &empty.Empty{}, nil
}

// Stop stops and saves the recording to the specified location.
func (svc *ScreenRecorderService) Stop(ctx context.Context, req *empty.Empty) (*pb.StopResponse, error) {
	if !svc.mutex.TryLock() {
		return nil, errors.New("the service is being used and locked now")
	}
	defer svc.mutex.Unlock()

	if !svc.isRecording(ctx) {
		return nil, errors.New("failed to stop when no recording is progress")
	}

	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	var fileName string
	if svc.fileName == "" {
		// Create a temporary file if user does not give a specific path
		tempFile, err := ioutil.TempFile("", "record*.webm")
		if err != nil {
			return nil, err
		}
		fileName = tempFile.Name()
	} else {
		// Ensure that parent directories of the provided path are created
		fileName = svc.fileName
		dir := path.Dir(fileName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create a dir at "+dir)
		}
	}

	uiauto.ScreenRecorderStopSaveRelease(ctx, svc.screenRecorder, fileName)
	svc.cleanup(ctx)
	return &pb.StopResponse{FileName: fileName}, nil
}

// cleanup cleans up and resets the service.
func (svc *ScreenRecorderService) cleanup(ctx context.Context) {
	svc.screenRecorder = nil
	svc.fileName = ""
	svc.streamScreenRecorder = nil
}

// isRecording checks if a screen recording is in progress.
func (svc *ScreenRecorderService) isRecording(ctx context.Context) bool {
	return svc.screenRecorder != nil || svc.streamScreenRecorder != nil
}

// streamScreenRecorder provides the ability to stream screen recording.
type streamScreenRecorder struct {
	isRecording   bool
	videoRecorder *chrome.JSObject
}

// StreamScreenRecording creates a new media recorder, records the screen and
// streams the content.
// Incremental data chunks can be appended to a file as they arrive and the
// resulting screen recording file will be valid.
// There can be only a single recording in progress at a time.
func (svc *ScreenRecorderService) StreamScreenRecording(req *pb.StreamScreenRecordingRequest,
	stream pb.ScreenRecorderService_StreamScreenRecordingServer) error {
	// In the streaming scenario, the lock on the service is held through out the whole duration.
	// Instead of waiting for the lock, we will return error right away to avoid deadlock.
	if !svc.mutex.TryLock() {
		return errors.New("the service is being used and locked now")
	}
	defer svc.mutex.Unlock()

	if svc.isRecording(stream.Context()) {
		return errors.New("cannot start again when recording is in progress")
	}

	interval := req.Interval
	if interval == 0 {
		interval = intervalStreamMessage
	}

	r, err := svc.newStreamScreenRecorder(stream.Context(), interval)
	if err != nil {
		return err
	}
	svc.streamScreenRecorder = r

	// This loop sends recording bytes at a regular cadence.
	for {
		select {
		case <-stream.Context().Done():
			// context is canceled and stream is closed, so even if we have remaining video
			// blob, we cannot send them to the client side.
			testing.ContextLog(stream.Context(), "stream.Context() is done and stream is closed")
			// The context of the stream is already closed. We need a new background context to
			// handle the best effort cleanup.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errRelease := r.release(cleanupCtx)
			testing.ContextLog(stream.Context(), "Finshed releasing screen recorder. Err: ", errRelease)
			svc.cleanup(cleanupCtx)
			return status.Error(codes.Canceled, "Stream has ended")
		case <-time.After(time.Millisecond * time.Duration(interval)):
			if err := streamRecordingBytes(stream.Context(), r, stream); err != nil {
				testing.ContextLog(stream.Context(), "Failed to stream recording: ", err)
				errRelease := r.release(stream.Context())
				testing.ContextLog(stream.Context(), "Finshed releasing screen recorder. Err: ", errRelease)
				svc.cleanup(stream.Context())
				return err
			}
		}
	}
}

// streamRecordingBytes retrieves the screen recording chunks from streamScreenRecorder
// and sends the chunks through gRPC streaming.
func streamRecordingBytes(ctx context.Context, sr *streamScreenRecorder, stream pb.ScreenRecorderService_StreamScreenRecordingServer) error {
	type ScreenRecordingBytes struct {
		Data   []byte `json:"data"`
		Length int    `json:"length"`
	}
	// Get the screen recording chunks from a buffer, convert it in a byte array and clear the buffer.
	expr := `async function() {
		if (this.chunks.length == 0) {
			return {
				'blob':null,
				'length':0,
			};
		}
		const blob = new Blob(this.chunks, {'type': 'video/webm'});
		const arrayBuffer = await blob.arrayBuffer();
		const uint8View = new Uint8Array(arrayBuffer);
		const videoBytes = Array.from(uint8View);

		this.chunks = [];
		return {
			'data':videoBytes,
			'length':videoBytes.length,
		};
	}`
	var videoBytes ScreenRecordingBytes
	if err := sr.videoRecorder.Call(ctx, &videoBytes, expr); err != nil {
		return err
	}
	if videoBytes.Length == 0 {
		return nil
	}
	res := &pb.StreamScreenRecordingResponse{
		Data:   videoBytes.Data,
		Length: uint32(videoBytes.Length),
	}
	err := stream.SendMsg(res)
	return err
}

// newStreamScreenRecorder creates a streaming screen recorder
func (svc *ScreenRecorderService) newStreamScreenRecorder(ctx context.Context, interval uint32) (*streamScreenRecorder, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	cr := svc.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("chrome is not instantiated")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	sr, err := requestScreenShare(ctx, tconn, interval)
	if err != nil {
		return nil, err
	}

	// Choose to record the entire desktop/screen with no audio.
	if err := uiauto.ChooseScreenRecorder(ctx, tconn); err != nil {
		return nil, err
	}

	if err := sr.start(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to start stream recording")
	}
	return sr, nil
}

// requestScreenShare creates a streaming screen recorder and initiates
// the UI flow to request for sharing screen.
func requestScreenShare(ctx context.Context, tconn *chrome.TestConn, interval uint32) (*streamScreenRecorder, error) {
	expr := fmt.Sprintf(
		`({
			chunks: [],
			recorder: null,
			streamPromise: null,
			videoTrack: null,
			request: function() {
				this.streamPromise = new Promise(resolve => {
					chrome.desktopCapture.chooseDesktopMedia(["screen", "window", "tab"], (streamId) => {
						navigator.mediaDevices.getUserMedia({
							video: {
								mandatory: {
									chromeMediaSource: "desktop",
									chromeMediaSourceId: streamId
								}
							}
						}).then((stream) => resolve(stream));
					});
				});
			},
			start: function() {
				this.chunks = [];
				return this.streamPromise.then(
					stream => {
						this.videoTrack = stream.getVideoTracks()[0];
						this.recorder = new MediaRecorder(stream, {mimeType: 'video/webm;codecs=vp9'});
						this.recorder.ondataavailable = (e) => {
							this.chunks.push(e.data);
						};
						this.recorder.start(%d);
					}
				);
			},
			stop: function() {
				return new Promise((resolve, reject) => {
					this.recorder.onstop = function() {
						resolve();
					};
					this.recorder.stop();
					this.videoTrack.stop();
				});
			}
		})
	`, interval)
	videoRecorder := &chrome.JSObject{}
	if err := tconn.Eval(ctx, expr, videoRecorder); err != nil {
		return nil, errors.Wrap(err, "failed to initialize video recorder")
	}
	sr := &streamScreenRecorder{
		isRecording:   false,
		videoRecorder: videoRecorder,
	}

	// Request to share the screen.
	// For security reason, the permission of screen sharing cannot be done purely with API
	// and can only be given through interacting with the UI.
	if err := sr.videoRecorder.Call(ctx, nil, `function() {this.request();}`); err != nil {
		return nil, errors.Wrap(err, "failed to request display media")
	}
	return sr, nil
}

// start creates a new media recorder and starts to record the screen.
func (r *streamScreenRecorder) start(ctx context.Context, tconn *chrome.TestConn) error {
	if r.isRecording == true {
		return errors.New("recorder already started")
	}

	if err := r.videoRecorder.Call(ctx, nil, `function() {this.start();}`); err != nil {
		return errors.Wrap(err, "failed to start to record screen")
	}
	testing.ContextLog(ctx, "Started screen recording")
	r.isRecording = true

	ui := uiauto.New(tconn)
	closeNotificationButton := nodewith.Name("Notification close").Role(role.Button)
	messagePopupAlert := nodewith.HasClass("MessagePopupView").Role(role.AlertDialog)
	if err := ui.LeftClickUntil(closeNotificationButton,
		ui.WithInterval(time.Second).WithTimeout(5*time.Second).WaitUntilGone(messagePopupAlert))(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to dismiss screenshare notification popup, it likely didn't appear: ", err)
	}
	return nil
}

// stop stops the screen recording.
func (r *streamScreenRecorder) stop(ctx context.Context) error {
	if r.isRecording == false {
		return errors.New("recorder hasn't started yet")
	}
	if err := r.videoRecorder.Call(ctx, nil, `function() {return this.stop();}`); err != nil {
		return errors.Wrap(err, "failed to stop recording screen")
	}
	r.isRecording = false

	return nil
}

// release frees streamScreenRecorder reference to Javascript for this video recorder.
func (r *streamScreenRecorder) release(ctx context.Context) error {
	if r.isRecording {
		if err := r.stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop screen recorder: ", err)
			return err
		}
	}
	if err := r.videoRecorder.Release(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to release screen recorder: ", err)
		return err
	}
	return nil
}
