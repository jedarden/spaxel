/*
 * Host stand-in for cJSON (spaxel-db7aab09).
 *
 * ble.c and websocket.c speak JSON to the mothership on every connection —
 * hello/health/ble up, role/config/ota/reboot down. A stub that only fakes
 * the object-building half would leave every DOWNLINK path (the
 * websocket_handle_message dispatch, the config parser) untestable, so this
 * shim implements the real thing at host scale: a recursive-descent parser
 * (objects, arrays, strings with escapes, numbers, true/false/null), a
 * tree walker for GetObjectItem/IsString/IsNumber, and an unformatted
 * serializer whose result is a plain malloc'd string — exactly the contract
 * the production code assumes (cJSON_PrintUnformatted's result is freed with
 * free(), not a cJSON-owned allocator).
 *
 * Everything is static: each production TU sees the private copy of the shim
 * that its own test TU expanded, so nothing collides at link time and each
 * TU's tree lives in its own memory. Numbers keep valueint (truncated) and
 * valuedouble like the real library; serializer emits integral doubles
 * without a decimal point, matching the wire format the mothership parses.
 */
#ifndef SPAXEL_TEST_STUB_CJSON_H
#define SPAXEL_TEST_STUB_CJSON_H

#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct cJSON {
    struct cJSON *next;
    struct cJSON *prev;
    struct cJSON *child;

    int type;

    char *valuestring;
    int   valueint;
    double valuedouble;

    char *string; /* key, when this node is a member of an object */
} cJSON;

/* Same bit values as the real library's public enum. */
#define cJSON_Invalid  0
#define cJSON_False   (1 << 0)
#define cJSON_True    (1 << 1)
#define cJSON_NULL    (1 << 2)
#define cJSON_Number  (1 << 3)
#define cJSON_String  (1 << 4)
#define cJSON_Array   (1 << 5)
#define cJSON_Object  (1 << 6)

typedef int cJSON_bool;

/* ---- Node plumbing ------------------------------------------------------------ */

__attribute__((unused))
static cJSON *cj_new(int type)
{
    cJSON *node = (cJSON *)calloc(1, sizeof(cJSON));
    if (node) {
        node->type = type;
    }
    return node;
}

__attribute__((unused))
static void cJSON_Delete(cJSON *item)
{
    while (item) {
        cJSON *next = item->next;
        if (item->child) {
            cJSON_Delete(item->child);
        }
        free(item->valuestring);
        free(item->string);
        free(item);
        item = next;
    }
}

/* Append to the child list (walking the tail — trees here are tiny). */
__attribute__((unused))
static void cJSON_AddItemToArray(cJSON *array, cJSON *item)
{
    if (!array || !item) {
        cJSON_Delete(item);
        return;
    }
    cJSON *tail = array->child;
    if (!tail) {
        array->child = item;
        return;
    }
    while (tail->next) {
        tail = tail->next;
    }
    tail->next = item;
    item->prev = tail;
}

/* ---- Builders the production code calls ------------------------------------------- */

/* strdup() is POSIX and hides under -std=c11; a local copy keeps the shim
 * independent of feature-test macro ordering in every including TU. */
static char *cj_strdup(const char *s)
{
    size_t n  = strlen(s) + 1;
    char *out = (char *)malloc(n);
    if (out) {
        memcpy(out, s, n);
    }
    return out;
}

__attribute__((unused))
static cJSON *cJSON_CreateObject(void) { return cj_new(cJSON_Object); }

__attribute__((unused))
static cJSON *cJSON_CreateArray(void) { return cj_new(cJSON_Array); }

__attribute__((unused))
static cJSON *cJSON_CreateString(const char *value)
{
    cJSON *node = cj_new(cJSON_String);
    if (node && value) {
        node->valuestring = cj_strdup(value);
        if (!node->valuestring) {
            free(node);
            node = NULL;
        }
    }
    return node;
}

