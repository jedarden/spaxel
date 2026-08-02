# Kaniko TARGETPLATFORM Build Arg Fix (bf-38dbu)

## Original Problem Statement

**Title:** Kaniko caches TARGETPLATFORM-conditional RUN steps across architectures, poisoning /project/build

**Initial Diagnosis:** Cache poisoning - Kaniko was thought to be reusing cached layers across different TARGETPLATFORM builds, causing the arm64 "skip" branch placeholder to poison the amd64 build.

## Actual Root Cause

The issue was **NOT** cache poisoning. Kaniko does not auto-populate `TARGETPLATFORM`, `BUILDPLATFORM`, or `TARGETARCH` build args (unlike Docker buildx). Without explicit defaults, these ARGs resolved to empty strings in the Dockerfile.

### Impact

In the firmware-builder stage:
```dockerfile
# Before fix - ARG with no default
ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" != "linux/amd64" ]; then \
        echo "# Placeholder" > /project/build/spaxel-firmware-merged.bin; \
    fi
```

When `TARGETPLATFORM` was empty:
- Condition `"" != "linux/amd64"` was **always true**
- The "skip" branch unconditionally created a placeholder file
- `idf.py set-target`'s implicit fullclean refused to clean this non-CMake file
- Build failed **100% deterministically**, regardless of caching

## The Fix (Commit c482782)

Added default values to all platform-related ARGs at every declaration site:

```dockerfile
# After fix - ARG with default
ARG TARGETPLATFORM=linux/amd64
ARG BUILDPLATFORM=linux/amd64
ARG TARGETARCH=amd64
```

This ensures:
- Kaniko builds (which don't pass platform build-args) use the CI builder's actual platform (amd64)
- Buildx can still override these for local multi-arch builds
- Conditional logic works correctly in both scenarios

## Three Previous Failed Attempts

The git history shows three commits that chased the "cache poisoning" red herring:
1. `51bc25e` - move FIRMWARE_CACHE_BUST before poisoned layers
2. `e0ff1de` - actually reference FIRMWARE_CACHE_BUST in RUN commands
3. `41a82e3` - use literal cache-bust text, not ARG substitution

All three tried to bust Kaniko's layer cache, but the real issue was that the layers genuinely **were** identical every time - the underlying command was deterministic given the always-empty ARG.

## Validation

The fix was validated against the real Kaniko binary:
```bash
docker pull gcr.io/kaniko-project/executor:latest
docker run --rm \
  -v "$PWD:/workspace" \
  gcr.io/kaniko-project/executor:latest \
  --dockerfile /workspace/Dockerfile \
  --context dir:///workspace \
  --no-push
```

Built all three stages successfully with no platform build-args (matching the CI invocation exactly).

## Status

✅ **RESOLVED** - Fix committed in c482782 (2026-08-02)
- Dockerfile now has proper ARG defaults
- Kaniko builds work correctly
- Buildx multi-arch builds still work
- No cache poisoning issue existed
