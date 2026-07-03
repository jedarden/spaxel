/*
 * ============================================================================
 *  Host test: serial provisioning JSON parser (with fuzz)
 * ============================================================================
 *
 *  Covers the plan's Testing-Strategy requirement:
 *      `serial_prov` — Provisioning JSON parser: verify valid JSON parsed
 *      correctly; invalid JSON returns {"ok":false}. (bead bf-31bp adds the
 *      fuzz pass: the parser must never crash on arbitrary UART input.)
 *
 *  gcc host test (see test_runner.h's header comment + decision record
 *  docs/notes/firmware-host-test-approach.md, bead bf-21t, for why this is
 *  plain gcc and NOT ESP-IDF --target linux: provision.c pulls in driver/uart.h
 *  and the `main` component REQUIRES esp_wifi/bt/driver, none of which have a
 *  linux build). The harness therefore mirrors the parser + protocol logic as
 *  a dependency-free extraction rather than linking the firmware source.
 *
 *  What is mirrored (decision-for-decision) from firmware/main/provision.c:
 *
 *    Protocol (provision_listen_window), per received line:
 *      cJSON_Parse(line) == NULL          → {"ok":false,"error":"invalid_json"}
 *      root has no "provision" member     → {"ok":false,"error":"missing_provision_key"}
 *      provision_write_nvs(prov) != ESP_OK→ {"ok":false,"error":"nvs_write_failed"}
 *      otherwise                          → {"ok":true,"mac":"<MAC>"}
 *
 *    Mapping (provision_write_nvs), JSON key → NVS key / type:
 *      wifi_ssid   string, NON-EMPTY, REQUIRED (else ESP_ERR_INVALID_ARG)
 *      wifi_pass   string  (optional)
 *      node_id     string  (optional)
 *      node_token  string  (optional)
 *      ms_mdns     string  (optional)
 *      ms_ip       string non-empty → writes BOTH ms_ip and ms_ip_prov
 *      ms_port     number > 0       → u16
 *      debug       bool             → u8 (cJSON_IsTrue ? 1 : 0)
 *      ntp_server  string  (optional)
 *      then unconditionally sets provisioned=1, schema_ver=NVS_SCHEMA_VERSION(=1)
 *
 *  cJSON is not vendored in the tree (it is the IDF `json` component), so this
 *  file ships a compact, BOUNDED JSON parser — j_*() below — sufficient for the
 *  provisioning object. It is the FUZZ TARGET: a UART line is untrusted,
 *  adversarial input, so the parser must be robust (no out-of-bounds, no
 *  unbounded recursion) and the protocol must always answer with a single,
 *  well-formed {"ok":...} line. The fuzz loop proves exactly that.
 *
 *  The real esp_ UART/NVS call sites remain validated on-target and via the Go
 *  spaxel-sim acceptance suite; this is the logic-and-robustness safety net.
 * ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- Status / classification (mirror esp_err_t subset + protocol result) - */
enum {
    PROV_OK = 0,            /* ESP_OK                            */
    PROV_ERR_INVALID_ARG,   /* ESP_ERR_INVALID_ARG — missing ssid */
};
typedef enum {
    CLASS_OK,
    CLASS_INVALID_JSON,
    CLASS_MISSING_KEY,
    CLASS_WRITE_FAILED,
} prov_class_t;

/* ---- In-memory string KV (mirrors the string-valued NVS writes) ---------- */
#define PSTR_MAX 32
#define PKEY_LEN 16
#define PVAL_LEN 128
typedef struct {
    char key[PKEY_LEN];
    char val[PVAL_LEN];
} pstr_t;
typedef struct { pstr_t rows[PSTR_MAX]; int count; } pstr_store_t;

