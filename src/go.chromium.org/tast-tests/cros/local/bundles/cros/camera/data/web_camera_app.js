// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/**
 * Functions of the web camera app
 */
class CameraApp {
  constructor() {
    this.cameraCapture = null;
    this.mediaRecorder = null;
    this.initDone = false;
    this.webLoader = document.getElementById('web-loader');
    this.cameraImage = document.getElementById('camera-image');
    this.videoOutput = document.getElementById('video-output');
    this.cameraSelector = document.getElementById('change-camera-selection');
    this.frameRateInput = document.getElementById('frame-rate-input');
    this.frameRateOutout = document.getElementById('frame-rate-value');
    this.takeStillCapturingButton =
        document.getElementById('take-still-capturing-button');
    this.takeSnapShotButton = document.getElementById('take-snapshot-button');
    this.recordingTimer = document.getElementById("recording-timer-display");
    this.recordingTimerIntervalId = null;
    this.startButton = document.getElementById('start-button');
    this.stopButton = document.getElementById('stop-button');
    this.cameraResolution = document.getElementById('camera-resolution');
    this.photoOutput = document.getElementById('photo-output');
    this.stream = null;
    this.constraints = {
      audio: false,
      video: {
        width: {exact: 640},
        height: {exact: 360},
        deviceId: null,
      },
      frameRate: 30,
    };
  }

  async connectCamera() {
    this.webLoader.style.display = 'flex';
    if (this.stream) {
      for (const track of this.stream.getTracks()) {
        track.stop();
      }
    }
    this.stream = await navigator.mediaDevices.getUserMedia(this.constraints);
    this.cameraImage.srcObject = this.stream;
    this.setCameraCapture();
    this.setMediaRecorder();
    const track = this.stream.getVideoTracks()[0];
    await track.applyConstraints(this.constraints);
    this.webLoader.style.display = 'none';
  }

  setCameraCapture() {
    const track = this.stream.getVideoTracks()[0];
    this.cameraCapture = new ImageCapture(track);
  }

  setMediaRecorder() {
    const options = { mimeType: 'video/mp4; codecs=vp9' };
    this.mediaRecorder = new MediaRecorder(this.stream, options);
    this.mediaRecorder.addEventListener(
        'dataavailable', (e) => this.dataAvailableHandler(e));
    this.mediaRecorder.addEventListener(
        'start', () => this.startRecordingTimer());
    this.mediaRecorder.addEventListener(
        'stop', () => this.stopRecordingTimer());
  }

  getCurrentTimeStamp() {
    const timeStamp = new Date();
    const padLeft = (str, len = 2, chr = `0`) => `${str}`.padStart(2, chr);
    const year = padLeft(timeStamp.getFullYear());
    const month = padLeft(timeStamp.getMonth() + 1);
    const date = padLeft(timeStamp.getDate());
    const hours = padLeft(timeStamp.getHours());
    const minutes = padLeft(timeStamp.getMinutes());
    const seconds = padLeft(timeStamp.getSeconds());
    return `${year}${month}${date}_${hours}${minutes}${seconds}`;
  }

  // Show the picture image on the webpage
  drawCanvas(canvas, img) {
    const ratio =
        Math.min(canvas.width / img.width, canvas.height / img.height);
    const x = (canvas.width - img.width * ratio) / 2;
    const y = (canvas.height - img.height * ratio) / 2;
    this.canvasContext = canvas.getContext('2d');
    this.canvasContext.fillStyle = 'black';
    this.canvasContext.fillRect(
        0, 0, canvas.width, canvas.height);
    this.canvasContext.drawImage(
        img, 0, 0, img.width, img.height, x, y, img.width * ratio,
        img.height * ratio);
  }

  async bitmapToBlob(img) {
    const canvas = document.createElement('canvas');
    canvas.width = img.width;
    canvas.height = img.height;
    const canvasContext = canvas.getContext('2d');
    canvasContext.fillStyle = 'black';
    canvasContext.fillRect(0, 0, canvas.width, canvas.height);
    canvasContext.drawImage(img, 0, 0, img.width, img.height);
    const blob = await new Promise((resolve, reject) => {
      canvas.toBlob(function(blob) {
        if (blob) {
          resolve(blob);
        } else {
          reject(new Error('Blob creation failed'));
        }
      }, 'image/jpeg', 1.0);
    });
    return blob;
  }

