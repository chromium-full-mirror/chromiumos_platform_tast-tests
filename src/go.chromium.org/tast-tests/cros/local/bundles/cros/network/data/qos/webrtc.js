// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

'use strict';

let localPeerConnection;
let remotePeerConnection;

// The entry point of this WebRTC test app. We setup two peer connections on the
// same tab: `local` is the side collecting and sending the streams, and
// `remote` is the side receiving and displaying the streams. The main logic
// here is following the below WebRTC sample:
// - https://webrtc.github.io/samples/src/content/peerconnection/pc1/
//
// The main difference with the sample is that in this test we restrict and fake
// the ICE candidate can be used on both side.
// - The local side will be known as `localNatIp` on the remote side;
// - The remote side will be known as `remoteNatIp` on the local side.
async function start(localNatIp, remoteNatIp) {
  let localStream;
  const localVideo = document.getElementById('localVideo');

  try {
    const constraints = {audio: true, video: true};
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    localVideo.srcObject = stream;
    localStream = stream;
  } catch (e) {
    console.error(`getUserMedia() error: ${e.name}`);
    return;
  }

  const videoTracks = localStream.getVideoTracks();
  const audioTracks = localStream.getAudioTracks();
  if (videoTracks.length > 0) {
    console.log(`Using video device: ${videoTracks[0].label}`);
  }
  if (audioTracks.length > 0) {
    console.log(`Using audio device: ${audioTracks[0].label}`);
  }
  const configuration = {};
  localPeerConnection = new RTCPeerConnection(configuration);
  localPeerConnection.addEventListener(
      'icecandidate',
      e => onIceCandidate(localPeerConnection, localNatIp, e));
  remotePeerConnection = new RTCPeerConnection(configuration);
  remotePeerConnection.addEventListener(
      'icecandidate',
      e => onIceCandidate(remotePeerConnection, remoteNatIp, e));
  remotePeerConnection.addEventListener('track', gotRemoteStream);

  localStream.getTracks().forEach(
      track => localPeerConnection.addTrack(track, localStream));

  try {
    const offerOptions = {offerToReceiveAudio: 1, offerToReceiveVideo: 1};

    const offer = await localPeerConnection.createOffer(offerOptions);
    await localPeerConnection.setLocalDescription(offer);
    await remotePeerConnection.setRemoteDescription(offer);

    const answer = await remotePeerConnection.createAnswer();
    await remotePeerConnection.setLocalDescription(answer);
    await localPeerConnection.setRemoteDescription(answer);
  } catch (e) {
    console.error(`Failed to exchange session description: ${e.toString()}`);
  }
}

function gotRemoteStream(e) {
  const remoteVideo = document.getElementById('remoteVideo');
  if (remoteVideo.srcObject !== e.streams[0]) {
    console.log(`Got remote stream`);
    remoteVideo.srcObject = e.streams[0];
  }
}

function getName(pc) {
  return (pc === localPeerConnection) ? 'localPeerConnection' :
                                        'remotePeerConnection';
}

function getOtherPc(pc) {
  return (pc === localPeerConnection) ? remotePeerConnection :
                                        localPeerConnection;
}

async function onIceCandidate(pc, natIp, event) {
  try {
    let candidate = event.candidate;
    if (!candidate) {
      return;
    }

    const foundation = candidate.foundation;
    const port = event.candidate.port;
    const protocol = candidate.protocol;
    const priority = candidate.priority;
    const uFrag = event.candidate.usernameFragment;
    const sdpMid = event.candidate.sdpMid;
    const sdpMLineIndex = event.candidate.sdpMLineIndex;

    // We need to fake a `server reflexive` candidate. The address is always
    // reachable by the NAT-ed address.
    const type = 'srflx';

    // Only inject the candidate which is UDP. Ideally we also want to filter
    // the candidates by IP address, and convert only the candidates for the
    // interface we care about. This is not always doable because candidates
    // bound to local (private) addresses may not always be returned (they will
    // be returned if the user accepts the permission manually). One drawback of
    // the current solution is not we may inject the same fake candidate
    // multiple times, but this won't affect the connectivity.
    if (protocol !== 'udp') {
      return;
    }

    console.log(`onIceCandidate: ${candidate.candidate}`);

    candidate = new RTCIceCandidate({
      'candidate': `a=candidate:${foundation} 1 udp ${priority} ${natIp} ${
          port} typ ${type} ufrag ${uFrag}`,
      'sdpMid': sdpMid,
      'sdpMLineIndex': sdpMLineIndex,
    });

    await (getOtherPc(pc).addIceCandidate(candidate));

  } catch (e) {
    console.error(
        `${getName(pc)} failed to add ICE Candidate: ${error.toString()}`);
  }
}