static cJSON *cj_add_to_object(cJSON *object, const char *key, cJSON *item)
{
    if (!object || !item) {
        cJSON_Delete(item);
        return NULL;
    }
    item->string = cj_strdup(key ? key : "");
    if (!item->string) {
        cJSON_Delete(item);
        return NULL;
    }
    cJSON_AddItemToArray(object, item);
    return item;
}

__attribute__((unused))
static cJSON *cJSON_AddStringToObject(cJSON *object, const char *key,
                                      const char *value)
{
    cJSON *item = cJSON_CreateString(value);
    return cj_add_to_object(object, key, item);
}

__attribute__((unused))
static cJSON *cJSON_AddNumberToObject(cJSON *object, const char *key,
                                      double num)
{
    cJSON *node = cj_new(cJSON_Number);
    if (node) {
        node->valuedouble = num;
        node->valueint    = (int)num;
    }
    return cj_add_to_object(object, key, node);
}

__attribute__((unused))
static cJSON *cJSON_AddBoolToObject(cJSON *object, const char *key,
                                    cJSON_bool value)
{
    cJSON *node = cj_new(value ? cJSON_True : cJSON_False);
    return cj_add_to_object(object, key, node);
}

/* ---- Queries -------------------------------------------------------------------- */

__attribute__((unused))
static cJSON *cJSON_GetObjectItem(cJSON *object, const char *key)
{
    if (!object || (object->type & 0xFF) != cJSON_Object || !key) {
        return NULL;
    }
    for (cJSON *c = object->child; c; c = c->next) {
        if (c->string && strcmp(c->string, key) == 0) {
            return c;
        }
    }
    return NULL;
}

__attribute__((unused))
static cJSON_bool cJSON_IsString(cJSON *item)
{
    return item && (item->type & 0xFF) == cJSON_String;
}

__attribute__((unused))
static cJSON_bool cJSON_IsNumber(cJSON *item)
{
    return item && (item->type & 0xFF) == cJSON_Number;
}

__attribute__((unused))
static cJSON_bool cJSON_IsBool(cJSON *item)
{
    return item && ((item->type & 0xFF) == cJSON_True ||
                    (item->type & 0xFF) == cJSON_False);
}

__attribute__((unused))
static cJSON_bool cJSON_IsArray(cJSON *item)
{
    return item && (item->type & 0xFF) == cJSON_Array;
}

/* ---- Parser ------------------------------------------------------------------------ */

typedef struct {
    const char *at;
    const char *end;
    int         failed;
} cj_parser_t;

__attribute__((unused))
static void cj_skip_ws(cj_parser_t *p)
{
    while (p->at < p->end &&
           (*p->at == ' ' || *p->at == '\t' || *p->at == '\n' ||
            *p->at == '\r')) {
        p->at++;
    }
}

static int cj_parse_hex4(const char *s, unsigned *out)
{
    unsigned v = 0;
    for (int i = 0; i < 4; i++) {
        char c = s[i];
        v <<= 4;
        if (c >= '0' && c <= '9')      v |= (unsigned)(c - '0');
        else if (c >= 'a' && c <= 'f') v |= (unsigned)(c - 'a' + 10);
        else if (c >= 'A' && c <= 'F') v |= (unsigned)(c - 'A' + 10);
        else return 0;
    }
    *out = v;
    return 1;
}

/* Parse a quoted string starting at p->at (which must point at '"').
 * Returns a malloc'd buffer or NULL. */
