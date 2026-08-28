# Bench Machine Network Connectivity

## Overview

This document validates the network path from the mothership to the bench machine used for hardware testing and development.

## Hostname Resolution

The bench hostname resolves successfully via Tailscale:

```bash
$ host bench
bench.tail1b1987.ts.net has address 100.67.12.56
```

- **Hostname:** `bench.tail1b1987.ts.net`
- **Tailscale IP:** `100.67.12.56`
- **DNS:** Resolves via Tailscale's MagicDNS (`.ts.net` suffix)

## Port Accessibility

Port 6080 on the bench machine is accessible:

```bash
$ nc -zv bench.tail1b1987.ts.net 6080
Connection to bench.tail1b1987.ts.net (100.67.12.56) 6080 port [tcp/*] succeeded!
```

**Status:** ✅ **OPEN** - Port 6080 is accessible and accepting connections

## Network Path

The network path to the bench machine is:

1. **Local machine** (`codinghome.ardenone.com`)
   - Resolves `bench` via Tailscale MagicDNS
   - Routes through Tailscale VPN (`100.67.12.56`)

2. **Bench machine** (`bench.tail1b1987.ts.net`)
   - Accepts connections on port 6080
   - Service running on port 6080 (purpose TBD based on usage)

## Validation Date

- **Last verified:** 2026-08-28
- **Verification method:** DNS resolution + TCP port check

## Usage Notes

- The bench machine is reachable via Tailscale VPN from any machine on the same tailnet
- Port 6080 is the designated service port for bench operations
- All connectivity is over the encrypted Tailscale mesh network
