/*
 * ============================================================================
 *  Host test: CSI binary frame serialization roundtrip
 * ============================================================================
 *
 *  Covers the plan's Testing-Strategy requirement:
 *      `csi` — Binary frame serialization: verify frame header fields and
 *              little-endian encoding.
 *
 *  This is a gcc host test (see test_runner.h's header comment + the decision
 *  record docs/notes/firmware-host-test-approach.md, bead bf-21t, for why this
 *  is plain gcc and NOT ESP-IDF --target linux: firmware/main cannot be
 *  host-linked because csi.c → esp_wifi.h and provision.c → driver/uart.h, and
 *  the single `main` component REQUIRES esp_wifi/bt/driver which have no linux
 *  build). The harness therefore pins the wire-format CONTRACT rather than
 *  linking the firmware source.
 *
 *  The reference encoder below mirrors — byte for byte, offset for offset — the
 *  production serializer in firmware/main/websocket.c `websocket_send_csi()`
 *  (websocket.c lines 236-252):
 *
 *      frame_len = 24 + n_sub*2
 *      memcpy(frame+0,  node_mac, 6)
 *      memcpy(frame+6,  peer_mac, 6)
 *      memcpy(frame+12, &timestamp_us, 8)        // little-endian on Xtensa
 *      frame[20] = (uint8_t)rssi                 // int8 reinterpreted
 *      frame[21] = (uint8_t)noise_floor
 *      frame[22] = channel
 *      frame[23] = n_sub
 *      memcpy(frame+24, iq_data, n_sub*2)        // int8 I,Q pairs
 *
 *  The reference decoder mirrors the byte layout the Go ingestion server parses
 *  (plan §Ingestion "Binary CSI frame validation"). Round-tripping the two
 *  against each other is the cross-system contract guard: if the firmware ever
 *  changes the layout, the offset table here documents what every consumer must
 *  match, and the real end-to-end check lives in the Go spaxel-sim acceptance
 *  suite.
 *
 *  The ingestion-side validator (csi_validate) reproduces the plan's ordered
 *  validation rules so we can assert malformed frames are flagged at the right
 *  stage — connecting the firmware encoder contract to the mothership decoder
 *  contract, which is the real cross-system value of this test.
 * ============================================================================
 */
#include "test_runner.h"

#include <stdint.h>
#include <string.h>

/* ---- Wire-format constants (mirror firmware/main/spaxel.h) ---------------- */
#define CSI_HEADER_SIZE   24u   /* SPAXEL_FRAME_HEADER_SIZE */
#define CSI_MAX_SUB       128u  /* ingestion safety margin (ESP32-S3 ships 64) */
#define CSI_MIN_FRAME_LEN CSI_HEADER_SIZE

/* Offsets within the 24-byte header. */
#define OFF_NODE_MAC   0
#define OFF_PEER_MAC   6
#define OFF_TIMESTAMP  12
#define OFF_RSSI       20
#define OFF_NOISE      21
#define OFF_CHANNEL    22
#define OFF_N_SUB      23

/* Decoded view of a frame — what the mothership reads back. */
typedef struct {
    uint8_t  node_mac[6];
    uint8_t  peer_mac[6];
    uint64_t timestamp_us;
    int8_t   rssi;
    int8_t   noise_floor;
    uint8_t  channel;
    uint8_t  n_sub;
    const int8_t *iq; /* points into the frame buffer; valid only while it lives */
} csi_frame_view_t;

/* Ingestion-side validation result (plan §"Binary CSI frame validation"). */
typedef enum {
    CSI_OK,
    CSI_TOO_SHORT,       /* len < 24                              (rule 1) */
    CSI_PAYLOAD_MISMATCH,/* 24 + n_sub*2 != len                   (rule 3) */
    CSI_N_SUB_TOO_BIG,   /* n_sub > 128                           (rule 4) */
    CSI_BAD_CHANNEL,     /* channel == 0 or > 14                  (rules 6,7) */
} csi_valid_t;

