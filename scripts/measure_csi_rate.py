#!/usr/bin/env python3
"""
CSI Frame Rate Measurement Script for Ambient Traffic Analysis

Reads CSI frames from the mothership's replay store file and categorizes them
to measure frame rates, beacon vs data traffic, and packet statistics.

This is part of ADR-003 / bead spaxel-76601bae.

Usage:
    python scripts/measure_csi_rate.py [OPTIONS]

Options:
    --duration SECONDS    Measurement duration in seconds (default: 300 = 5 minutes)
    --output FILE         Output file path (default: stdout)
    --format FORMAT       Output format: json or csv (default: json)
    --replay-file PATH   Path to CSI replay file (default: /data/spaxel/csi_replay.bin)
    --ap-bssid MAC        AP BSSID to classify beacon frames (auto-detected if not set)
    --help                Show this help message

Output (JSON):
    {
        "start_time": "2024-03-15T14:32:05.123Z",
        "end_time": "2024-03-15T14:37:05.456Z",
        "duration_s": 300.333,
        "total_frames": 15234,
        "frames_per_second": 50.78,
        "beacon_frames": 12045,
        "beacon_rate_hz": 40.15,
        "data_frames": 3189,
        "data_rate_hz": 10.63,
        "ap_bssid": "AA:BB:CC:DD:EE:FF",
        "unique_links": 5,
        "links": {
            "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55": {
                "peer_mac": "00:11:22:33:44:55",
                "node_mac": "AA:BB:CC:DD:EE:FF",
                "frame_count": 15234,
                "avg_rssi_dbm": -45.2,
                "avg_channel": 6,
                "min_rssi_dbm": -52,
                "max_rssi_dbm": -38
            }
        },
        "frame_timestamps": [1710505925.123, 1710505925.145, ...]
    }

Output (CSV):
    timestamp_iso,node_mac,peer_mac,rssi_dbm,channel,n_sub,frame_type,link_id
    2024-03-15T14:32:05.123Z,AA:BB:CC:DD:EE:FF,00:11:22:33:44:55,-45,6,64,beacon,AA:BB:CC:DD:EE:FF:00:11:22:33:44:55
    ...
"""

import argparse
import struct
import json
import sys
import csv
from datetime import datetime, timezone
from collections import defaultdict
from typing import Dict, List, Optional, Tuple
import os