static pstr_t *pstr_find(pstr_store_t *s, const char *k) {
    for (int i = 0; i < s->count; i++)
        if (strncmp(s->rows[i].key, k, PKEY_LEN) == 0) return &s->rows[i];
    return NULL;
}
static bool pstr_exists(pstr_store_t *s, const char *k) { return pstr_find(s, k) != NULL; }
static void pstr_set(pstr_store_t *s, const char *k, const char *v) {
    pstr_t *r = pstr_find(s, k);
    if (!r) {
        if (s->count >= PSTR_MAX) return;
        r = &s->rows[s->count++];
        strncpy(r->key, k, PKEY_LEN - 1); r->key[PKEY_LEN - 1] = '\0';
    }
    strncpy(r->val, v, PVAL_LEN - 1); r->val[PVAL_LEN - 1] = '\0';
}
static const char *pstr_get(pstr_store_t *s, const char *k) {
    pstr_t *r = pstr_find(s, k); return r ? r->val : NULL;
}

/* Provisioned device state: string keys + the typed u8/u16 slots provision.c writes. */
typedef struct {
    pstr_store_t str;
    uint8_t  debug;        bool debug_set;
    uint16_t ms_port;      bool ms_port_set;
    uint8_t  provisioned;
    uint8_t  schema_ver;
} prov_state_t;

static void prov_reset(prov_state_t *st) {
    memset(st, 0, sizeof(*st));
}

/* NVS key names (mirror spaxel.h NVS_KEY_*). */
#define K_SSID        "wifi_ssid"
#define K_PASS        "wifi_pass"
#define K_NODE_ID     "node_id"
#define K_TOKEN       "node_token"
#define K_MDNS        "ms_mdns"
#define K_MS_IP       "ms_ip"
#define K_MS_IP_PROV  "ms_ip_prov"
#define K_NTP         "ntp_server"

/* ========================================================================== */
/*  Bounded JSON parser (the fuzz target)                                     */
/* ========================================================================== */
/*
 * A small recursive-descent parser over the JSON grammar (object, array,
 * string, number, true/false/null). Every allocation comes from fixed-size
 * pools with hard caps, so NO input can cause unbounded memory use or
 * recursion:
 *   - at most J_MAX_NODES nodes,
 *   - at most J_ARENA bytes of string data,
 *   - at most J_MAX_DEPTH nesting.
 * Any violation (malformed token, overflow, excess depth) returns NULL up the
 * stack. Returned node trees point into parser-owned pools and are valid only
 * for the parser's lifetime.
 */
#define J_MAX_NODES 64
#define J_ARENA     4096
#define J_MAX_DEPTH 32

typedef enum { J_NULL, J_BOOL, J_NUM, J_STR, J_OBJ, J_ARR } jtype_t;

typedef struct jnode {
    jtype_t type;
    struct jnode *child;   /* first member (obj) / element (arr) */
    struct jnode *next;    /* next sibling */
    const char *name;      /* member name (obj members only); NULL otherwise */
    bool   b;
    double num;
    const char *str;       /* points into arena (J_STR) */
} jnode_t;

typedef struct {
    const char *src;
    size_t len, pos;
    int depth;
    jnode_t nodes[J_MAX_NODES];
    int node_count;
    char arena[J_ARENA];
    size_t arena_used;
} jparser_t;

static jnode_t *j_alloc(jparser_t *p) {
    if (p->node_count >= J_MAX_NODES) return NULL;
    jnode_t *n = &p->nodes[p->node_count++];
    memset(n, 0, sizeof(*n));
    return n;
}

static void j_skip_ws(jparser_t *p) {
    while (p->pos < p->len) {
        char c = p->src[p->pos];
        if (c == ' ' || c == '\t' || c == '\n' || c == '\r') p->pos++;
        else break;
    }
}

