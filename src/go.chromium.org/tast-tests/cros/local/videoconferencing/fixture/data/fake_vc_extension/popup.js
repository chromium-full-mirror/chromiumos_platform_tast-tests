document.addEventListener('DOMContentLoaded',  () => {
    document.getElementById("startVideoBtn").addEventListener("click", startVideo);
    document.getElementById("stopVideoBtn").addEventListener("click", stopVideo);
    document.getElementById("startAudioBtn").addEventListener("click", startAudio);
    document.getElementById("stopAudioBtn").addEventListener("click", stopAudio);
    document.getElementById("startScreenSharingBtn").addEventListener("click", startScreenSharing);
    document.getElementById("stopScreenSharingBtn").addEventListener("click", stopScreenSharing);

    var video_stream = null
    var audio_stream = null
    var screen_stream = null

    async function startVideo() {
        video_stream = await navigator.mediaDevices.getUserMedia({ video: { width: 320, height: 240 } })
        document.getElementById("startVideoBtn").disabled = true;
        document.getElementById('video_player').srcObject = video_stream;
        document.getElementById('video_player').play();
        document.getElementById("stopVideoBtn").disabled = false;
    }

    function stopVideo() {
        document.getElementById('video_player').pause();
        video_stream.getTracks().forEach((track) => {
            track.stop();
        });
        document.getElementById("stopVideoBtn").disabled = true;
        document.getElementById("startVideoBtn").disabled = false;
    }

    async function startAudio() {
        document.getElementById("startAudioBtn").disabled = true;
        audio_stream = await navigator.mediaDevices.getUserMedia({ audio: true })
        document.getElementById('audio_player').srcObject = audio_stream;
        document.getElementById("stopAudioBtn").disabled = false;
    }

    function stopAudio() {
        document.getElementById('audio_player').pause();
        audio_stream.getTracks().forEach((track) => {
            track.stop();
        });
        document.getElementById("stopAudioBtn").disabled = true;
        document.getElementById("startAudioBtn").disabled = false;
    }

    async function startScreenSharing() {
        document.getElementById("startScreenSharingBtn").disabled = true;
        screen_stream = await navigator.mediaDevices.getDisplayMedia()
        document.getElementById("stopScreenSharingBtn").disabled = false;
    }

    function stopScreenSharing() {
        screen_stream.getTracks().forEach((track) => {
            track.stop();
        });
        document.getElementById("stopScreenSharingBtn").disabled = true;
        document.getElementById("startScreenSharingBtn").disabled = false;
    }
}, false);