class CSIReplayReader:
    """Reads CSI replay file and extracts frame statistics."""

    # CSI Replay file format
    FILE_MAGIC = b"SPAXLREP"
    HEADER_SIZE = 32  # magic(8) + writePos(8) + oldestPos(8) + wrapPos(8)
    RECORD_OVERHEAD = 10  # recvTimeNS(8) + frameLen(2)

    # CSI frame header
    FRAME_HEADER_SIZE = 24
    MAX_FRAME_SIZE = 280  # 24 + 128*2

    def __init__(self, replay_file: str, ap_bssid: Optional[str] = None):
        self.replay_file = replay_file
        self.ap_bssid = ap_bssid

        # File positions
        self.write_pos = 0
        self.oldest_pos = 0
        self.wrap_pos = 0

        # Statistics
        self.total_frames = 0
        self.beacon_frames = 0
        self.data_frames = 0
        self.frame_timestamps: List[float] = []
        self.frame_records: List[Dict] = []

        # Per-link statistics
        self.link_stats: Dict[str, Dict] = defaultdict(lambda: {
            "frame_count": 0,
            "rssi_values": [],
            "channels": defaultdict(int),
            "node_mac": "",
            "peer_mac": ""
        })

        # Track peer MACs for auto-detection
        self.peer_macs: set = set()
        self.ap_candidates: Dict[str, int] = defaultdict(int)  # peer_mac -> frame count

        # Time range
        self.first_timestamp_ns: Optional[int] = None
        self.last_timestamp_ns: Optional[int] = None

    def read_file(self, duration_s: int) -> bool:
        """Read and parse the replay file for specified duration."""
        if not os.path.exists(self.replay_file):
            print(f"Error: Replay file not found: {self.replay_file}", file=sys.stderr)
            return False

        file_size = os.path.getsize(self.replay_file)
        if file_size < self.HEADER_SIZE:
            print(f"Error: File too small to be valid ({file_size} bytes)", file=sys.stderr)
            return False

        try:
            with open(self.replay_file, 'rb') as f:
                # Read and validate header
                if not self._read_header(f):
                    return False

                # Read frames
                self._read_frames(f, duration_s)

                # Auto-detect AP BSSID if not set
                if self.ap_bssid is None and self.ap_candidates:
                    self.ap_bssid = max(self.ap_candidates.items(), key=lambda x: x[1])[0]
                    print(f"Auto-detected AP BSSID: {self.ap_bssid}", file=sys.stderr)

                return True

        except IOError as e:
            print(f"Error reading file: {e}", file=sys.stderr)
            return False
        except struct.error as e:
            print(f"Error parsing file format: {e}", file=sys.stderr)
            return False

    def _read_header(self, f) -> bool:
        """Read and validate replay file header."""
        header_data = f.read(self.HEADER_SIZE)
        if len(header_data) < self.HEADER_SIZE:
            print("Error: Could not read complete header", file=sys.stderr)
            return False

        # Parse magic
        magic = header_data[0:8]
        if magic != self.FILE_MAGIC:
            print(f"Error: Invalid magic: {magic!r}", file=sys.stderr)
            return False

        # Parse positions (little-endian uint64)
        self.write_pos = struct.unpack('<Q', header_data[8:16])[0]
        self.oldest_pos = struct.unpack('<Q', header_data[16:24])[0]
        self.wrap_pos = struct.unpack('<Q', header_data[24:32])[0]

        print(f"Replay file: write_pos={self.write_pos}, oldest_pos={self.oldest_pos}, wrap_pos={self.wrap_pos}", file=sys.stderr)
        return True

    def _read_frames(self, f, duration_s: int) -> None:
        """Read CSI frames for specified duration."""
        start_pos = self.oldest_pos if self.oldest_pos > 0 else self.HEADER_SIZE
        end_pos = self.write_pos if self.write_pos > start_pos else f.seek(0, 2)  # EOF

        # Seek to first frame
        f.seek(start_pos)

        start_time = None
        end_time = None
        target_duration_ns = duration_s * 1_000_000_000

        while True:
            # Check if we've read enough
            pos = f.tell()

            # Handle wrap-around
            if pos >= self.write_pos and self.wrap_pos > 0:
                f.seek(self.HEADER_SIZE)
                pos = self.HEADER_SIZE

            # Check if we've wrapped back to write position
            if pos >= self.write_pos and self.wrap_pos == 0:
                break

            # Read record header
            record_header = f.read(10)
            if len(record_header) < 10:
                break  # End of file or incomplete record

            recv_time_ns = struct.unpack('<q', record_header[0:8])[0]
            frame_len = struct.unpack('<H', record_header[8:10])[0]

            # Track time range
            if self.first_timestamp_ns is None:
                self.first_timestamp_ns = recv_time_ns
                start_time = recv_time_ns

            if self.last_timestamp_ns is None or recv_time_ns > self.last_timestamp_ns:
                self.last_timestamp_ns = recv_time_ns
                end_time = recv_time_ns

            # Check duration
            elapsed = end_time - start_time
            if elapsed >= target_duration_ns:
                break

            # Read frame data
            if frame_len > self.MAX_FRAME_SIZE or frame_len < self.FRAME_HEADER_SIZE:
                # Skip invalid frame
                f.seek(pos + 10 + frame_len)
                continue

            frame_data = f.read(frame_len)
            if len(frame_data) < frame_len:
                break  # Incomplete frame

            # Process frame
            self._process_frame(frame_data, recv_time_ns)

            # Move to next record
            pos = f.tell()

    def _process_frame(self, frame_data: bytes, recv_time_ns: int) -> None:
        """Parse and categorize a CSI binary frame."""
        if len(frame_data) < self.FRAME_HEADER_SIZE:
            return  # Too short to be valid

        # Parse CSI frame header (24 bytes)
        node_mac = frame_data[0:6]
        peer_mac = frame_data[6:12]
        # timestamp_us = frame_data[12:20]  # Node boot time (not needed for rate analysis)
        rssi_signed = struct.unpack('<b', frame_data[20:21])[0]  # signed byte
        channel = frame_data[22]
        n_sub = frame_data[23]

        # Validate
        if channel < 1 or channel > 14:
            return  # Invalid channel

        expected_len = self.FRAME_HEADER_SIZE + n_sub * 2
        if len(frame_data) != expected_len:
            return  # Payload mismatch

        # Format MACs
        node_mac_str = self._format_mac(node_mac)
        peer_mac_str = self._format_mac(peer_mac)
        link_id = f"{node_mac_str}:{peer_mac_str}"

        # Convert timestamp to seconds (for output)
        timestamp_s = recv_time_ns / 1_000_000_000

        # Update statistics
        self.total_frames += 1
        self.frame_timestamps.append(timestamp_s)
        self.peer_macs.add(peer_mac_str)
        self.ap_candidates[peer_mac_str] += 1

        # Categorize as beacon or data
        # Beacon frames come from the AP (peer_mac == ap_bssid)
        is_beacon = (self.ap_bssid and peer_mac_str == self.ap_bssid)

        if is_beacon:
            self.beacon_frames += 1
            frame_type = "beacon"
        else:
            self.data_frames += 1
            frame_type = "data"

        # Update link stats
        self.link_stats[link_id]["frame_count"] += 1
        self.link_stats[link_id]["rssi_values"].append(rssi_signed)
        self.link_stats[link_id]["channels"][channel] += 1
        self.link_stats[link_id]["node_mac"] = node_mac_str
        self.link_stats[link_id]["peer_mac"] = peer_mac_str

        # Store frame record for CSV output
        self.frame_records.append({
            "timestamp_ns": recv_time_ns,
            "timestamp_s": timestamp_s,
            "node_mac": node_mac_str,
            "peer_mac": peer_mac_str,
            "rssi_dbm": rssi_signed,
            "channel": channel,
            "n_sub": n_sub,
            "frame_type": frame_type,
            "link_id": link_id
        })

    def _format_mac(self, mac_bytes: bytes) -> str:
        """Format 6-byte MAC as uppercase colon-separated hex."""
        return ":".join(f"{b:02X}" for b in mac_bytes)

    def generate_results(self) -> Dict:
        """Generate the results dictionary."""
        if self.total_frames == 0:
            return {
                "error": "No CSI frames found in replay file",
                "replay_file": self.replay_file
            }

        # Calculate duration
        duration_ns = self.last_timestamp_ns - self.first_timestamp_ns
        duration_s = duration_ns / 1_000_000_000 if duration_ns > 0 else 0

        # Calculate per-link statistics
        links = {}
        for link_id, stats in self.link_stats.items():
            if stats["frame_count"] == 0:
                continue

            rssi_values = stats["rssi_values"]
            avg_rssi = sum(rssi_values) / len(rssi_values) if rssi_values else 0
            min_rssi = min(rssi_values) if rssi_values else 0
            max_rssi = max(rssi_values) if rssi_values else 0
            most_common_channel = max(stats["channels"].items(), key=lambda x: x[1])[0] if stats["channels"] else 0

            links[link_id] = {
                "node_mac": stats["node_mac"],
                "peer_mac": stats["peer_mac"],
                "frame_count": stats["frame_count"],
                "avg_rssi_dbm": round(avg_rssi, 2),
                "min_rssi_dbm": min_rssi,
                "max_rssi_dbm": max_rssi,
                "avg_channel": most_common_channel
            }

        # Convert timestamps to ISO format for JSON
        timestamp_isos = [datetime.fromtimestamp(ts, tz=timezone.utc).isoformat()
                         for ts in self.frame_timestamps[:1000]]  # Limit to 1000 for JSON size

        start_time_iso = datetime.fromtimestamp(self.first_timestamp_ns / 1_000_000_000, tz=timezone.utc).isoformat() if self.first_timestamp_ns else None
        end_time_iso = datetime.fromtimestamp(self.last_timestamp_ns / 1_000_000_000, tz=timezone.utc).isoformat() if self.last_timestamp_ns else None

        return {
            "start_time": start_time_iso,
            "end_time": end_time_iso,
            "duration_s": round(duration_s, 3),
            "total_frames": self.total_frames,
            "frames_per_second": round(self.total_frames / duration_s, 2) if duration_s > 0 else 0,
            "beacon_frames": self.beacon_frames,
            "beacon_rate_hz": round(self.beacon_frames / duration_s, 2) if duration_s > 0 else 0,
            "data_frames": self.data_frames,
            "data_rate_hz": round(self.data_frames / duration_s, 2) if duration_s > 0 else 0,
            "ap_bssid": self.ap_bssid or "auto-detected-from-missing",
            "unique_links": len(self.link_stats),
            "links": links,
            "frame_timestamps": timestamp_isos
        }

    def write_json(self, results: Dict, output_file: Optional[str]) -> None:
        """Write results as JSON."""
        output = json.dumps(results, indent=2)
        if output_file:
            with open(output_file, 'w') as f:
                f.write(output)
            print(f"Results written to {output_file}", file=sys.stderr)
        else:
            print(output)

    def write_csv(self, results: Dict, output_file: Optional[str]) -> None:
        """Write results as CSV (one row per frame)."""
        output_f = open(output_file, 'w') if output_file else sys.stdout

        try:
            writer = csv.writer(output_f)
            writer.writerow([
                "timestamp_iso", "node_mac", "peer_mac", "rssi_dbm",
                "channel", "n_sub", "frame_type", "link_id"
            ])

            # Write all frame records
            for record in self.frame_records:
                timestamp_iso = datetime.fromtimestamp(record["timestamp_s"], tz=timezone.utc).isoformat()
                writer.writerow([
                    timestamp_iso,
                    record["node_mac"],
                    record["peer_mac"],
                    record["rssi_dbm"],
                    record["channel"],
                    record["n_sub"],
                    record["frame_type"],
                    record["link_id"]
                ])

            if output_file:
                print(f"CSV written to {output_file} ({len(self.frame_records)} frames)", file=sys.stderr)

        finally:
            if output_file and output_f != sys.stdout:
                output_f.close()


