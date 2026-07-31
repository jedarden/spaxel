cmake_minimum_required(VERSION 3.16)

# SPAXEL ESP32-S3 Firmware
# WiFi CSI-based indoor positioning node

# Version comes from the repo VERSION file, which is the single source of truth
# the mothership also uses (it derives the expected version from the uploaded
# firmware filename). Falling back to `git describe` produced a version that
# could never match, and the node reported a hardcoded "1.0.0" regardless — so
# every successful OTA was logged as a rollback. See ADR-004 / bf-556tl.
if(EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/../VERSION")
    file(READ "${CMAKE_CURRENT_SOURCE_DIR}/../VERSION" PROJECT_VERSION)
    string(STRIP "${PROJECT_VERSION}" PROJECT_VERSION)
else()
    execute_process(
        COMMAND git describe --tags --dirty 2>/dev/null
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
        OUTPUT_VARIABLE GIT_VERSION
        OUTPUT_STRIP_TRAILING_WHITESPACE
        ERROR_QUIET
    )
    if(GIT_VERSION)
        set(PROJECT_VERSION "${GIT_VERSION}")
    else()
        set(PROJECT_VERSION "0.1.0")
    endif()
endif()

message(STATUS "Building SPAXEL firmware version: ${PROJECT_VERSION}")

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(spaxel-firmware VERSION "${PROJECT_VERSION}")

# Generate version header
configure_file(
    ${CMAKE_CURRENT_SOURCE_DIR}/main/version.h.in
    ${CMAKE_CURRENT_BINARY_DIR}/spaxel-firmware/main/version.h
    @ONLY
)

