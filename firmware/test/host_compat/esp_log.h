/*
 * Host stand-in for ESP-IDF's esp_log.h.
 *
 * The ESP_LOGx macros become calls to host_compat_log(): a no-op carrying the
 * printf format attribute, so every call site's format string and argument
 * types are still checked by the compiler exactly as they are against the real
 * macros, but nothing is written to stdout during a test run.
 *
 * The tag argument is passed through so a file-scope
 * `static const char *TAG` in the production TU is referenced rather than
 * tripping -Wunused-variable.
 */
#pragma once

void host_compat_log(const char *tag, const char *fmt, ...)
    __attribute__((format(printf, 2, 3)));

#define ESP_LOGI(tag, fmt, ...) host_compat_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGW(tag, fmt, ...) host_compat_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGE(tag, fmt, ...) host_compat_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGD(tag, fmt, ...) host_compat_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGV(tag, fmt, ...) host_compat_log((tag), (fmt), ##__VA_ARGS__)