/* Copy a decoded string into the arena; returns arena pointer or NULL. */
static const char *j_parse_string(jparser_t *p) {
    if (p->pos >= p->len || p->src[p->pos] != '"') return NULL;
    p->pos++; /* opening quote */
    size_t start = p->arena_used;
    while (p->pos < p->len) {
        char c = p->src[p->pos++];
        if (c == '"') {
            if (p->arena_used >= J_ARENA) return NULL;
            p->arena[p->arena_used++] = '\0';
            return &p->arena[start];
        }
        if (c == '\\') {
            if (p->pos >= p->len) return NULL;
            char esc = p->src[p->pos++];
            char out = '\0';
            switch (esc) {
                case '"': case '\\': case '/': out = esc; break;
                case 'b': out = '\b'; break;
                case 'f': out = '\f'; break;
                case 'n': out = '\n'; break;
                case 'r': out = '\r'; break;
                case 't': out = '\t'; break;
                case 'u': {
                    /* \uXXXX — decode to one byte via the low byte; surrogate
                     * pairs are not valid for our ASCII NVS values, so a lone
                     * surrogate is accepted as its raw code unit low byte. The
                     * point is robustness, not canonical UTF-8. */
                    if (p->pos + 4 > p->len) return NULL;
                    unsigned int u = 0;
                    for (int i = 0; i < 4; i++) {
                        char h = p->src[p->pos + i];
                        u <<= 4;
                        if (h >= '0' && h <= '9') u |= (unsigned)(h - '0');
                        else if (h >= 'a' && h <= 'f') u |= (unsigned)(h - 'a' + 10);
                        else if (h >= 'A' && h <= 'F') u |= (unsigned)(h - 'A' + 10);
                        else return NULL;
                    }
                    p->pos += 4;
                    out = (char)(u & 0xFF);
                    break;
                }
                default: return NULL;   /* invalid escape */
            }
            c = out;
        } else if ((unsigned char)c < 0x20) {
            return NULL;   /* raw control char not allowed in JSON string */
        }
        if (p->arena_used >= J_ARENA) return NULL;
        p->arena[p->arena_used++] = c;
    }
    return NULL;   /* unterminated string */
}

static jnode_t *j_parse_value(jparser_t *p);   /* fwd */

static jnode_t *j_parse_array(jparser_t *p) {
    /* assumes p->src[p->pos] == '[' */
    p->pos++;
    jnode_t *arr = j_alloc(p);
    if (!arr) return NULL;
    arr->type = J_ARR;
    jnode_t *tail = NULL;
    j_skip_ws(p);
    if (p->pos < p->len && p->src[p->pos] == ']') { p->pos++; return arr; }
    for (;;) {
        jnode_t *v = j_parse_value(p);
        if (!v) return NULL;
        if (!arr->child) arr->child = v; else tail->next = v;
        tail = v;
        j_skip_ws(p);
        if (p->pos >= p->len) return NULL;
        char c = p->src[p->pos++];
        if (c == ']') return arr;
        if (c != ',') return NULL;
        j_skip_ws(p);
    }
}

static jnode_t *j_parse_object(jparser_t *p) {
    /* assumes p->src[p->pos] == '{' */
    p->pos++;
    jnode_t *obj = j_alloc(p);
    if (!obj) return NULL;
    obj->type = J_OBJ;
    jnode_t *tail = NULL;
    j_skip_ws(p);
    if (p->pos < p->len && p->src[p->pos] == '}') { p->pos++; return obj; }
    for (;;) {
        j_skip_ws(p);
        const char *name = j_parse_string(p);
        if (!name) return NULL;
        j_skip_ws(p);
        if (p->pos >= p->len || p->src[p->pos] != ':') return NULL;
        p->pos++;
        jnode_t *v = j_parse_value(p);
        if (!v) return NULL;
        v->name = name;
        if (!obj->child) obj->child = v; else tail->next = v;
        tail = v;
        j_skip_ws(p);
        if (p->pos >= p->len) return NULL;
        char c = p->src[p->pos++];
        if (c == '}') return obj;
        if (c != ',') return NULL;
    }
}

