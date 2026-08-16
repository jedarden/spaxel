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
# When anti-rollback is enabled (CONFIG_APP_ANTI_ROLLBACK=y), each firmware image
# must include a security version number. The device will refuse to boot any image
# with a lower security version than what's burned in eFuse.
#
# For now, we use the firmware version as the security version. In production,
# you may want a separate security version that increments on security fixes only.

if(CONFIG_APP_ANTI_ROLLBACK)
    message(STATUS "Anti-rollback enabled - security version: ${SPAXEL_VERSION}")
endif()

