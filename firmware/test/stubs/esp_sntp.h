/*
 * Host stand-in for ESP-IDF's esp_sntp.h — the recording fake slice
 * firmware/main/ntp.c reaches (spaxel-db7aab09, esp_task_wdt.h precedent).
 *
 * The SNTP client itself needs a network stack and cannot exist on a host.
 * What ntp.c's contract is built on is the CALL ORDER around it: a running
 * SNTP client hard-asserts (aborts the node) if setoperatingmode() is called
 * before stop(), so every reconfigure path must issue esp_sntp_stop() first.
 * To make that order observable every API call appends a token to a sequence
 * log the tests assert on, alongside the recorded server name / opmode /
 * sync callback. sntp_mock_enabled() models the running flag a real client
 * would carry (init → running, stop → stopped).
 *
 * Static state, single-TU inclusion (ntp.c is compiled only into the test TU
 * that includes this header).
 */
#ifndef SPAXEL_TEST_STUB_ESP_SNTP_H
#define SPAXEL_TEST_STUB_ESP_SNTP_H

#include <stdbool.h>
#include <stddef.h>
#include <string.h>
#include <sys/time.h>

/* ---- Sequence tokens ---------------------------------------------------------- */

#define SNTP_MOCK_STOP       1
#define SNTP_MOCK_SETMODE    2
#define SNTP_MOCK_SETSERVER  3
#define SNTP_MOCK_SETCB      4
#define SNTP_MOCK_INIT       5

#define SNTP_MOCK_SEQ_MAX 64

__attribute__((unused))
static int sntp_mock_seq[SNTP_MOCK_SEQ_MAX];
__attribute__((unused))
static int sntp_mock_seq_len;

__attribute__((unused))
static void sntp_mock_record(int token)
{
    if (sntp_mock_seq_len < SNTP_MOCK_SEQ_MAX) {
        sntp_mock_seq[sntp_mock_seq_len++] = token;
    }
}

/* ---- Recorded calls ------------------------------------------------------------- */

typedef void (*sntp_sync_time_cb_t)(struct timeval *tv);

typedef enum { SNTP_OPMODE_POLL = 0, SNTP_OPMODE_LISTENONLY } sntp_opmode_t;

__attribute__((unused))
static int           sntp_mock_stop_calls;
__attribute__((unused))
static int           sntp_mock_setmode_calls;
__attribute__((unused))
static sntp_opmode_t sntp_mock_opmode;
__attribute__((unused))
static int           sntp_mock_setserver_calls;
__attribute__((unused))
static int           sntp_mock_server_idx;
__attribute__((unused))
static char          sntp_mock_server_name[64];
__attribute__((unused))
static int           sntp_mock_setcb_calls;
__attribute__((unused))
static sntp_sync_time_cb_t sntp_mock_time_cb;
__attribute__((unused))
static int           sntp_mock_init_calls;
__attribute__((unused))
static bool          sntp_mock_running;

/* ---- Reset + helpers ---------------------------------------------------------------- */

__attribute__((unused))
static void sntp_mock_reset_all(void)
{
    memset(sntp_mock_seq, 0, sizeof(sntp_mock_seq));
    sntp_mock_seq_len      = 0;
    sntp_mock_stop_calls   = 0;
    sntp_mock_setmode_calls = 0;
    sntp_mock_opmode       = SNTP_OPMODE_POLL;
    sntp_mock_setserver_calls = 0;
    sntp_mock_server_idx   = -1;
    memset(sntp_mock_server_name, 0, sizeof(sntp_mock_server_name));
    sntp_mock_setcb_calls  = 0;
    sntp_mock_time_cb      = NULL;
    sntp_mock_init_calls   = 0;
    sntp_mock_running      = false;
}

/* Deliver a time sync through the callback the production code registered. */
__attribute__((unused))
static void sntp_mock_fire_time_cb(struct timeval *tv)
{
    if (sntp_mock_time_cb) {
        sntp_mock_time_cb(tv);
    }
}

/* ---- The mocked API ------------------------------------------------------------------- */

__attribute__((unused))
static void esp_sntp_stop(void)
{
    sntp_mock_stop_calls++;
    sntp_mock_running = false;
    sntp_mock_record(SNTP_MOCK_STOP);
}

__attribute__((unused))
static void esp_sntp_setoperatingmode(sntp_opmode_t op_mode)
{
    sntp_mock_setmode_calls++;
    sntp_mock_opmode = op_mode;
    sntp_mock_record(SNTP_MOCK_SETMODE);
}

__attribute__((unused))
static void esp_sntp_setservername(int idx, const char *server)
{
    sntp_mock_setserver_calls++;
    sntp_mock_server_idx = idx;
    if (server) {
        strncpy(sntp_mock_server_name, server, sizeof(sntp_mock_server_name) - 1);
        sntp_mock_server_name[sizeof(sntp_mock_server_name) - 1] = '\0';
    } else {
        sntp_mock_server_name[0] = '\0';
    }
    sntp_mock_record(SNTP_MOCK_SETSERVER);
}

__attribute__((unused))
static void sntp_set_time_sync_notification_cb(sntp_sync_time_cb_t cb)
{
    sntp_mock_setcb_calls++;
    sntp_mock_time_cb = cb;
    sntp_mock_record(SNTP_MOCK_SETCB);
}

__attribute__((unused))
static void esp_sntp_init(void)
{
    sntp_mock_init_calls++;
    sntp_mock_running = true;
    sntp_mock_record(SNTP_MOCK_INIT);
}

#endif /* SPAXEL_TEST_STUB_ESP_SNTP_H */