def main():
    parser = argparse.ArgumentParser(
        description="Measure CSI frame rate from replay store file",
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--duration",
        type=int,
        default=300,
        help="Measurement duration in seconds (default: 300 = 5 minutes)"
    )
    parser.add_argument(
        "--output",
        type=str,
        help="Output file path (default: stdout)"
    )
    parser.add_argument(
        "--format",
        choices=["json", "csv"],
        default="json",
        help="Output format (default: json)"
    )
    parser.add_argument(
        "--replay-file",
        type=str,
        default="/data/spaxel/csi_replay.bin",
        help="Path to CSI replay file (default: /data/spaxel/csi_replay.bin)"
    )
    parser.add_argument(
        "--ap-bssid",
        type=str,
        help="AP BSSID to classify beacon frames (format: AA:BB:CC:DD:EE:FF). Auto-detected if not set."
    )

    args = parser.parse_args()

    reader = CSIReplayReader(args.replay_file, ap_bssid=args.ap_bssid)

    print(f"Reading CSI replay file: {args.replay_file}", file=sys.stderr)
    print(f"Duration: {args.duration} seconds", file=sys.stderr)

    if reader.read_file(args.duration):
        # Generate and write results
        results = reader.generate_results()

        if args.format == "csv":
            reader.write_csv(results, args.output)
        else:
            reader.write_json(results, args.output)

        print(f"\nSummary:", file=sys.stderr)
        print(f"  Total frames: {results.get('total_frames', 0)}", file=sys.stderr)
        print(f"  Frames/sec: {results.get('frames_per_second', 0)}", file=sys.stderr)
        print(f"  Beacon frames: {results.get('beacon_frames', 0)} ({results.get('beacon_rate_hz', 0)} Hz)", file=sys.stderr)
        print(f"  Data frames: {results.get('data_frames', 0)} ({results.get('data_rate_hz', 0)} Hz)", file=sys.stderr)
        print(f"  Unique links: {results.get('unique_links', 0)}", file=sys.stderr)
    else:
        print("Failed to read CSI replay file", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