static jnode_t *j_parse_number(jparser_t *p) {
    size_t start = p->pos;
    if (p->pos < p->len && (p->src[p->pos] == '-' || p->src[p->pos] == '+')) p->pos++;
    bool any = false;
    while (p->pos < p->len) {
        char c = p->src[p->pos];
        if ((c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' ||
            c == '+' || c == '-') { p->pos++; any = true; }
        else break;
    }
    if (!any) return NULL;
    char buf[32];
    size_t n = p->pos - start;
    if (n >= sizeof(buf)) n = sizeof(buf) - 1;   /* truncate huge numbers */
    memcpy(buf, p->src + start, n);
    buf[n] = '\0';
    jnode_t *node = j_alloc(p);
    if (!node) return NULL;
    node->type = J_NUM;
    node->num = strtod(buf, NULL);
    return node;
}

static jnode_t *j_parse_value(jparser_t *p) {
    j_skip_ws(p);
    if (p->pos >= p->len) return NULL;
    if (p->depth >= J_MAX_DEPTH) return NULL;
    p->depth++;
    jnode_t *out = NULL;
    char c = p->src[p->pos];
    if (c == '{') out = j_parse_object(p);
    else if (c == '[') out = j_parse_array(p);
    else if (c == '"') {
        const char *s = j_parse_string(p);
        if (s) { out = j_alloc(p); if (out) { out->type = J_STR; out->str = s; } }
    } else if (c == '-' || c == '+' || (c >= '0' && c <= '9')) {
        out = j_parse_number(p);
    } else if (p->pos + 4 <= p->len && memcmp(p->src + p->pos, "true", 4) == 0) {
        p->pos += 4; out = j_alloc(p); if (out) { out->type = J_BOOL; out->b = true; }
    } else if (p->pos + 5 <= p->len && memcmp(p->src + p->pos, "false", 5) == 0) {
        p->pos += 5; out = j_alloc(p); if (out) { out->type = J_BOOL; out->b = false; }
    } else if (p->pos + 4 <= p->len && memcmp(p->src + p->pos, "null", 4) == 0) {
        p->pos += 4; out = j_alloc(p); if (out) { out->type = J_NULL; }
    }
    p->depth--;
    return out;
}

/*
 * Parse a full document into the CALLER-owned parser state `*p`. After the top
 * value, only trailing whitespace is allowed; anything else is malformed
 * (returns NULL). NULL on any error.
 *
 * The parser owns the node pool and string arena inside `*p`; the returned node
 * tree (and every node->str) points into it. The CALLER must keep `*p` alive for
 * as long as it holds the returned tree — hence `p` is passed in rather than
 * being a local here: a local would die with this stack frame and leave every
 * returned pointer dangling (use-after-return).
 */
static jnode_t *j_parse(jparser_t *p, const char *src, size_t len) {
    memset(p, 0, sizeof(*p));
    p->src = src; p->len = len;
    jnode_t *root = j_parse_value(p);
    if (!root) return NULL;
    j_skip_ws(p);
    if (p->pos != p->len) return NULL;   /* trailing garbage */
    return root;
}

/* Find a member by name in an object; NULL if not an object / not found. */
static jnode_t *j_get(jnode_t *obj, const char *key) {
    if (!obj || obj->type != J_OBJ) return NULL;
    for (jnode_t *c = obj->child; c; c = c->next)
        if (c->name && strcmp(c->name, key) == 0) return c;
    return NULL;
}

/* ========================================================================== */
/*  Mirror of provision_write_nvs (firmware/main/provision.c)                 */
/* ========================================================================== */
static int provision_write_nvs(jnode_t *prov, prov_state_t *st) {
    /* wifi_ssid is REQUIRED and must be a non-empty string. */
    jnode_t *ssid = j_get(prov, "wifi_ssid");
    if (!ssid || ssid->type != J_STR || ssid->str[0] == '\0') {
        return PROV_ERR_INVALID_ARG;
    }
    pstr_set(&st->str, K_SSID, ssid->str);

    jnode_t *pass = j_get(prov, "wifi_pass");
    if (pass && pass->type == J_STR) pstr_set(&st->str, K_PASS, pass->str);

    jnode_t *node_id = j_get(prov, "node_id");
    if (node_id && node_id->type == J_STR) pstr_set(&st->str, K_NODE_ID, node_id->str);

    jnode_t *token = j_get(prov, "node_token");
    if (token && token->type == J_STR) pstr_set(&st->str, K_TOKEN, token->str);

    jnode_t *mdns = j_get(prov, "ms_mdns");
    if (mdns && mdns->type == J_STR) pstr_set(&st->str, K_MDNS, mdns->str);

    jnode_t *ms_ip = j_get(prov, "ms_ip");
    if (ms_ip && ms_ip->type == J_STR && ms_ip->str[0] != '\0') {
        pstr_set(&st->str, K_MS_IP, ms_ip->str);
        pstr_set(&st->str, K_MS_IP_PROV, ms_ip->str);   /* mirrored to both keys */
    }

    jnode_t *port = j_get(prov, "ms_port");
    if (port && port->type == J_NUM && port->num > 0) {
        st->ms_port = (uint16_t)port->num;
        st->ms_port_set = true;
    }

    jnode_t *dbg = j_get(prov, "debug");
    if (dbg) {
        st->debug = (dbg->type == J_BOOL && dbg->b) ? 1 : 0;   /* cJSON_IsTrue */
        st->debug_set = true;
    }

    jnode_t *ntp = j_get(prov, "ntp_server");
    if (ntp && ntp->type == J_STR) pstr_set(&st->str, K_NTP, ntp->str);

    st->provisioned = 1;
    st->schema_ver = 1;   /* NVS_SCHEMA_VERSION */
    return PROV_OK;
}

/* ========================================================================== */
/*  Mirror of provision_listen_window's per-line decision                     */
/* ========================================================================== */
/*
 * Returns the protocol classification and writes the exact response line the
 * firmware would emit on UART into resp (always a single well-formed JSON
 * object terminated by '\n'). Mirrors provision.c's four branches.
 */
static prov_class_t provision_handle_line(const char *line, size_t len,
                                          const char *mac,
                                          prov_state_t *st,
                                          char *resp, size_t resp_cap)
{
    /*
     * The parser state lives on THIS frame so the returned node tree (and every
     * node->str into the arena) stays valid for the whole function — every
     * j_get / provision_write_nvs read below dereferences into `parser`.
     */
    jparser_t parser;
    jnode_t *root = j_parse(&parser, line, len);
    if (!root) {
        snprintf(resp, resp_cap, "{\"ok\":false,\"error\":\"invalid_json\"}\n");
        return CLASS_INVALID_JSON;
    }
    jnode_t *prov = j_get(root, "provision");
    if (!prov) {
        snprintf(resp, resp_cap, "{\"ok\":false,\"error\":\"missing_provision_key\"}\n");
        return CLASS_MISSING_KEY;
    }
    if (provision_write_nvs(prov, st) != PROV_OK) {
        snprintf(resp, resp_cap, "{\"ok\":false,\"error\":\"nvs_write_failed\"}\n");
        return CLASS_WRITE_FAILED;
    }
    snprintf(resp, resp_cap, "{\"ok\":true,\"mac\":\"%s\"}\n", mac);
    return CLASS_OK;
}

/* ========================================================================== */
/*  Tests                                                                      */
/* ========================================================================== */

static const char *TEST_MAC = "AA:BB:CC:DD:EE:FF";

/* A complete valid provisioning payload maps every field into NVS correctly. */
TEST(serial_prov_valid_full_payload)
{
    const char *line =
        "{\"provision\":{\"wifi_ssid\":\"HomeNet\",\"wifi_pass\":\"secret\","
        "\"node_id\":\"f47ac10b-58cf\",\"node_token\":\"a1b2c3d4\","
        "\"ms_mdns\":\"spaxel\",\"ms_ip\":\"192.168.1.5\",\"ms_port\":8080,"
        "\"debug\":true,\"ntp_server\":\"time.google.com\"}}";

    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));

    ASSERT_EQ(c, CLASS_OK);
    ASSERT_TRUE(strstr(resp, "\"ok\":true") != NULL);
    ASSERT_TRUE(strstr(resp, TEST_MAC) != NULL);

    ASSERT_EQ(strcmp(pstr_get(&st.str, K_SSID), "HomeNet"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_PASS), "secret"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_NODE_ID), "f47ac10b-58cf"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_TOKEN), "a1b2c3d4"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_MDNS), "spaxel"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_MS_IP), "192.168.1.5"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_MS_IP_PROV), "192.168.1.5"), 0);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_NTP), "time.google.com"), 0);
    ASSERT_EQ(st.ms_port, 8080);
    ASSERT_TRUE(st.ms_port_set);
    ASSERT_EQ(st.debug, 1);
    ASSERT_TRUE(st.debug_set);
    ASSERT_EQ(st.provisioned, 1);
    ASSERT_EQ(st.schema_ver, 1);
}

