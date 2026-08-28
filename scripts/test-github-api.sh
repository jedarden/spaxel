#!/run/current-system/sw/bin/bash
# Test script to verify GitHub API access for Kaniko releases
# This script tests the GitHub API endpoint and authentication configuration

set -e

GITHUB_API_BASE="https://api.github.com"
KANIKO_OWNER="GoogleContainerTools"
KANIKO_REPO="kaniko"

echo "=== GitHub API Access Test for Kaniko Releases ==="
echo ""

# Check if GitHub token is configured
if [ -n "$SPAXEL_GITHUB_TOKEN" ]; then
    echo "✓ GitHub token found (SPAXEL_GITHUB_TOKEN is set)"
    TOKEN_LEN=${#SPAXEL_GITHUB_TOKEN}
    echo "  Token length: $TOKEN_LEN characters"

    # Test authenticated API access
    echo ""
    echo "Testing authenticated GitHub API access..."
    AUTH_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $SPAXEL_GITHUB_TOKEN" \
        -H "Accept: application/vnd.github+json" \
        "$GITHUB_API_BASE/user")

    if [ "$AUTH_RESPONSE" = "200" ] || [ "$AUTH_RESPONSE" = "401" ]; then
        echo "✓ GitHub API endpoint is accessible (HTTP $AUTH_RESPONSE)"
        if [ "$AUTH_RESPONSE" = "200" ]; then
            echo "  Authentication: Token is valid"
        else
            echo "  Authentication: Token is invalid (but API is reachable)"
        fi
    else
        echo "✗ GitHub API ping failed (HTTP $AUTH_RESPONSE)"
        exit 1
    fi
else
    echo "⚠ GitHub token not configured (SPAXEL_GITHUB_TOKEN not set)"
    echo "  Unauthenticated requests will be rate-limited (60 requests/hour)"

    # Test unauthenticated API access
    echo ""
    echo "Testing unauthenticated GitHub API access..."
    # Use a public repository endpoint that doesn't require authentication
    AUTH_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Accept: application/vnd.github+json" \
        "$GITHUB_API_BASE/repos/github/github-example")

    if [ "$AUTH_RESPONSE" = "200" ] || [ "$AUTH_RESPONSE" = "403" ] || [ "$AUTH_RESPONSE" = "404" ]; then
        echo "✓ GitHub API endpoint is accessible (HTTP $AUTH_RESPONSE)"
    else
        echo "✗ GitHub API ping failed (HTTP $AUTH_RESPONSE)"
        exit 1
    fi
fi

echo ""
echo "Testing Kaniko releases endpoint..."
RELEASES_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Accept: application/vnd.github+json" \
    "$GITHUB_API_BASE/repos/$KANIKO_OWNER/$KANIKO_REPO/releases")

if [ "$RELEASES_RESPONSE" = "200" ]; then
    echo "✓ Kaniko releases endpoint is accessible"

    # Fetch actual releases to verify we can get data
    echo ""
    echo "Fetching latest Kaniko releases (first 5)..."
    RELEASES_DATA=$(curl -s \
        -H "Accept: application/vnd.github+json" \
        "$GITHUB_API_BASE/repos/$KANIKO_OWNER/$KANIKO_REPO/releases?per_page=5")

    if echo "$RELEASES_DATA" | jq -e '.[] | {tag_name, name, published_at}' 2>/dev/null; then
        echo "✓ Successfully retrieved Kaniko releases data"
    else
        echo "⚠ Could not parse releases JSON (but endpoint is accessible)"
    fi
elif [ "$RELEASES_RESPONSE" = "404" ]; then
    echo "✗ Repository not found (HTTP 404)"
    echo "  Expected repository: $KANIKO_OWNER/$KANIKO_REPO"
    exit 1
elif [ "$RELEASES_RESPONSE" = "403" ]; then
    echo "⚠ Rate limited by GitHub API (HTTP 403)"
    echo "  Consider setting SPAXEL_GITHUB_TOKEN for authenticated access"
    echo "  Unauthenticated rate limit: 60 requests/hour"
    echo "  Authenticated rate limit: 5000 requests/hour"
else
    echo "✗ Unexpected response (HTTP $RELEASES_RESPONSE)"
    exit 1
fi

echo ""
echo "=== GitHub API Access Summary ==="
echo "✓ GitHub API endpoint is accessible: $GITHUB_API_BASE"
echo "✓ Kaniko releases endpoint: $GITHUB_API_BASE/repos/$KANIKO_OWNER/$KANIKO_REPO/releases"
if [ -n "$SPAXEL_GITHUB_TOKEN" ]; then
    echo "✓ Authentication method configured: Bearer token"
else
    echo "⚠ No authentication (rate limited: 60 req/hour)"
fi
echo ""
echo "Next steps:"
echo "1. Set SPAXEL_GITHUB_TOKEN environment variable for authenticated access"
echo "2. Use the GitHub client in mothership/internal/github to fetch Kaniko releases"