static char *cj_parse_string(cj_parser_t *p)
{
    if (p->at >= p->end || *p->at != '"') {
        p->failed = 1;
        return NULL;
    }
    p->at++;
    size_t cap = 32, len = 0;
    char  *out = (char *)malloc(cap);
    if (!out) {
        p->failed = 1;
        return NULL;
    }

    while (p->at < p->end && *p->at != '"') {
        char c = *p->at;
        if ((unsigned char)c < 0x20) { /* raw control char: invalid JSON */
            p->failed = 1;
            break;
        }
        char decoded[5];
        size_t dlen = 1;
        if (c == '\\') {
            p->at++;
            if (p->at >= p->end) {
                p->failed = 1;
                break;
            }
            switch (*p->at) {
            case '"':  decoded[0] = '"';  break;
            case '\\': decoded[0] = '\\'; break;
            case '/':  decoded[0] = '/';  break;
            case 'b':  decoded[0] = '\b'; break;
            case 'f':  decoded[0] = '\f'; break;
            case 'n':  decoded[0] = '\n'; break;
            case 'r':  decoded[0] = '\r'; break;
            case 't':  decoded[0] = '\t'; break;
            case 'u': {
                unsigned cp = 0;
                if (p->end - p->at < 5 || !cj_parse_hex4(p->at + 1, &cp)) {
                    p->failed = 1;
                    break;
                }
                if (cp < 0x80) {
                    decoded[0] = (char)cp;
                    dlen = 1;
                } else if (cp < 0x800) {
                    decoded[0] = (char)(0xC0 | (cp >> 6));
                    decoded[1] = (char)(0x80 | (cp & 0x3F));
                    dlen = 2;
                } else {
                    decoded[0] = (char)(0xE0 | (cp >> 12));
                    decoded[1] = (char)(0x80 | ((cp >> 6) & 0x3F));
                    decoded[2] = (char)(0x80 | (cp & 0x3F));
                    dlen = 3;
                }
                p->at += 4;
                break;
            }
            default:
                p->failed = 1;
                break;
            }
            if (p->failed) {
                break;
            }
            p->at++;
        } else {
            decoded[0] = c;
            p->at++;
        }
        if (len + dlen + 1 > cap) {
            cap *= 2;
            char *grown = (char *)realloc(out, cap);
            if (!grown) {
                free(out);
                p->failed = 1;
                return NULL;
            }
            out = grown;
        }
        memcpy(out + len, decoded, dlen);
        len += dlen;
    }

    if (p->failed || p->at >= p->end) {
        free(out);
        p->failed = 1;
        return NULL;
    }
    p->at++; /* closing quote */
    out[len] = '\0';
    return out;
}

static cJSON *cj_parse_value(cj_parser_t *p);

static cJSON *cj_parse_number(cj_parser_t *p)
{
    const char *start = p->at;
    while (p->at < p->end &&
           ((*p->at >= '0' && *p->at <= '9') || *p->at == '-' || *p->at == '+' ||
            *p->at == '.' || *p->at == 'e' || *p->at == 'E')) {
        p->at++;
    }
    if (p->at == start) {
        p->failed = 1;
        return NULL;
    }
    char token[64];
    size_t n = (size_t)(p->at - start);
    if (n >= sizeof(token)) {
        p->failed = 1;
        return NULL;
    }
    memcpy(token, start, n);
    token[n] = '\0';

    char *endp = NULL;
    double v = strtod(token, &endp);
    if (endp == token || *endp != '\0') {
        p->failed = 1;
        return NULL;
    }
    cJSON *node = cj_new(cJSON_Number);
    if (node) {
        node->valuedouble = v;
        node->valueint    = (int)v;
    }
    return node;
}

static cJSON *cj_parse_object(cj_parser_t *p)
{
    cJSON *obj = cj_new(cJSON_Object);
    if (!obj) {
        p->failed = 1;
        return NULL;
    }
    p->at++; /* '{' */
    cj_skip_ws(p);
    if (p->at < p->end && *p->at == '}') {
        p->at++;
        return obj;
    }
    for (;;) {
        cj_skip_ws(p);
        char *key = cj_parse_string(p);
        if (p->failed) {
            cJSON_Delete(obj);
            return NULL;
        }
        cj_skip_ws(p);
        if (p->at >= p->end || *p->at != ':') {
            free(key);
            p->failed = 1;
            cJSON_Delete(obj);
            return NULL;
        }
        p->at++;
        cj_skip_ws(p);
        cJSON *value = cj_parse_value(p);
        if (p->failed || !value) {
            free(key);
            cJSON_Delete(obj);
            return NULL;
        }
        value->string = key;
        cJSON_AddItemToArray(obj, value);
        cj_skip_ws(p);
        if (p->at < p->end && *p->at == ',') {
            p->at++;
            continue;
        }
        if (p->at < p->end && *p->at == '}') {
            p->at++;
            return obj;
        }
        p->failed = 1;
        cJSON_Delete(obj);
        return NULL;
    }
}