/* Missing wifi_ssid → nvs_write_failed (provision_write_nvs rejects it). */
TEST(serial_prov_missing_ssid_rejected)
{
    const char *line = "{\"provision\":{\"wifi_pass\":\"x\"}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));

    ASSERT_EQ(c, CLASS_WRITE_FAILED);
    ASSERT_TRUE(strstr(resp, "nvs_write_failed") != NULL);
    ASSERT_FALSE(pstr_exists(&st.str, K_PASS));  /* nothing written */
    ASSERT_EQ(st.provisioned, 0);                /* not provisioned on failure */
}

/* Empty wifi_ssid is also rejected (must be non-empty). */
TEST(serial_prov_empty_ssid_rejected)
{
    const char *line = "{\"provision\":{\"wifi_ssid\":\"\"}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_WRITE_FAILED);
    ASSERT_FALSE(pstr_exists(&st.str, K_SSID));
}

/* Optional fields absent: provisioning still succeeds with just the SSID. */
TEST(serial_prov_minimal_payload_ok)
{
    const char *line = "{\"provision\":{\"wifi_ssid\":\"Solo\"}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_OK);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_SSID), "Solo"), 0);
    ASSERT_FALSE(pstr_exists(&st.str, K_PASS));
    ASSERT_FALSE(st.ms_port_set);
    ASSERT_FALSE(st.debug_set);
    ASSERT_EQ(st.provisioned, 1);
}

