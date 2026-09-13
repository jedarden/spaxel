/*
 * Host stand-in for ESP-IDF's esp_log.h.
 *
 * The ESP_LOGx macros become calls to a no-op that still carries the printf
 * format attribute, so every call site's format string and argument types are
 * checked by the compiler exactly as they are against the real macros, while
 * the run stays quiet. The tag is passed through so the production TU's
 * file-scope `static const char *TAG` is referenced rather than tripping
 * -Wunused-variable.
 *
 * Deliberately NOT included by test_watchdog.c itself: watchdog.c's own
 * #include "esp_log.h" resolves here on the CI include path, and letting that
 * include do the resolving is what keeps a single esp_log.h expanded per TU
 * (a second stand-in for the same header in another test directory would
 * redefine these macros with a different body).
 */
#ifndef SPAXEL_TEST_STUB_ESP_LOG_H
#define SPAXEL_TEST_STUB_ESP_LOG_H

static inline void spaxel_stub_log(const char *tag, const char *fmt, ...)
    __attribute__((format(printf, 2, 3)));
static inline void spaxel_stub_log(const char *tag, const char *fmt, ...)
{
    (void)tag;
    (void)fmt;
}

#define ESP_LOGI(tag, fmt, ...) spaxel_stub_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGW(tag, fmt, ...) spaxel_stub_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGE(tag, fmt, ...) spaxel_stub_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGD(tag, fmt, ...) spaxel_stub_log((tag), (fmt), ##__VA_ARGS__)
#define ESP_LOGV(tag, fmt, ...) spaxel_stub_log((tag), (fmt), ##__VA_ARGS__)

#endif /* SPAXEL_TEST_STUB_ESP_LOG_H */
