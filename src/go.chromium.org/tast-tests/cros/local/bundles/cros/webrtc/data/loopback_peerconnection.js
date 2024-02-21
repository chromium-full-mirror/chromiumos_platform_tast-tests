// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// TestVisible holds JS variables to be queried by the tast test code.
class TestVisible {
  constructor() {
    this.localPeerConnections = [];
    this.remotePeerConnections = [];
  }
  setPeerConnections(localPCs, remotePCs) {
    this.localPeerConnections = localPCs;
    this.remotePeerConnections = remotePCs;
  }
}

let testVisible = new TestVisible;

async function getStream(width, height, displayMediaType) {
  let constraints = {
    audio: false,
    video: {
      width: width,
      height: height,
      framerate: 30,
    },
  };
  if (displayMediaType !== '') {
    constraints.video.displaySurface = displayMediaType;
    constraints.selfBrowserSurface = 'include';
  }

  let stream;
  if (constraints.video.displaySurface) {
    stream = await navigator.mediaDevices.getDisplayMedia(constraints);
    constraints.framerate = { min: 30, max: 30 };
    stream.getVideoTracks()[0].applyConstraints(constraints);
  } else {
    stream = await navigator.mediaDevices.getUserMedia(constraints);
  }
  return stream;
}

async function createStreamAndPCs(
  width,
  height,
  encodedInsertableStreams,
  displayMediaType
) {
  let localPC = new RTCPeerConnection({
    encodedInsertableStreams: encodedInsertableStreams,
  });
  let remotePC = new RTCPeerConnection({
    encodedInsertableStreams: encodedInsertableStreams,
  });

  return {
    stream: await getStream(width, height, displayMediaType),
    localPC: localPC,
    remotePC: remotePC,
  };
}

async function start(
  profile,
  width,
  height,
  simulcasts,
  svcScalabilityMode,
  displayMediaType
) {
  if (svcScalabilityMode !== '' && !svcScalabilityMode.startsWith('L1')) {
    console.log('unexpected svcScalabilityMode: ', svcScalabilityMode);
    return;
  }
  let { stream, localPC, remotePC } = await createStreamAndPCs(
    width,
    height,
    false,
    displayMediaType
  );

  let init = {
    // Prefer resolution even at the cost of visual quality to avoid falling
    // down to SW video encoding, see b/181320567 or crbug.com/1179020.
    degradationPreference: 'maintain-resolution',
    streams: [stream],
  };
  let rids = [];
  if (simulcasts > 1) {
    for (let i = 0; i < simulcasts; i++) {
      rids.push(i);
    }
    init.sendEncodings = rids.map((i) => {
      return { rid: i, scaleResolutionDownBy: 2 ** (rids.length - (i + 1)) };
    });
  }
  localPC.addTransceiver(stream.getVideoTracks()[0], init);
  remotePC.addTransceiver('video');

  const onTrack = new Promise((resolve, reject) => {
    remotePC.ontrack = (e) => {
      const remoteVideo = document.getElementById('remoteVideo0');
      remoteVideo.srcObject = e.streams[0];
      resolve();
    };
  });

  // |targetBitrate| uses a conservative 0.05 bits per pixel (bpp) estimate.
  const targetBitrate = width * height * 30 /*fps*/ * 0.05;
  await connect(localPC, remotePC, profile, targetBitrate, rids);
  testVisible.setPeerConnections([localPC], [remotePC]);
  await onTrack;
}