/* Valid JSON but no "provision" wrapper key → missing_provision_key. */
TEST(serial_prov_missing_provision_key)
{
    const char *line = "{\"wifi_ssid\":\"HomeNet\"}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_MISSING_KEY);
    ASSERT_TRUE(strstr(resp, "missing_provision_key") != NULL);
}

/* Top-level non-object (array) has no members → missing_provision_key. */
TEST(serial_prov_top_level_array_is_missing_key)
{
    const char *line = "[1,2,3]";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_MISSING_KEY);
}

/* Garbage input → invalid_json, never crashes. */
TEST(serial_prov_invalid_json)
{
    const char *cases[] = {
        "",
        "not json",
        "{",
        "{unquoted}",
        "{\"provision\":}",
        "{\"a\":1,}",          /* trailing comma */
        "}{",
        "\xff\xfe garbage",
    };
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        prov_state_t st; prov_reset(&st);
        char resp[128];
        prov_class_t c = provision_handle_line(cases[i], strlen(cases[i]),
                                               TEST_MAC, &st, resp, sizeof(resp));
        ASSERT_EQ(c, CLASS_INVALID_JSON);
        ASSERT_TRUE(strstr(resp, "invalid_json") != NULL);
    }
}

/* A debug value that is present but not a bool writes 0 (cJSON_IsTrue==false). */
TEST(serial_prov_debug_non_bool_writes_zero)
{
    const char *line = "{\"provision\":{\"wifi_ssid\":\"H\",\"debug\":\"yes\"}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_OK);
    ASSERT_TRUE(st.debug_set);
    ASSERT_EQ(st.debug, 0);
}

/* debug:false explicitly writes 0; ms_port given as a string is ignored. */
TEST(serial_prov_port_wrong_type_ignored)
{
    const char *line =
        "{\"provision\":{\"wifi_ssid\":\"H\",\"ms_port\":\"8080\",\"debug\":false}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_FALSE(st.ms_port_set);   /* string, not number → ignored */
    ASSERT_TRUE(st.debug_set);
    ASSERT_EQ(st.debug, 0);
}

/*
 * String escapes round-trip: a SSID with quotes/backslashes/control escapes
 * is decoded into the stored value exactly as the firmware's cJSON would.
 */
TEST(serial_prov_string_escapes_decoded)
{
    const char *line = "{\"provision\":{\"wifi_ssid\":\"a\\\"b\\\\c\\nd\"}}";
    prov_state_t st; prov_reset(&st);
    char resp[128];
    prov_class_t c = provision_handle_line(line, strlen(line), TEST_MAC, &st, resp, sizeof(resp));
    ASSERT_EQ(c, CLASS_OK);
    ASSERT_EQ(strcmp(pstr_get(&st.str, K_SSID), "a\"b\\c\nd"), 0);
}

/* ========================================================================== */
/*  Fuzz: the parser must never crash on arbitrary UART input, and the        */
/*  protocol must always answer with a single well-formed {"ok":...} line.    */
/* ========================================================================== */

