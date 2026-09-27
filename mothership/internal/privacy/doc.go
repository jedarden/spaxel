// Package privacy documents and enforces the mothership's source-level
// privacy guarantees: no cloud relay and no media capture.
//
// # No cloud relay
//
// Spaxel is local-first: CSI ingestion, fusion, localization and the dashboard
// all run on the operator's LAN, and the mothership must never relay detection
// data to a cloud service. The mechanically checkable form of that guarantee
// is: the mothership binary only ever opens outbound network connections from
// packages whose egress is directed by operator configuration — notification
// channels, webhook integrations, the MQTT broker, connectivity diagnostics —
// and never from the detection path (ingestion, fusion, api, dashboard,
// recorder, tracking), which contains zero outbound call sites.
//
// egress_allowlist_test.go is a deterministic, offline source scan. It walks
// the module's cmd/ and internal/ trees, matches every Go outbound-dial
// construct (net.Dial family, http.Get/Post/Head/PostForm,
// http.NewRequest[WithContext], http.Client.Do, paho mqtt.NewClient), and
// fails if any hit falls outside an explicit, commented allowlist of packages
// whose egress is user-configured. Introducing a dial site anywhere else —
// in particular anywhere in the detection path — fails the test with a
// message naming the offending file and line.
//
// # No media capture
//
// Spaxel senses through WiFi CSI alone: there is no camera or microphone in
// the design, and no code path that could drive one. media_capture_prohibition_test.go
// enforces this on the two surfaces that physically could touch such
// hardware — the browser (the dashboard, whose only route to a device
// camera/mic is the W3C capture API family: getUserMedia, navigator.mediaDevices,
// getDisplayMedia, enumerateDevices, MediaRecorder, MediaStreamTrack,
// ImageCapture) and the ESP32-S3 sensor firmware (the esp32-camera driver
// and the I2S/PDM digital-microphone path). Both first-party trees are
// scanned and any hit fails with the offending file and line. Audio
// playback (AudioContext, used for audible alerts) is deliberately allowed —
// the guarantee bans capture, not sound output.
//
// # Enforcement
//
// Both guards are offline: they only read source files and never open a
// network connection. _test.go sources, build-ignored files (go:build
// ignore codegen helpers) and testdata/ are out of scope — they are not
// part of any shipped artifact. The full verified contract, including the
// live socket-sampling e2e test and the access-control model, is documented
// in docs/notes/network-boundary.md (with the PIN model in
// docs/notes/dashboard-pin-auth.md).
package privacy
