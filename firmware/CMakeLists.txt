cmake_minimum_required(VERSION 3.16)

# SPAXEL ESP32-S3 Firmware
# WiFi CSI-based indoor positioning node

# Version comes from the repo VERSION file, which is the single source of truth
# the mothership also uses (it derives the expected version from the uploaded
# firmware filename). Falling back to `git describe` produced a version that
# could never match, and the node reported a hardcoded "1.0.0" regardless — so
# every successful OTA was logged as a rollback. See ADR-004 / bf-556tl.
# Re-run CMake when VERSION changes. Without this the version is baked at first
# configure and a later bump silently no-ops: the build succeeds, the filename
# says the new version, and the firmware still reports the old one — which is
# precisely the mismatch that made every successful OTA look like a rollback.
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS
             "${CMAKE_CURRENT_SOURCE_DIR}/../VERSION")

if(EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/../VERSION")
    file(READ "${CMAKE_CURRENT_SOURCE_DIR}/../VERSION" PROJECT_VERSION)
    string(STRIP "${PROJECT_VERSION}" PROJECT_VERSION)
else()
    execute_process(
        COMMAND git describe --tags --dirty
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        RESULT_VARIABLE GIT_RESULT
        OUTPUT_VARIABLE GIT_VERSION
        OUTPUT_STRIP_TRAILING_WHITESPACE
        ERROR_QUIET
    )
    if(GIT_RESULT EQUAL 0 AND GIT_VERSION)
        set(PROJECT_VERSION "${GIT_VERSION}")
    else()
        set(PROJECT_VERSION "0.1.0")
    endif()
endif()

message(STATUS "Building SPAXEL firmware version: ${PROJECT_VERSION}")

# Keep the version in a variable ESP-IDF does not own.
#
# ESP-IDF wraps project() with its own macro, which reassigns PROJECT_VERSION —
# so configure_file() below saw an EMPTY value and generated
# SPAXEL_FIRMWARE_VERSION "", which the node then reported as a blank version.
# SPAXEL_VERSION survives because nothing else touches it.
set(SPAXEL_VERSION "${PROJECT_VERSION}")

# PROJECT_VER is what ESP-IDF stamps into the app descriptor, which is what
# esp_app_get_description()->version returns and what OTA/rollback logic reads.
set(PROJECT_VER "${SPAXEL_VERSION}")

# Console board profile. Native USB-Serial/JTAG is the shipped ESP32-S3
# default; bridge-equipped boards select sdkconfig.uart-console explicitly.
# Keep this selection before project.cmake so ESP-IDF sees it while generating
# sdkconfig. A command-line -D SDKCONFIG_DEFAULTS=... remains authoritative.
if(NOT DEFINED SDKCONFIG_DEFAULTS)
    set(SDKCONFIG_DEFAULTS "sdkconfig.defaults;sdkconfig.usbjtag")
endif()

# Anti-rollback secure version.
#
# firmware/SECURE_VERSION is the single source of truth. It feeds
# CONFIG_BOOTLOADER_APP_SECURE_VERSION, which esptool stamps into the app
# image header, which the ESP32-S3 bootloader then compares against the
# SECURE_VERSION eFuse field. It is deliberately separate from VERSION above:
# VERSION tracks what the firmware is, SECURE_VERSION tracks whether it is
# safe to accept, and conflating them (as this block used to do by printing
# ${SPAXEL_VERSION} next to "security version") would bump the eFuse counter
# on every feature release and burn nodes that can never be un-bricked.
#
# Before this wiring existed every image was built with secure version 0 —
# verified by parsing the header of the 2026-09-05 build artifact — so the
# eFuse check could never reject anything and anti-rollback was inert.
set(SPAXEL_SECURE_VERSION_FILE "${CMAKE_CURRENT_SOURCE_DIR}/SECURE_VERSION")

if(NOT EXISTS "${SPAXEL_SECURE_VERSION_FILE}")
    message(FATAL_ERROR
        "firmware/SECURE_VERSION is missing. Anti-rollback is on "
        "(CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK), so an image without a secure "
        "version builds as 0 and the eFuse check would never reject anything. "
        "Restore the file from git (or run scripts/bump-secure-version.sh).")
endif()

file(READ "${SPAXEL_SECURE_VERSION_FILE}" SPAXEL_SECURE_VERSION)
# Strip the policy comments so the value can sit next to its explanation.
string(REGEX REPLACE "#[^\n]*" "" SPAXEL_SECURE_VERSION "${SPAXEL_SECURE_VERSION}")
string(STRIP "${SPAXEL_SECURE_VERSION}" SPAXEL_SECURE_VERSION)

if(NOT SPAXEL_SECURE_VERSION MATCHES "^[0-9]+$")
    message(FATAL_ERROR
        "firmware/SECURE_VERSION must hold one non-negative integer, got "
        "'${SPAXEL_SECURE_VERSION}'")
endif()

# CONFIG_BOOTLOADER_APP_SEC_VER_SIZE_EFUSE_FIELD is 16 bits on the ESP32-S3.
if(SPAXEL_SECURE_VERSION GREATER 65535)
    message(FATAL_ERROR
        "firmware/SECURE_VERSION=${SPAXEL_SECURE_VERSION} does not fit the "
        "16-bit SECURE_VERSION eFuse field")
endif()

set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS
             "${SPAXEL_SECURE_VERSION_FILE}")