/* Deterministic LCG (no reliance on libc rand state / seed). */
static uint32_t fuzz_lcg(uint32_t *s) {
    *s = (*s * 1103515245u + 12345u) & 0x7fffffffu;
    return *s;
}

/*
 * Validate that a response line is a single, complete JSON object: starts with
 * '{"ok":', contains no embedded newline, and ends with "}\n". This is the
 * robustness contract — a malformed UART line must never yield a half-framed
 * response that could desync the host's line reader.
 */
static bool resp_is_well_formed(const char *resp) {
    if (resp[0] != '{') return false;
    if (strstr(resp, "\"ok\":") == NULL) return false;
    size_t n = strlen(resp);
    if (n < 4) return false;
    if (resp[n - 1] != '\n' || resp[n - 2] != '}') return false;
    for (size_t i = 0; i + 1 < n; i++)
        if (resp[i] == '\n') return false;   /* no embedded newlines */
    return true;
}

TEST(serial_prov_fuzz_random_bytes_never_crash)
{
    static const char *corpus[] = {
        "{", "}", "[]", "[[[[[[[[[[[", "{\"provision\":{\"wifi_ssid\":",
        "{\"a\":" , "{\"a\":null}", "null", "true", "false", "1234567890",
        "\"unterminated", "{\"provision\":{\"wifi_ssid\":\"\\u00",
        "{\"provision\":{\"wifi_ssid\":\"x\",\"extra\":" ,
        "\xef\xbb\xbf{\"provision\":{\"wifi_ssid\":\"bom\"}}",   /* UTF-8 BOM */
        "{\"provision\"  :  {  \"wifi_ssid\"  :  \"ws\"  }  }",  /* ws tolerance */
    };
    uint32_t s = 0xC0FFEEu;   /* fixed seed → reproducible */
    unsigned char buf[300];

    /* Random byte streams of varied length. */
    for (int iter = 0; iter < 4000; iter++) {
        size_t len = fuzz_lcg(&s) % (sizeof(buf));
        for (size_t i = 0; i < len; i++) buf[i] = (unsigned char)(fuzz_lcg(&s) & 0xFF);

        prov_state_t st; prov_reset(&st);
        char resp[160];
        prov_class_t c = provision_handle_line((const char *)buf, len, TEST_MAC,
                                               &st, resp, sizeof(resp));
        (void)c;   /* any class is fine — the contract is robustness */
        ASSERT_TRUE(resp_is_well_formed(resp));
    }

    /* Fixed corpus of tricky / malformed inputs. */
    for (size_t i = 0; i < sizeof(corpus) / sizeof(corpus[0]); i++) {
        prov_state_t st; prov_reset(&st);
        char resp[160];
        provision_handle_line(corpus[i], strlen(corpus[i]), TEST_MAC,
                              &st, resp, sizeof(resp));
        ASSERT_TRUE(resp_is_well_formed(resp));
    }
}

/*
 * Deep-nesting stress: the parser's depth cap must reject pathological input
 * without unbounded recursion (which would overflow the stack). Each input is
 * a wall of opening braces/brackets.
 */
TEST(serial_prov_fuzz_deep_nesting_capped)
{
    char deep[2048];
    memset(deep, '{', sizeof(deep) - 1);
    deep[sizeof(deep) - 1] = '\0';

    uint32_t s = 1;
    for (int iter = 0; iter < 500; iter++) {
        /* Mix of '{', '[', '"', and random bytes to stress the depth path. */
        size_t len = 64 + (fuzz_lcg(&s) % (sizeof(deep) - 65));
        for (size_t i = 0; i < len; i++) {
            uint32_t r = fuzz_lcg(&s) & 3;
            deep[i] = (r == 0) ? '{' : (r == 1) ? '[' : (r == 2) ? '"' : (char)(fuzz_lcg(&s) & 0x7F);
        }
        deep[len] = '\0';

        prov_state_t st; prov_reset(&st);
        char resp[160];
        provision_handle_line(deep, len, TEST_MAC, &st, resp, sizeof(resp));
        ASSERT_TRUE(resp_is_well_formed(resp));
    }
}
