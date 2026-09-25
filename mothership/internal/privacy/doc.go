// Package privacy documents and enforces the mothership's no-cloud-relay
// guarantee at the source level.
//
// # The guarantee
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
// # Enforcement
//
// egress_allowlist_test.go in this package is a deterministic, offline source
// scan. It walks the module's cmd/ and internal/ trees, matches every Go
// outbound-dial construct (net.Dial family, http.Get/Post/Head/PostForm,
// http.NewRequest[WithContext], http.Client.Do, paho mqtt.NewClient), and
// fails if any hit falls outside an explicit, commented allowlist of packages
// whose egress is user-configured. Introducing a dial site anywhere else —
// in particular anywhere in the detection path — fails the test with a
// message naming the offending file and line.
//
// The test never opens a network connection itself: it only reads module
// source files. Build-ignored files (go:build ignore codegen helpers) and
// _test.go sources are out of scope — they are not part of the shipped
// binary.
package privacy