static cJSON *cj_parse_array(cj_parser_t *p)
{
    cJSON *arr = cj_new(cJSON_Array);
    if (!arr) {
        p->failed = 1;
        return NULL;
    }
    p->at++; /* '[' */
    cj_skip_ws(p);
    if (p->at < p->end && *p->at == ']') {
        p->at++;
        return arr;
    }
    for (;;) {
        cj_skip_ws(p);
        cJSON *value = cj_parse_value(p);
        if (p->failed || !value) {
            cJSON_Delete(arr);
            return NULL;
        }
        cJSON_AddItemToArray(arr, value);
        cj_skip_ws(p);
        if (p->at < p->end && *p->at == ',') {
            p->at++;
            continue;
        }
        if (p->at < p->end && *p->at == ']') {
            p->at++;
            return arr;
        }
        p->failed = 1;
        cJSON_Delete(arr);
        return NULL;
    }
}

static cJSON *cj_parse_value(cj_parser_t *p)
{
    cj_skip_ws(p);
    if (p->at >= p->end) {
        p->failed = 1;
        return NULL;
    }
    switch (*p->at) {
    case '{': return cj_parse_object(p);
    case '[': return cj_parse_array(p);
    case '"': {
        char *s = cj_parse_string(p);
        if (!s) {
            return NULL;
        }
        cJSON *node = cj_new(cJSON_String);
        if (!node) {
            free(s);
            p->failed = 1;
            return NULL;
        }
        node->valuestring = s;
        return node;
    }
    case 't':
        if (p->end - p->at >= 4 && strncmp(p->at, "true", 4) == 0) {
            p->at += 4;
            return cj_new(cJSON_True);
        }
        p->failed = 1;
        return NULL;
    case 'f':
        if (p->end - p->at >= 5 && strncmp(p->at, "false", 5) == 0) {
            p->at += 5;
            return cj_new(cJSON_False);
        }
        p->failed = 1;
        return NULL;
    case 'n':
        if (p->end - p->at >= 4 && strncmp(p->at, "null", 4) == 0) {
            p->at += 4;
            return cj_new(cJSON_NULL);
        }
        p->failed = 1;
        return NULL;
    default:
        return cj_parse_number(p);
    }
}

__attribute__((unused))
static cJSON *cJSON_ParseWithLength(const char *value, size_t buffer_length)
{
    if (!value || buffer_length == 0) {
        return NULL;
    }
    cj_parser_t p = { .at = value, .end = value + buffer_length,
                      .failed = 0 };
    cJSON *root = cj_parse_value(&p);
    if (p.failed || !root) {
        cJSON_Delete(root);
        return NULL;
    }
    cj_skip_ws(&p);
    /* Trailing garbage after the top-level value is a parse failure, matching
     * the real parser's end-of-input check. */
    if (p.at != p.end) {
        cJSON_Delete(root);
        return NULL;
    }
    return root;
}

__attribute__((unused))
static cJSON *cJSON_Parse(const char *value)
{
    return value ? cJSON_ParseWithLength(value, strlen(value)) : NULL;
}

/* ---- Serializer -------------------------------------------------------------------- */

typedef struct {
    char  *buf;
    size_t len;
    size_t cap;
    int    oom;
} cj_print_t;

static void cj_print_grow(cj_print_t *b, size_t extra)
{
    if (b->oom) {
        return;
    }
    if (b->len + extra + 1 <= b->cap) {
        return;
    }
    size_t cap = b->cap ? b->cap : 128;
    while (cap < b->len + extra + 1) {
        cap *= 2;
    }
    char *grown = (char *)realloc(b->buf, cap);
    if (!grown) {
        b->oom = 1;
        return;
    }
    b->buf = grown;
    b->cap = cap;
}