# Hand the value to ESP-IDF through a generated defaults fragment rather than
# an edited sdkconfig.defaults: the fragment is machine-owned and shows up in
# every build, while a hand-edited defaults line would still lose to an
# already-generated local sdkconfig (ESP-IDF lets sdkconfig win over
# SDKCONFIG_DEFAULTS), which is exactly how a bump would silently no-op.
set(SPAXEL_SECURE_VERSION_FRAGMENT
    "${CMAKE_CURRENT_BINARY_DIR}/sdkconfig.secure-version")
file(WRITE "${SPAXEL_SECURE_VERSION_FRAGMENT}"
    "# Generated from firmware/SECURE_VERSION by CMakeLists.txt - do not edit.\n"
    "CONFIG_BOOTLOADER_APP_SECURE_VERSION=${SPAXEL_SECURE_VERSION}\n")

set(SDKCONFIG_DEFAULTS "${SDKCONFIG_DEFAULTS};${SPAXEL_SECURE_VERSION_FRAGMENT}")

# A generated local sdkconfig outlives the defaults that seeded it, so a
# stale `CONFIG_BOOTLOADER_APP_SECURE_VERSION=0` there would beat the
# fragment above on every subsequent configure and ship the old version
# again. Drop just this one line when it disagrees; ESP-IDF regenerates the
# file from the fragment during project(). Nothing else in the file is
# touched — sdkconfig stays a developer-owned file apart from this symbol,
# which the release process owns.
if(NOT DEFINED SDKCONFIG)
    set(SDKCONFIG "${CMAKE_CURRENT_SOURCE_DIR}/sdkconfig")
endif()
if(EXISTS "${SDKCONFIG}")
    file(READ "${SDKCONFIG}" SPAXEL_SDKCONFIG_TEXT)
    if(NOT SPAXEL_SDKCONFIG_TEXT MATCHES
       "(^|\n)CONFIG_BOOTLOADER_APP_SECURE_VERSION=${SPAXEL_SECURE_VERSION}(\n|$)")
        string(REGEX REPLACE
            "\n# CONFIG_BOOTLOADER_APP_SECURE_VERSION[^\n]*" "\n"
            SPAXEL_SDKCONFIG_TEXT "${SPAXEL_SDKCONFIG_TEXT}")
        string(REGEX REPLACE
            "\nCONFIG_BOOTLOADER_APP_SECURE_VERSION=[^\n]*" "\n"
            SPAXEL_SDKCONFIG_TEXT "${SPAXEL_SDKCONFIG_TEXT}")
        file(WRITE "${SDKCONFIG}" "${SPAXEL_SDKCONFIG_TEXT}")
        message(STATUS
            "Reconciled ${SDKCONFIG} to the secure version in "
            "firmware/SECURE_VERSION (${SPAXEL_SECURE_VERSION})")
    endif()
endif()

message(STATUS "Anti-rollback secure version: ${SPAXEL_SECURE_VERSION} (from firmware/SECURE_VERSION)")

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(spaxel-firmware VERSION "${SPAXEL_VERSION}")

