// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

let recordedBlobs;
let recordFinished = false;

function handleDataAvailable(event) {
  console.log('handleDataAvailable', event);
  if (event.data && event.data.size > 0) {
    recordedBlobs.push(event.data);
  }
}

async function recordAudio() {
  // No echo cancellation for recording from loopback.
  let stream = await navigator.mediaDevices.getUserMedia(
      {audio: {'echoCancellation': false}, video: false});
  recordedBlobs = [];
  console.log(stream);

  mediaRecorder =
      new MediaRecorder(stream, {mimeType: 'audio/webm;codecs=pcm'});
  mediaRecorder.ondataavailable = handleDataAvailable;

  mediaRecorder.start();
  setTimeout((event) => {
    console.log('stop recording');
    mediaRecorder.stop();
    recordFinished = true;
  }, 5000);
}

function download() {
  var blob = new Blob(recordedBlobs, {type: 'audio/webm;codecs=pcm'});
  console.log(blob);
  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  document.body.appendChild(a);
  a.style = 'display: none';
  a.href = url;
  a.download = 'test.webm';
  a.click();
  window.URL.revokeObjectURL(url);
}