static void cj_print_str(cj_print_t *b, const char *s, size_t n)
{
    cj_print_grow(b, n);
    if (b->oom) {
        return;
    }
    memcpy(b->buf + b->len, s, n);
    b->len += n;
    b->buf[b->len] = '\0';
}

static void cj_print_cstr(cj_print_t *b, const char *s)
{
    cj_print_str(b, s, strlen(s));
}

/* cJSON escaping: quote, backslash, and the C0 controls. */
static void cj_print_escaped(cj_print_t *b, const char *s)
{
    cj_print_grow(b, strlen(s) * 6 + 2);
    if (b->oom) {
        return;
    }
    cj_print_cstr(b, "\"");
    for (const unsigned char *c = (const unsigned char *)s; *c; c++) {
        switch (*c) {
        case '"':  cj_print_cstr(b, "\\\""); break;
        case '\\': cj_print_cstr(b, "\\\\"); break;
        case '\b': cj_print_cstr(b, "\\b");  break;
        case '\f': cj_print_cstr(b, "\\f");  break;
        case '\n': cj_print_cstr(b, "\\n");  break;
        case '\r': cj_print_cstr(b, "\\r");  break;
        case '\t': cj_print_cstr(b, "\\t");  break;
        default:
            if (*c < 0x20) {
                char esc[8];
                snprintf(esc, sizeof(esc), "\\u%04x", *c);
                cj_print_cstr(b, esc);
            } else {
                cj_print_str(b, (const char *)c, 1);
            }
            break;
        }
    }
    cj_print_cstr(b, "\"");
}

static void cj_print_number(cj_print_t *b, double v)
{
    char num[48];
    /* Integral doubles (every number the production code emits) print without
     * a decimal point, matching cJSON's wire format. */
    if (v == (double)(long long)v && v > -1e15 && v < 1e15) {
        snprintf(num, sizeof(num), "%lld", (long long)v);
    } else {
        snprintf(num, sizeof(num), "%.17g", v);
    }
    cj_print_cstr(b, num);
}

static void cj_print_value(cj_print_t *b, const cJSON *item)
{
    if (!item || b->oom) {
        return;
    }
    switch (item->type & 0xFF) {
    case cJSON_Object:
        cj_print_cstr(b, "{");
        for (cJSON *c = item->child; c; c = c->next) {
            if (c != item->child) {
                cj_print_cstr(b, ",");
            }
            cj_print_escaped(b, c->string ? c->string : "");
            cj_print_cstr(b, ":");
            cj_print_value(b, c);
        }
        cj_print_cstr(b, "}");
        break;
    case cJSON_Array:
        cj_print_cstr(b, "[");
        for (cJSON *c = item->child; c; c = c->next) {
            if (c != item->child) {
                cj_print_cstr(b, ",");
            }
            cj_print_value(b, c);
        }
        cj_print_cstr(b, "]");
        break;
    case cJSON_String:
        cj_print_escaped(b, item->valuestring ? item->valuestring : "");
        break;
    case cJSON_Number:
        cj_print_number(b, item->valuedouble);
        break;
    case cJSON_True:
        cj_print_cstr(b, "true");
        break;
    case cJSON_False:
        cj_print_cstr(b, "false");
        break;
    case cJSON_NULL:
        cj_print_cstr(b, "null");
        break;
    default:
        cj_print_cstr(b, "null");
        break;
    }
}

__attribute__((unused))
static char *cJSON_PrintUnformatted(const cJSON *item)
{
    cj_print_t b = { .buf = NULL, .len = 0, .cap = 0, .oom = 0 };
    cj_print_grow(&b, 128);
    if (b.oom) {
        return NULL;
    }
    b.buf[0] = '\0';
    cj_print_value(&b, item);
    if (b.oom) {
        free(b.buf);
        return NULL;
    }
    return b.buf;
}

#endif /* SPAXEL_TEST_STUB_CJSON_H */
