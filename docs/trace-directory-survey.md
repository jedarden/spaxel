# Trace Directory File Type Survey

**Date:** 2026-08-24  
**Purpose:** Sample bead trace directories to document file types and size ranges

## Methodology

Examined 5 trace directories out of 96 total directories in `.beads/traces/`:
- spaxel-9b4087cc
- spaxel-888e1a53  
- spaxel-5b7fb2c9
- spaxel-53998433
- spaxel-069d866b

## File Types Present

Each trace directory contains a consistent set of file types:

### 1. metadata.json
- **Type:** JSON metadata file
- **Size:** ~410-411 bytes (consistent across all samples)
- **Purpose:** Trace session metadata

### 2. stderr.txt
- **Type:** Text log file
- **Size:** 266 bytes (consistent across all samples)
- **Purpose:** Standard error output from trace execution

### 3. stdout.txt
- **Type:** Text log file
- **Size Range:** 1.1 MB - 6.4 MB
- **Samples:**
  - spaxel-888e1a53: 1.2 MB
  - spaxel-5b7fb2c9: 1.1 MB  
  - spaxel-9b4087cc: 3.3 MB
  - spaxel-069d866b: 3.8 MB
  - spaxel-53998433: 6.4 MB (largest observed)
- **Purpose:** Standard output log from trace execution

### 4. trace.jsonl
- **Type:** JSON Lines file
- **Size Range:** 18 KB - 59 KB
- **Presence:** Not all directories contain this file
- **Samples:**
  - spaxel-5b7fb2c9: 18 KB
  - spaxel-069d866b: 35 KB
  - spaxel-9b4087cc: 56 KB
  - spaxel-53998433: 59 KB (largest observed)
  - spaxel-888e1a53: *not present*
- **Purpose:** Structured trace events in JSON Lines format

## Summary

- **Total directories sampled:** 5 of 96
- **File type consistency:** High - all directories contain metadata.json, stderr.txt, and stdout.txt
- **Variable file:** trace.jsonl (present in 4 of 5 sampled directories)
- **Largest file size observed:** 6.4 MB (stdout.txt in spaxel-53998433)
- **Smallest file size observed:** 266 bytes (stderr.txt, consistent)
- **Dominant file type by size:** stdout.txt (megabyte-scale logs)
