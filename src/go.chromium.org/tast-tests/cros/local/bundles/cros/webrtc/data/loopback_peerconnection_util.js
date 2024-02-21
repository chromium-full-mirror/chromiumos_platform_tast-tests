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
// profile and bitrate.
async function connect(localPC, remotePC, codec, targetBitrate = 0, rids = []) {
  function findFirstCodec(name) {
    return RTCRtpReceiver.getCapabilities(name.split('/')[0]).codecs.filter(
      (c) =>
        c.mimeType.localeCompare(name, undefined, { sensitivity: 'base' }) === 0
    )[0];
  }

  localPC.onicecandidate = (e) => remotePC.addIceCandidate(e.candidate);
  remotePC.onicecandidate = (e) => localPC.addIceCandidate(e.candidate);

  const senders = localPC.getSenders();
  if (senders.length !== 1) {
    console.log('Unexpected senders length: ', senders);
    return;
  }

  let isValidCodec = false;
  const kValidCodecs = ['H264', 'VP8', 'VP9', 'AV1'];
  for (let i = 0; i < kValidCodecs.length; i++) {
    if (codec == kValidCodecs[i]) {
      isValidCodec = true;
      break;
    }
  }
  if (!isValidCodec) {
    console.log('Unexpected codec: ', codec);
    return;
  }

  let sender = senders[0];
  let params = sender.getParameters();
  params.degradationPreference = 'maintain-resolution';
  const rtcRTPCodec = findFirstCodec('video/' + codec);
  const numEncodings = Math.max(rids.length, 1);
  for (let i = 0; i < numEncodings; i++) {
    params.encodings[i].codec = rtcRTPCodec;
    if (targetBitrate !== 0) {
      params.encodings[i].maxBitrate = targetBitrate;
    }
    if (rids.length > 0 && params.encodings[i].rid !== i) {
      console.log(
        'encodings[' + i + ']: unexpected rid',
        params.encodings[i].rid
      );
    }
  }
  await sender.setParameters(params);

  const isSimulcast = rids.length > 0;
  let offer = await localPC.createOffer();
  if (isSimulcast) {
    await localPC.setLocalDescription(offer);
    await remotePC.setRemoteDescription({
      type: 'offer',
      sdp: swapRidAndMidExtensionsInSimulcastOffer(offer, rids),
    });
    const answer = await remotePC.createAnswer();
    await remotePC.setLocalDescription(answer);
    await localPC.setRemoteDescription({
      type: 'answer',
      sdp: swapRidAndMidExtensionsInSimulcastAnswer(
        answer,
        localPC.localDescription,
        rids
      ),
    });
  } else {
    await localPC.setLocalDescription(offer);
    await remotePC.setRemoteDescription(localPC.localDescription);
    await remotePC.setLocalDescription();
    await localPC.setRemoteDescription(remotePC.localDescription);
  }
  return obtainSsrcFromMid(localPC.localDescription, 0);
}

function setUpHeaderExtension(transceiver) {
  const DependencyDescriptorURI =
    'http://www.webrtc.org/experiments/rtp-hdrext/generic-frame-descriptor-00';
  let headerExtensions = transceiver.getHeaderExtensionsToNegotiate();
  headerExtensions = headerExtensions.map((ext) => {
    if (ext.uri == DependencyDescriptorURI) {
      ext.direction = 'sendrecv';
    }
    return ext;
  });
  transceiver.setHeaderExtensionsToNegotiate(headerExtensions);
}

// TODO(b/326387183): Unify the duplicated code in rtc.
// TransformSmodeStream extracts encoded video frames from the |scalabilityMode|
// (S-mode) stream and feeds into a video decoder so that the input stream is
// decodable S-mode stream whose spatial index is |decodeSpatialIndex|.
// If the stream is not a |scalabilityMode| stream, then it doesn't input any
// frames to a decoder.
//
// Caveat: WebRTC Encoded Transform API doesn't invoke transform() in decode
// order, but calls transform() as a frame is assembled from received packets.
// This implementation is not resilient to the reordered frames. Handling the
// frame dependency is troublesome so we don't implement it assuming the
// reordering seldom happens.
class TransformSmodeStream {
  constructor(scalabilityMode, decodeSpatialIndex) {
    if (!scalabilityMode.startsWith('S')) {
      console.log('Unexpected scalabilityMode: ', scalabilityMode);
      return;
    }
    this.decodeSpatialIndex = decodeSpatialIndex;
  }
  async transform(frame, controller) {
    const metadata = frame.getMetadata();
    if (metadata.spatialIndex == this.decodeSpatialIndex) {
      controller.enqueue(frame);
    }
  }
}

// TransformkSVCStream extracts encoded video frames from the |scalabilityMode|
// (k-SVC) stream and feeds into a video decoder so that the input stream is
// decodable k-SVC stream whose spatial index is |maxDecodeSpatialIndex|.
// Because of the bitrate adaptation in a WebRTC peer connection, the encoded
// video frames doesn't necessary compose a |scalabilityMode| stream. In this
// case, TransformkSVCStream inputs the current top spatial index layer.
//
// Caveat: WebRTC Encoded Transform API doesn't invoke transform() in decode
// order, but calls transform() as a frame is assembled from received packets.
// This implementation is not resilient to the reordered frames. Handling the
// frame dependency is troublesome so we don't implement it assuming the
// reordering seldom happens.
class TransformkSVCStream {
  constructor(scalabilityMode, maxDecodeSpatialIndex) {
    this.maxDecodeSpatialIndex = maxDecodeSpatialIndex;
    this.currentTopSpatialLayer = 0;
    this.decodeLowerLayers = false;
    if (!scalabilityMode.endsWith('KEY')) {
      console.log('Unexpected scalabilityMode: ', scalabilityMode);
    }
  }
  async transform(frame, controller) {
    const metadata = frame.getMetadata();
    if (metadata.spatialIndex > this.maxDecodeSpatialIndex) {
      // Ignore the upper spatial layer than one the transformer sends out.
      return;
    }

    const isKeyFrame = frame.type == 'key';
    if (isKeyFrame) {
      if (metadata.spatialIndex !== 0) {
        console.log('Keyframe is only in the bottom spatial layer');
        return;
      }
      // Start decoding lower spatial layers when the bottom layer is keyframe.
      this.decodeLowerLayers = true;
    } else if (metadata.spatialIndex == 0) {
      // Stop decoding lower spatial layers when we get the bottom layer.
      // This also stops updating |currentTopSpatialLayer|.
      this.decodeLowerLayers = false;
    }

    if (this.decodeLowerLayers) {
      this.currentTopSpatialLayer = metadata.spatialIndex;
    }
    const decode =
      metadata.spatialIndex == this.currentTopSpatialLayer ||
      this.decodeLowerLayers;
    if (decode) {
      controller.enqueue(frame);
    }
  }
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