/*
 * Reference encoder — same layout/offsets as websocket.c:websocket_send_csi().
 * `out` must point to at least CSI_HEADER_SIZE + n_sub*2 bytes. Returns the
 * number of bytes written. n_sub==0 produces a header-only probe (24 bytes),
 * which the plan explicitly allows.
 */
static size_t csi_encode(const uint8_t node_mac[6], const uint8_t peer_mac[6],
                         uint64_t timestamp_us, int8_t rssi, int8_t noise_floor,
                         uint8_t channel, uint8_t n_sub, const int8_t *iq,
                         uint8_t *out)
{
    size_t frame_len = CSI_HEADER_SIZE + (size_t)n_sub * 2u;

    memcpy(out + OFF_NODE_MAC,  node_mac, 6);
    memcpy(out + OFF_PEER_MAC,  peer_mac, 6);
    memcpy(out + OFF_TIMESTAMP, &timestamp_us, 8); /* LE on ESP32 + x86-64 gcc */
    out[OFF_RSSI]   = (uint8_t)rssi;
    out[OFF_NOISE]  = (uint8_t)noise_floor;
    out[OFF_CHANNEL] = channel;
    out[OFF_N_SUB]  = n_sub;
    if (n_sub > 0 && iq != NULL) {
        memcpy(out + CSI_HEADER_SIZE, iq, (size_t)n_sub * 2u);
    }
    return frame_len;
}

/* Reference decoder — reads back the byte layout the Go ingestion server sees. */
static void csi_decode(const uint8_t *frame, size_t len, csi_frame_view_t *v)
{
    /* Caller is expected to have validated len >= CSI_HEADER_SIZE first. */
    memcpy(v->node_mac, frame + OFF_NODE_MAC, 6);
    memcpy(v->peer_mac, frame + OFF_PEER_MAC, 6);
    memcpy(&v->timestamp_us, frame + OFF_TIMESTAMP, 8);
    v->rssi        = (int8_t)frame[OFF_RSSI];
    v->noise_floor = (int8_t)frame[OFF_NOISE];
    v->channel     = frame[OFF_CHANNEL];
    v->n_sub       = frame[OFF_N_SUB];
    v->iq          = (len > CSI_HEADER_SIZE)
                     ? (const int8_t *)(frame + CSI_HEADER_SIZE) : NULL;
}

/*
 * Ingestion-side validation, mirroring the plan's ordered rules exactly.
 * Order matters: a frame is dropped at the FIRST rule it violates.
 */
static csi_valid_t csi_validate(const uint8_t *frame, size_t len)
{
    if (len < CSI_MIN_FRAME_LEN) {        /* rule 1 */
        return CSI_TOO_SHORT;
    }
    uint8_t n_sub = frame[OFF_N_SUB];      /* rule 2 */
    if (CSI_HEADER_SIZE + (size_t)n_sub * 2u != len) { /* rule 3 */
        return CSI_PAYLOAD_MISMATCH;
    }
    if (n_sub > CSI_MAX_SUB) {             /* rule 4 */
        return CSI_N_SUB_TOO_BIG;
    }
    /* rule 5: rssi == 0 is allowed (invalid-RSSI flag), not a drop. */
    uint8_t channel = frame[OFF_CHANNEL];
    if (channel == 0 || channel > 14) {    /* rules 6, 7 */
        return CSI_BAD_CHANNEL;
    }
    return CSI_OK;
}

/* ---- Tests ---------------------------------------------------------------- */

/* A 64-subcarrier frame is 24 + 64*2 = 152 bytes; n_sub==0 is 24 (probe). */
TEST(csi_frame_header_size)
{
    uint8_t buf[CSI_HEADER_SIZE + CSI_MAX_SUB * 2u];
    uint8_t mac[6] = {0};
    uint8_t peer[6] = {0};

    ASSERT_EQ(csi_encode(mac, peer, 0, 0, 0, 6, 0, NULL, buf), 24);
    ASSERT_EQ(csi_encode(mac, peer, 0, 0, 0, 6, 64, NULL, buf), 152);
}