# Generate version header
configure_file(
    ${CMAKE_CURRENT_SOURCE_DIR}/main/version.h.in
    ${CMAKE_CURRENT_BINARY_DIR}/spaxel-firmware/main/version.h
    @ONLY
)

# Secure Boot V2: Firmware signing configuration
#
# When Secure Boot V2 is enabled (CONFIG_SECURE_BOOT_V2=y), the build system
# automatically signs bootloader and app binaries with the private key.
# The signing key is expected at firmware/keys/firmware_signing_key.pem.
#
# Generate signing key if it doesn't exist:
#   ./scripts/generate-signing-key.sh
#
# IMPORTANT: The private signing key MUST be kept secret. Never commit it to git.
# The keys/ directory is already in .gitignore.
#
# For production deployment, store the private key in a secure location (HSM, offline storage)
# and set the path via secure_boot_signing_key in the project configuration.

if(CONFIG_SECURE_BOOT_V2)
    message(STATUS "Secure Boot V2 enabled - firmware will be signed")

    # Set signing key path (default: keys/firmware_signing_key.pem)
    set(SECURE_BOOT_SIGNING_KEY "${CMAKE_CURRENT_SOURCE_DIR}/keys/firmware_signing_key.pem")

    if(NOT EXISTS "${SECURE_BOOT_SIGNING_KEY}")
        message(WARNING "Secure Boot V2 signing key not found: ${SECURE_BOOT_SIGNING_KEY}")
        message(WARNING "Generate signing key: ./scripts/generate-signing-key.sh")
        message(WARNING "Build will fail without the signing key")
    else()
        message(STATUS "Using signing key: ${SECURE_BOOT_SIGNING_KEY}")
    endif()

    # Configure secure boot signing key path
    set(secure_boot_signing_key "${SECURE_BOOT_SIGNING_KEY}")
endif()

# Anti-rollback: Security version configuration
#
# The number itself is resolved above, before project(), because ESP-IDF
# regenerates sdkconfig inside project() and that is the only point where a
# change to it can still land in this build.
#
# When anti-rollback is enabled (CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK=y — the
# current symbol name; CONFIG_APP_ANTI_ROLLBACK survives only as a
# deprecated mirror in the generated sdkconfig), each firmware image
# must include a security version number. The device will refuse to boot any image
# with a lower security version than what's burned in eFuse.

if(NOT CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK)
    message(WARNING
        "Anti-rollback is disabled in this build, so the secure version "
        "${SPAXEL_SECURE_VERSION} in firmware/SECURE_VERSION is recorded but "
        "not enforced. sdkconfig.defaults expects it on.")
endif()

# Verify the artifact in this build directory, not just the intent. What the
# header actually says is the only thing a node can act on, and a build that
# silently emitted 0 (this project's state until 2026-09-05) would otherwise
# pass unnoticed. This runs at configure time, so on a fresh build directory
# it fires on the *next* configure rather than on the build that produced the
# image; the release gate that inspects the artifact CI just attached is
# scripts/inspect-firmware-header.py run with --expect, as described in
# docs/notes/anti-rollback-secure-version.md.
if(EXISTS "${CMAKE_CURRENT_BINARY_DIR}/spaxel-firmware.bin")
    find_package(Python3 COMPONENTS Interpreter QUIET)
    if(Python3_Interpreter_FOUND)
        execute_process(
            COMMAND "${Python3_EXECUTABLE}"
                    "${CMAKE_CURRENT_SOURCE_DIR}/scripts/inspect-firmware-header.py"
                    "${CMAKE_CURRENT_BINARY_DIR}/spaxel-firmware.bin"
                    "--expect" "${SPAXEL_SECURE_VERSION}"
            RESULT_VARIABLE SPAXEL_HEADER_VERIFY_RESULT
            OUTPUT_VARIABLE SPAXEL_HEADER_VERIFY_OUT
            ERROR_VARIABLE SPAXEL_HEADER_VERIFY_OUT
        )
        if(NOT SPAXEL_HEADER_VERIFY_RESULT EQUAL 0)
            message(FATAL_ERROR
                "Built image does not carry the secure version this release "
                "promised (${SPAXEL_SECURE_VERSION}).\n"
                "${SPAXEL_HEADER_VERIFY_OUT}")
        endif()
    endif()
endif()
