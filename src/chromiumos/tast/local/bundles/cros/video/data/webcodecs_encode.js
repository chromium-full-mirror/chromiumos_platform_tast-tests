// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

'use strict';

let bitstreamSaver = new BitstreamSaver();

let encoderInputFrames = [];

let encoders = [];

async function DecodeFrames(videoURL, numFrames) {
  encoderInputFrames = await decodeVideoInURL(videoURL, numFrames);
  TEST.expect(encoderInputFrames.length == numFrames,
              'Decode frames mismatch: ' + encoderInputFrames.length);
  return encoderInputFrames.length == numFrames;
}

async function EncodeAndSave(codec, acceleration, width, height, bitrate,
                             framerate, scalabilityMode, bitrateMode,
                             numEncoders) {
  if (scalabilityMode === "") {
    scalabilityMode = undefined;
  }

  for (let i = 0; i < numEncoders; i++) {
    let encoder = await CreateEncoder(codec, acceleration, width, height,
                                      bitrate, framerate, bitstreamSaver,
                                      scalabilityMode, bitrateMode);
    if (!encoder) {
      TEST.failExit();
      return;
    }

    encoders.push(encoder);
  }

  for (const frame of encoderInputFrames) {
    console.assert(frame, "null frame");
    for (const encoder of encoders) {
      // A value of false indicates that the User Agent has flexibility to
      // decide whether the frame will be encoded as a key frame.
      encoder.encode(frame, { keyFrame: false });
    }
    frame.close();
  }

  for (const encoder of encoders) {
    await encoder.flush();
  }
}

async function CloseEncoders() {
  for (const encoder of encoders) {
    await encoder.close();
  }

  TEST.expect(
    TEST.numEncodedFrames == encoderInputFrames.length * encoders.length,
    'Encode frames mismatch: ' + TEST.numEncodedFrames);
  TEST.expect(
    TEST.encoderError == 0,
    'Encoding errors occurred during the test');
  TEST.exit();
}
