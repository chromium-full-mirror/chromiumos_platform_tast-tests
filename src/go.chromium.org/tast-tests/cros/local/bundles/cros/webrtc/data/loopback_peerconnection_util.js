// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Transforms the "container" <div> that holds the real time <video> into a
// |dimension| x |dimension| grid, and fills it with |videoURL| <video>s.
// Reusing the same URL being played back should not affect the test since each
// <video> will need to decode and play independently from the others.
function makeVideoGrid(dimension, videoURL) {
  // Find the |container| and make it a |dimension| x |dimension| grid; repeat()
  // allows for automatically ordering sub-grids into |dimension| columns, see
  // https://developer.mozilla.org/en-US/docs/Web/CSS/grid-template-columns
  const container = document.getElementById('container');
  container.style.display = 'grid';
  container.style.gridTemplateColumns = 'repeat(' + dimension + ', 1fr)';

  // Fill the grid with <video>s. Note that there is already one <video> in the
  // grid for the remote RTCPeerConnection stream feed.
  const numExtraVideosInGrid = dimension * dimension - 1;
  for (let i = 0; i < numExtraVideosInGrid; i++) {
    const video = document.createElement('video');
    video.src = videoURL;
    video.style.maxWidth = '100%';
    video.autoplay = true;
    video.muted = true;
    video.loop = true;
    const div = document.createElement('div');
    div.appendChild(video);
    container.appendChild(div);
  }
}

// Appends a 'a=fmtp:bla x-google-start-bitrate=foo' statement to |sdp|, where
// 'bla' is the SDP id for |profile| and 'foo' is the |startBitrate| in Kbps;
// this statement is needed to prevent RTCPeerConnections from dropping
// resolution to keep a by-default low start bitrate.
function appendStartBitrateToSDP(sdp, profile, startBitrate) {
  const codec_id = findRtpmapId(splitSdpLines(sdp), profile);
  if (codec_id) {
    const targetBitrateKbpsAsInt = Math.trunc(startBitrate / 1000);
    sdp +=
      `a=fmtp:${codec_id} ` +
      `x-google-start-bitrate=${targetBitrateKbpsAsInt}\r\n`;
  }
  return sdp;
}

// Get the synchronization source from SDP.
function obtainSsrcFromMid(description) {
  const lines = description.sdp.split('\r\n');
  let midFound = false;
  for (let i = 0; i < lines.length; ++i) {
    const line = lines[i];
    if (line.startsWith('a=mid:0')) {
      midFound = true;
    }
    if (midFound && line.startsWith('a=ssrc:')) {
      const spaceIndex = line.indexOf(' ');
      const ssrc = line.substr(7, spaceIndex - 7);
      return ssrc;
    }
  }
  return null;
}

// Establish a peer connection between localPC and remotePC with the given
// profile and bitrate. If rids is not empty, the peer connection has
// multiple streams (i.e. simulcast).
async function connect(localPC, remotePC, profile, targetBitrate, rids) {
  localPC.onicecandidate = (e) => remotePC.addIceCandidate(e.candidate);
  remotePC.onicecandidate = (e) => localPC.addIceCandidate(e.candidate);

  const isSimulcast = rids.length > 0;
  let offer = await localPC.createOffer();
  if (isSimulcast) {
    await localPC.setLocalDescription(offer);
    await remotePC.setRemoteDescription({
      type: 'offer',
      sdp: swapRidAndMidExtensionsInSimulcastOffer(offer, rids),
    });
  } else {
    offer.sdp = setSdpDefaultVideoCodec(offer.sdp, profile);
    await localPC.setLocalDescription(offer);
    await remotePC.setRemoteDescription(localPC.localDescription);
  }

  const answer = await remotePC.createAnswer();
  if (targetBitrate > 0) {
    answer.sdp = appendStartBitrateToSDP(answer.sdp, profile, targetBitrate);
  } else {
    answer.sdp = setSdpDefaultVideoCodec(answer.sdp, profile);
  }
  await remotePC.setLocalDescription(answer);
  if (isSimulcast) {
    await localPC.setRemoteDescription({
      type: 'answer',
      sdp: swapRidAndMidExtensionsInSimulcastAnswer(
        answer,
        localPC.localDescription,
        rids
      ),
    });
  } else {
    await localPC.setRemoteDescription(remotePC.localDescription);
  }

  return obtainSsrcFromMid(localPC.localDescription, 0);
}

// Returns true if the video frame being displayed is considered "black".
// Specifying |width| or |height| smaller than the feeding |remoteVideo| can be
// used for speeding up the calculation by downscaling.
function isBlackVideoFrame(width, height, id = 0) {
  const context = new OffscreenCanvas(width, height).getContext('2d');

  const remoteVideo = document.getElementById('remoteVideo' + id);
  context.drawImage(remoteVideo, 0, 0, width, height);
  const imageData = context.getImageData(0, 0, width, height);
  return isBlackFrame(imageData.data, imageData.data.length);
}

const IDENTICAL_FRAME_SSIM_THRESHOLD = 0.99;
// Returns true if the previous video frame is too similar to the current video
// frame, implying that the video feed is frozen. The similarity is calculated
// using ssim() and comparing with the IDENTICAL_FRAME_SSIM_THRESHOLD.
// Specifying |width| or |height| smaller than the feeding |remoteVideo| can be
// used for speeding up the calculation by downscaling.
function isFrozenVideoFrame(width, height, id = 0) {
  const context = new OffscreenCanvas(width, height).getContext('2d');

  const remoteVideo = document.getElementById('remoteVideo' + id);
  context.drawImage(remoteVideo, 0, 0, width, height);
  const imageData = context.getImageData(0, 0, width, height);

  if (isFrozenVideoFrame.previousImageData == null) {
    isFrozenVideoFrame.previousImageData = imageData;
    return false;
  }

  const ssim = new Ssim();
  const ssimValue = ssim.calculate(
    imageData.data,
    isFrozenVideoFrame.previousImageData.data
  );
  isFrozenVideoFrame.previousImageData = imageData;
  return ssimValue > IDENTICAL_FRAME_SSIM_THRESHOLD;
}