/* n_sub==0 is a valid header-only probe (plan: "n_sub=0 is valid"). */
TEST(csi_frame_header_only_probe)
{
    uint8_t buf[CSI_HEADER_SIZE];
    uint8_t mac[6] = {0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF};
    uint8_t peer[6] = {0x11, 0x22, 0x33, 0x44, 0x55, 0x66};

    size_t len = csi_encode(mac, peer, 42, -52, -95, 6, 0, NULL, buf);
    ASSERT_EQ(len, 24);
    ASSERT_EQ(csi_validate(buf, len), CSI_OK);

    csi_frame_view_t v;
    csi_decode(buf, len, &v);
    ASSERT_EQ(v.n_sub, 0);
    ASSERT_TRUE(v.iq == NULL);
}

/* Round-trip every header field through encode → decode. */
TEST(csi_frame_roundtrip_fields)
{
    uint8_t buf[CSI_HEADER_SIZE + 64 * 2u];
    uint8_t node[6] = {0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF};
    uint8_t peer[6] = {0x11, 0x22, 0x33, 0x44, 0x55, 0x66};
    int8_t  iq[64 * 2];
    for (int i = 0; i < 64 * 2; i++) {
        iq[i] = (int8_t)((i % 51) - 25); /* some negatives */
    }

    size_t len = csi_encode(node, peer, 0x1122334455667788ULL, -52, -95, 6, 64,
                            iq, buf);
    ASSERT_EQ(len, 152);

    csi_frame_view_t v;
    csi_decode(buf, len, &v);

    ASSERT_EQ(memcmp(v.node_mac, node, 6), 0);
    ASSERT_EQ(memcmp(v.peer_mac, peer, 6), 0);
    ASSERT_EQ(v.timestamp_us, 0x1122334455667788ULL);
    ASSERT_EQ(v.rssi, -52);
    ASSERT_EQ(v.noise_floor, -95);
    ASSERT_EQ(v.channel, 6);
    ASSERT_EQ(v.n_sub, 64);
    ASSERT_EQ(memcmp(v.iq, iq, 64 * 2), 0);
}

/*
 * Explicitly pin LITTLE-ENDIAN byte order of the 8-byte timestamp, independent
 * of host endianness. ts = 0x0102030405060708 must land as
 * bytes {08,07,06,05,04,03,02,01} at offset 12. This is the plan's "verify
 * little-endian encoding" requirement, made into a concrete byte assertion.
 */
TEST(csi_frame_timestamp_is_little_endian)
{
    uint8_t buf[CSI_HEADER_SIZE];
    uint8_t mac[6] = {0};
    uint8_t peer[6] = {0};

    csi_encode(mac, peer, 0x0102030405060708ULL, 0, 0, 6, 0, NULL, buf);

    ASSERT_EQ(buf[OFF_TIMESTAMP + 0], 0x08);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 1], 0x07);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 2], 0x06);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 3], 0x05);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 4], 0x04);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 5], 0x03);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 6], 0x02);
    ASSERT_EQ(buf[OFF_TIMESTAMP + 7], 0x01);

    /* And decoding reconstructs the original 64-bit value. */
    csi_frame_view_t v;
    csi_decode(buf, sizeof(buf), &v);
    ASSERT_EQ(v.timestamp_us, 0x0102030405060708ULL);
}

/*
 * RSSI / noise_floor are signed dBm carried as raw bytes. A negative value
 * (e.g. -52 dBm) must survive the (uint8_t) cast on encode and reinterpret as
 * int8 on decode. Validates the firmware's `frame[20] = (uint8_t)rssi` trick.
 */
TEST(csi_frame_signed_rssi_roundtrip)
{
    uint8_t buf[CSI_HEADER_SIZE];
    uint8_t mac[6] = {0};
    uint8_t peer[6] = {0};

    csi_encode(mac, peer, 0, -1, -128, 11, 0, NULL, buf);
    csi_frame_view_t v;
    csi_decode(buf, sizeof(buf), &v);
    ASSERT_EQ(v.rssi, -1);
    ASSERT_EQ(v.noise_floor, -128);

    csi_encode(mac, peer, 0, -52, -95, 1, 0, NULL, buf);
    csi_decode(buf, sizeof(buf), &v);
    ASSERT_EQ(v.rssi, -52);
    ASSERT_EQ(v.noise_floor, -95);
}