  async takeStillCapturingButtonEventHandler() {
    const caps = await this.cameraCapture.getPhotoCapabilities();
    const photoSettings = {
      imageWidth: caps.imageWidth.max,
      imageHeight: caps.imageHeight.max,
    };
    const blob = await this.cameraCapture.takePhoto(photoSettings);
    const img = await createImageBitmap(blob);
    this.drawCanvas(this.photoOutput, img);
    this.downloadBlob(blob, 'IMG_');
  }

  async takeSnapShotButtonEventHandler() {
    const img = await this.cameraCapture.grabFrame();
    this.drawCanvas(this.photoOutput, img);
    const blob = await this.bitmapToBlob(img);
    this.downloadBlob(blob, 'IMG_');
  }

  startButtonEventHandler() {
    this.videoOutput.style.display = 'none';
    URL.revokeObjectURL(this.videoOutput.src);
    this.mediaRecorder.start();
  }

  stopButtonEventHandler() {
    this.mediaRecorder.stop();
  }

  startRecordingTimer() {
    this.setRecordingTimerDisplay(0);
    const startTimestamp = performance.now();
    this.recordingTimerIntervalId = setInterval(() => {
      this.setRecordingTimerDisplay(performance.now() - startTimestamp);
    }, 100);
  }

  stopRecordingTimer() {
    clearInterval(this.recordingTimerIntervalId);
  }

  setRecordingTimerDisplay(milliseconds) {
    const seconds = Math.floor(milliseconds/1000);
    const formattedMinutes = Math.floor(seconds/60).toString().padStart(2, '0');
    const formattedSeconds = (seconds%60).toString().padStart(2, '0');
    const formattedTime = `${formattedMinutes}:${formattedSeconds}`;
    this.recordingTimer.textContent = formattedTime;
  }

  async changeCameraEventHandler(e) {
    this.constraints.video.deviceId = e.target.value;
    await this.connectCamera();
  }

  async changeFrameRateEventHandler() {
    this.constraints.frameRate = this.frameRateInput.value;
    await this.connectCamera();
  }

  async changeResolutionEventHandler(e) {
    const [width, height] = e.target.value.split(' ').map((x) => Number(x));
    this.constraints.video.height = {exact: height};
    this.constraints.video.width = {exact: width};
    await this.connectCamera();
  }

  // Show the video on the webpage
  dataAvailableHandler(e) {
    const blob = e.data;
    const dataUrl = URL.createObjectURL(blob);
    const recordingObj = this.videoOutput;
    this.downloadBlob(blob, 'VID_');
    recordingObj.src = dataUrl;
    recordingObj.load();
    recordingObj.style.display = 'inline';
  }

  // Automatically download taken blob
  downloadBlob(blob, prefix) {
    const blobUrl = window.URL.createObjectURL(blob);
    const fileName = prefix.concat(this.getCurrentTimeStamp());
    const downloadEvent = document.createElement('a');
    downloadEvent.href = blobUrl;
    downloadEvent.download = fileName;
    downloadEvent.style.display = 'none';
    document.body.appendChild(downloadEvent);
    downloadEvent.click();
    document.body.removeChild(downloadEvent);
    URL.revokeObjectURL(blobUrl);
  }

  inputFrameRateInputEventHandler() {
    this.frameRateOutout.value = this.frameRateInput.value;
  }

  async init() {
    await this.connectCamera();
    // List cameras and microphones.
    this.cameraSelector.innerHTML = '';
    const devices = await navigator.mediaDevices.enumerateDevices();
    for (const device of devices) {
      const option = document.createElement('option');
      if (device.kind === 'videoinput') {
        option.text =
            device.label || `camera ${this.cameraSelector.length + 1}`;
        option.value = device.deviceId;
        this.cameraSelector.appendChild(option);
      }
    }
  }

  async start() {
    this.takeStillCapturingButton.addEventListener(
      'click', () => this.takeStillCapturingButtonEventHandler());
  this.takeSnapShotButton.addEventListener(
      'click', () => this.takeSnapShotButtonEventHandler());
    this.startButton.addEventListener(
        'click', () => this.startButtonEventHandler());
    this.stopButton.addEventListener(
        'click', () => this.stopButtonEventHandler());
    this.cameraSelector.addEventListener(
        'change', (e) => this.changeCameraEventHandler(e));
    this.frameRateInput.addEventListener(
        'input', () => this.inputFrameRateInputEventHandler());
    this.frameRateInput.addEventListener(
        'change', () => this.changeFrameRateEventHandler());
    this.cameraResolution.addEventListener(
        'change', (e) => this.changeResolutionEventHandler(e));
    await this.init()
    this.initDone = true;
  }
}

window.app = new CameraApp();
window.app.start();