async function startSpatialSVC(profile, width, height, svcScalabilityMode) {
  if (svcScalabilityMode == '' || svcScalabilityMode.startsWith('L1')) {
    console.log('unexpected svcScalabilityMode: ', svcScalabilityMode);
    return;
  }
  let { stream, localPC, remotePC } = await createStreamAndPCs(
    width,
    height,
    true,
    ''
  );
  let localPCTransceiver = localPC.addTransceiver(stream.getVideoTracks()[0], {
    streams: [stream],
    sendEncodings: [{ scalabilityMode: svcScalabilityMode }],
  });
  // TODO(crbug.com/1513866): Remove this header extension setting.
  setUpHeaderExtension(localPCTransceiver);
  // Prefer resolution even at the cost of visual quality to avoid falling
  // down to SW video encoding, see b/181320567 or crbug.com/1179020.
  let params = localPCTransceiver.sender.getParameters();
  params.degradationPreference = 'maintain-resolution';
  await localPCTransceiver.sender.setParameters(params);

  let localPCStream = localPCTransceiver.sender.createEncodedStreams();

  const numStreams = parseInt(svcScalabilityMode[1]);

  let localPCs = new Array(numStreams);
  let remotePCs = new Array(numStreams);
  let remoteVideoIds = new Array(numStreams);
  let clonedLocalPCWriters = new Array(numStreams - 1);

  for (let i = 0; i < numStreams; i++) {
    remoteVideoIds[i] = 'remoteVideo' + i;
    if (i > 0) {
      let videoElement = document.createElement('video');
      videoElement.id = remoteVideoIds[i];
      videoElement.autoplay = true;
      videoElement.muted = true;
      document.getElementById('container').append(videoElement);
    }
  }

  for (let i = 0; i < numStreams - 1; i++) {
    const clonedLocalPC = new RTCPeerConnection({
      encodedInsertableStreams: true,
    });
    let clonedLocalPCTransceiver = clonedLocalPC.addTransceiver('video');
    // TODO(crbug.com/1513866): Remove this header extension setting.
    setUpHeaderExtension(clonedLocalPCTransceiver);
    let clonedLocalPCWriter = clonedLocalPCTransceiver.sender
      .createEncodedStreams()
      .writable.getWriter();
    localPCs[i] = clonedLocalPC;
    clonedLocalPCWriters[i] = clonedLocalPCWriter;
    remotePCs[i] = new RTCPeerConnection({ encodedInsertableStreams: true });
  }
  localPCs[numStreams - 1] = localPC;
  remotePCs[numStreams - 1] = remotePC;

  const topSpatialLayerIndex = parseInt(svcScalabilityMode[1]) - 1;
  let onTracks = new Array(numStreams);
  for (let i = 0; i < numStreams; i++) {
    const onTrack = new Promise((resolve, reject) => {
      remotePCs[i].ontrack = (e) => {
        let remoteVideo = document.getElementById('remoteVideo' + i);
        remoteVideo.srcObject = new MediaStream([e.track]);
        let receiver = e.receiver;
        let receiverStream = receiver.createEncodedStreams();
        // TODO(bugs.webrtc.org/15795): Set the marker bit of RTP Packet
        // and RemotePC[i] decodes frames whose spatial indices are i in S-mode
        // encoding.
        const decodeSpatialIndex = topSpatialLayerIndex;
        let transformInit;
        if (svcScalabilityMode.startsWith('S')) {
          transformInit = new TransformSmodeStream(
            svcScalabilityMode,
            decodeSpatialIndex
          );
        } else {
          transformInit = new TransformkSVCStream(
            svcScalabilityMode,
            decodeSpatialIndex
          );
        }

        receiverStream.readable
          .pipeThrough(new TransformStream(transformInit))
          .pipeTo(receiverStream.writable);
        resolve();
      };
    });
    onTracks[i] = onTrack;
  }

  let ssrcs = new Array(numStreams - 1);
  for (let i = 0; i < numStreams; i++) {
    const ssrc = await connect(localPCs[i], remotePCs[i], profile);
    if (i < ssrcs.length) {
      ssrcs[i] = ssrc;
    }
  }

  localPCStream.readable
    .pipeThrough(
      new TransformStream({
        transform(frame, controller) {
          const metadata = frame.getMetadata();
          for (let i = 0; i < ssrcs.length; i++) {
            const clonedFrame = structuredClone(frame);
            const modifiedMetadata = structuredClone(metadata);
            modifiedMetadata.synchronizationSource = ssrcs[i];
            clonedFrame.setMetadata(modifiedMetadata);
            clonedLocalPCWriters[i].write(clonedFrame);
          }
          controller.enqueue(frame);
        },
      })
    )
    .pipeTo(localPCStream.writable);

  testVisible.setPeerConnections(localPCs, remotePCs);

  for (let i = 0; i < onTracks.length; i++) {
    await onTracks[i];
  }
}