/* I/Q payload bytes are copied verbatim — verify a small known payload. */
TEST(csi_frame_iq_payload)
{
    uint8_t buf[CSI_HEADER_SIZE + 4 * 2u];
    uint8_t mac[6] = {0};
    uint8_t peer[6] = {0};
    int8_t  iq[8] = {10, -10, 20, -20, 30, -30, 40, -40};

    size_t len = csi_encode(mac, peer, 0, -40, -90, 6, 4, iq, buf);
    ASSERT_EQ(len, 32);

    csi_frame_view_t v;
    csi_decode(buf, len, &v);
    ASSERT_EQ(v.n_sub, 4);
    ASSERT_EQ(v.iq[0], 10);
    ASSERT_EQ(v.iq[1], -10);
    ASSERT_EQ(v.iq[2], 20);
    ASSERT_EQ(v.iq[3], -20);
    ASSERT_EQ(v.iq[4], 30);
    ASSERT_EQ(v.iq[5], -30);
    ASSERT_EQ(v.iq[6], 40);
    ASSERT_EQ(v.iq[7], -40);
}

/*
 * Ingestion-side validation: malformed frames are dropped at the right rule,
 * matching the plan's ordered checks. This ties the firmware encoder contract to
 * the mothership decoder contract.
 */
TEST(csi_frame_ingestion_validation)
{
    /* Zeroed up front: the first sub-test passes len=23, and although csi_validate
     * returns CSI_TOO_SHORT before reading any byte, zeroing removes any
     * -Wmaybe-uninitialized ambiguity under differing opt levels. */
    uint8_t buf[CSI_HEADER_SIZE + CSI_MAX_SUB * 2u];
    memset(buf, 0, sizeof(buf));
    uint8_t mac[6] = {0};
    uint8_t peer[6] = {0};

    /* Rule 1: too short to contain a header. */
    ASSERT_EQ(csi_validate(buf, 23), CSI_TOO_SHORT);

    /* Rule 3: payload length mismatch — 24-byte frame claims n_sub=5 (→34 B). */
    memset(buf, 0, sizeof(buf));
    csi_encode(mac, peer, 0, -50, -95, 6, 5, NULL, buf); /* claims 34 B */
    ASSERT_EQ(csi_validate(buf, 24), CSI_PAYLOAD_MISMATCH);

    /* Rule 4: n_sub > 128 with a length that otherwise matches. n_sub=130 → 284 B. */
    memset(buf, 0, sizeof(buf));
    buf[OFF_N_SUB] = 130;
    buf[OFF_CHANNEL] = 6;
    ASSERT_EQ(csi_validate(buf, 24 + 130u * 2u), CSI_N_SUB_TOO_BIG);

    /* Rule 6: channel == 0 is invalid. */
    memset(buf, 0, sizeof(buf)); /* n_sub=0, channel=0 */
    ASSERT_EQ(csi_validate(buf, 24), CSI_BAD_CHANNEL);

    /* Rule 7: channel > 14 is invalid. */
    memset(buf, 0, sizeof(buf));
    buf[OFF_CHANNEL] = 15;
    ASSERT_EQ(csi_validate(buf, 24), CSI_BAD_CHANNEL);

    /* Valid: channel 1..14, n_sub=0. rssi==0 is allowed (rule 5, not a drop). */
    memset(buf, 0, sizeof(buf));
    buf[OFF_CHANNEL] = 6;
    ASSERT_EQ(csi_validate(buf, 24), CSI_OK);

    /* Valid 64-subcarrier frame. */
    size_t len = csi_encode(mac, peer, 0, -52, -95, 11, 64, NULL, buf);
    ASSERT_EQ(csi_validate(buf, len), CSI_OK);
}
