#!/usr/bin/env python3
"""
CSI Frame Rate Measurement Script for Spaxel

This script reads CSI frames from the mothership's replay store and produces
statistics about frame arrival rates, categorizing frames as beacon vs data.

Usage:
    python scripts/measure_csi_rate.py [OPTIONS]

Options:
    --duration SECONDS    Measurement duration in seconds (default: 300 = 5 minutes)
    --output PATH         Output file path (default: stdout)
    --format FORMAT       Output format: json or csv (default: json)
    --replay-path PATH    Path to CSI replay store (default: /data/csi_replay.bin)
    --ap-bssid MAC        AP BSSID for beacon detection (format: AA:BB:CC:DD:EE:FF)
                          If not provided, beacons are detected automatically

Output (JSON):
    {
        "measurement_start_ms": 1234567890,
        "measurement_end_ms": 1234567890,
        "duration_seconds": 300,
        "total_frames": 12345,
        "frames_per_second": 41.15,
        "beacon_frames": 8234,
        "beacon_rate_hz": 27.45,
        "data_frames": 4111,
        "data_rate_hz": 13.70,
        "links": {
            "AA:BB:CC:DD:EE:FF": {
                "frames": 2345,
                "rate_hz": 7.82,
                "type": "beacon"
            }
        }
    }
"""

import argparse
import struct
import json
import csv
import sys
import os
import time
from datetime import datetime
from pathlib import Path
from typing import Dict, Optional, Tuple


# Constants from the CSI replay store format
FILE_MAGIC = b"SPAXLREC"
HEADER_SIZE = 32
RECORD_OVERHEAD = 10  # recvTimeNS(8) + frameLen(2)
CSI_HEADER_SIZE = 24


class CSIReplayReader:
    """Read CSI frames from the mothership's replay store."""

    def __init__(self, replay_path: str):
        self.replay_path = Path(replay_path)
        self.file = None
        self.file_size = 0
        self.write_pos = 0
        self.oldest_pos = 0

    def __enter__(self):
        self.file = open(self.replay_path, 'rb')
        self._read_header()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if self.file:
            self.file.close()

    def _read_header(self):
        """Read and validate the replay store header."""
        header_data = self.file.read(HEADER_SIZE)
        if len(header_data) < HEADER_SIZE:
            raise ValueError(f"File too small to contain header: {len(header_data)} bytes")

        magic = header_data[:8]
        if magic != FILE_MAGIC:
            raise ValueError(f"Invalid file magic: {magic}")

        self.write_pos = struct.unpack('<Q', header_data[8:16])[0]
        self.oldest_pos = struct.unpack('<Q', header_data[16:24])[0]

        self.file_size = os.fstat(self.file.fileno()).st_size

        # Check if file has data
        if self.oldest_pos == 0:
            raise ValueError("Replay store is empty (no data recorded)")

    def scan_frames(self, duration_seconds: Optional[float] = None):
        """
        Scan CSI frames from the replay store.

        Yields tuples of (recv_time_ms, frame_bytes) for each frame.
        Stops after duration_seconds if provided.
        """
        if self.oldest_pos == 0:
            return

        current_pos = self.oldest_pos
        start_time = None
        end_time_ns = None

        if duration_seconds:
            end_time_ns = time.time_ns() + int(duration_seconds * 1e9)

        while current_pos < self.write_pos:
            # Seek to current record position
            self.file.seek(current_pos)

            # Read record header
            record_header = self.file.read(RECORD_OVERHEAD)
            if len(record_header) < RECORD_OVERHEAD:
                # Reached end or incomplete record
                break

            recv_time_ns = struct.unpack('<q', record_header[:8])[0]
            frame_len = struct.unpack('<H', record_header[8:10])[0]

            # Check if we've exceeded duration
            if start_time is None:
                start_time = recv_time_ns
            if end_time_ns and recv_time_ns > end_time_ns:
                break

            # Read frame data
            frame_data = self.file.read(frame_len)
            if len(frame_data) < frame_len:
                # Incomplete frame
                break

            recv_time_ms = recv_time_ns // 1_000_000
            yield (recv_time_ms, frame_data)

            # Move to next record
            current_pos += RECORD_OVERHEAD + frame_len

            # Handle wrap-around
            if current_pos >= self.file_size:
                current_pos = HEADER_SIZE


class CSIFrameParser:
    """Parse CSI binary frames."""

    @staticmethod
    def parse_frame(frame_data: bytes) -> Dict:
        """Parse a CSI frame and return its fields as a dictionary."""
        if len(frame_data) < CSI_HEADER_SIZE:
            return None

        frame = {}

        # Parse header
        frame['node_mac'] = ':'.join(f'{b:02X}' for b in frame_data[0:6])
        frame['peer_mac'] = ':'.join(f'{b:02X}' for b in frame_data[6:12])
        frame['timestamp_us'] = struct.unpack('<Q', frame_data[12:20])[0]
        frame['rssi'] = struct.unpack('<b', frame_data[20:21])[0]
        frame['noise_floor'] = struct.unpack('<b', frame_data[21:22])[0]
        frame['channel'] = struct.unpack('<B', frame_data[22:23])[0]
        frame['n_sub'] = struct.unpack('<B', frame_data[23:24])[0]

        return frame


class CSIFrameAnalyzer:
    """Analyze CSI frames and produce statistics."""

    def __init__(self, ap_bssid: Optional[str] = None):
        self.ap_bssid = ap_bssid
        self.frames = []
        self.links: Dict[str, Dict] = {}

    def add_frame(self, timestamp_ms: int, frame: Dict):
        """Add a frame to the analysis."""
        self.frames.append({
            'timestamp_ms': timestamp_ms,
            'node_mac': frame['node_mac'],
            'peer_mac': frame['peer_mac'],
            'rssi': frame['rssi'],
            'channel': frame['channel'],
            'n_sub': frame['n_sub']
        })

        # Track per-link statistics
        link_id = f"{frame['node_mac']}:{frame['peer_mac']}"
        if link_id not in self.links:
            self.links[link_id] = {
                'frames': 0,
                'rssi_sum': 0,
                'type': 'unknown'
            }

        self.links[link_id]['frames'] += 1
        self.links[link_id]['rssi_sum'] += frame['rssi']

    def categorize_frames(self):
        """Categorize frames as beacon or data."""
        if not self.frames:
            return

        # Detect AP BSSID if not provided
        if self.ap_bssid is None:
            # Auto-detect: AP is the most common peer MAC
            from collections import Counter
            peer_counts = Counter(f['peer_mac'] for f in self.frames)
            if peer_counts:
                self.ap_bssid = peer_counts.most_common(1)[0][0]

        # Categorize frames
        for link_id, link_stats in self.links.items():
            _, peer_mac = link_id.split(':', 1)
            if peer_mac == self.ap_bssid:
                link_stats['type'] = 'beacon'
            else:
                link_stats['type'] = 'data'

    def calculate_statistics(self) -> Dict:
        """Calculate final statistics."""
        if not self.frames:
            return {
                'total_frames': 0,
                'frames_per_second': 0.0,
                'beacon_frames': 0,
                'beacon_rate_hz': 0.0,
                'data_frames': 0,
                'data_rate_hz': 0.0,
                'links': {}
            }

        timestamps = [f['timestamp_ms'] for f in self.frames]
        start_ms = min(timestamps)
        end_ms = max(timestamps)
        duration_sec = (end_ms - start_ms) / 1000.0

        total_frames = len(self.frames)
        frames_per_second = total_frames / duration_sec if duration_sec > 0 else 0.0

        # Count beacon and data frames
        beacon_frames = sum(1 for l in self.links.values() if l['type'] == 'beacon')
        data_frames = sum(1 for l in self.links.values() if l['type'] == 'data')

        # Recalculate based on actual frame counts per link
        beacon_count = sum(l['frames'] for l in self.links.values() if l['type'] == 'beacon')
        data_count = sum(l['frames'] for l in self.links.values() if l['type'] == 'data')

        beacon_rate = beacon_count / duration_sec if duration_sec > 0 else 0.0
        data_rate = data_count / duration_sec if duration_sec > 0 else 0.0

        # Build per-link statistics
        links_output = {}
        for link_id, link_stats in self.links.items():
            node_mac, peer_mac = link_id.split(':', 1)
            links_output[link_id] = {
                'node_mac': node_mac,
                'peer_mac': peer_mac,
                'frames': link_stats['frames'],
                'rate_hz': link_stats['frames'] / duration_sec if duration_sec > 0 else 0.0,
                'avg_rssi': link_stats['rssi_sum'] / link_stats['frames'] if link_stats['frames'] > 0 else 0,
                'type': link_stats['type']
            }

        return {
            'measurement_start_ms': start_ms,
            'measurement_end_ms': end_ms,
            'duration_seconds': duration_sec,
            'total_frames': total_frames,
            'frames_per_second': round(frames_per_second, 2),
            'beacon_frames': beacon_count,
            'beacon_rate_hz': round(beacon_rate, 2),
            'data_frames': data_count,
            'data_rate_hz': round(data_rate, 2),
            'ap_bssid': self.ap_bssid,
            'links': links_output
        }


def format_timestamp(ms: int) -> str:
    """Format millisecond timestamp as ISO8601 string."""
    return datetime.fromtimestamp(ms / 1000.0).isoformat() + 'Z'


def output_json(stats: Dict, file=None):
    """Output statistics as JSON."""
    # Add human-readable timestamps
    stats['measurement_start_iso'] = format_timestamp(stats['measurement_start_ms'])
    stats['measurement_end_iso'] = format_timestamp(stats['measurement_end_ms'])

    json.dump(stats, file, indent=2)
    file.write('\n')


def output_csv(stats: Dict, file=None):
    """Output statistics as CSV."""
    writer = csv.writer(file)

    # Write summary
    writer.writerow(['Metric', 'Value'])
    writer.writerow(['Measurement Start', stats['measurement_start_iso']])
    writer.writerow(['Measurement End', stats['measurement_end_iso']])
    writer.writerow(['Duration (seconds)', round(stats['duration_seconds'], 2)])
    writer.writerow(['Total Frames', stats['total_frames']])
    writer.writerow(['Frames Per Second', stats['frames_per_second']])
    writer.writerow(['Beacon Frames', stats['beacon_frames']])
    writer.writerow(['Beacon Rate (Hz)', stats['beacon_rate_hz']])
    writer.writerow(['Data Frames', stats['data_frames']])
    writer.writerow(['Data Rate (Hz)', stats['data_rate_hz']])
    writer.writerow(['AP BSSID', stats.get('ap_bssid', 'N/A')])
    writer.writerow([])

    # Write per-link statistics
    writer.writerow(['Link', 'Node MAC', 'Peer MAC', 'Frames', 'Rate (Hz)', 'Avg RSSI (dBm)', 'Type'])
    for link_id, link_stats in stats['links'].items():
        writer.writerow([
            link_id,
            link_stats['node_mac'],
            link_stats['peer_mac'],
            link_stats['frames'],
            round(link_stats['rate_hz'], 2),
            round(link_stats['avg_rssi'], 1),
            link_stats['type']
        ])


def main():
    parser = argparse.ArgumentParser(
        description='Measure CSI frame rates from Spaxel replay store',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__
    )

    parser.add_argument(
        '--duration',
        type=float,
        default=300.0,
        help='Measurement duration in seconds (default: 300 = 5 minutes)'
    )

    parser.add_argument(
        '--output',
        type=str,
        default='-',
        help='Output file path (default: stdout)'
    )

    parser.add_argument(
        '--format',
        choices=['json', 'csv'],
        default='json',
        help='Output format (default: json)'
    )

    parser.add_argument(
        '--replay-path',
        type=str,
        default='/data/csi_replay.bin',
        help='Path to CSI replay store (default: /data/csi_replay.bin)'
    )

    parser.add_argument(
        '--ap-bssid',
        type=str,
        default=None,
        help='AP BSSID for beacon detection (format: AA:BB:CC:DD:EE:FF). Auto-detected if not provided.'
    )

    args = parser.parse_args()

    # Check if replay store exists
    if not os.path.exists(args.replay_path):
        print(f"Error: Replay store not found: {args.replay_path}", file=sys.stderr)
        print("The mothership may not have recorded any CSI data yet.", file=sys.stderr)
        sys.exit(1)

    # Initialize analyzer
    analyzer = CSIFrameAnalyzer(ap_bssid=args.ap_bssid)

    print(f"Starting CSI measurement from {args.replay_path}...", file=sys.stderr)
    print(f"Duration: {args.duration} seconds", file=sys.stderr)

    # Read and analyze frames
    try:
        with CSIReplayReader(args.replay_path) as reader:
            frame_count = 0
            for timestamp_ms, frame_data in reader.scan_frames(duration_seconds=args.duration):
                frame = CSIFrameParser.parse_frame(frame_data)
                if frame:
                    analyzer.add_frame(timestamp_ms, frame)
                    frame_count += 1

                    # Progress indicator
                    if frame_count % 1000 == 0:
                        print(f"Processed {frame_count} frames...", file=sys.stderr)

        print(f"Total frames processed: {frame_count}", file=sys.stderr)

    except ValueError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    # Categorize and calculate statistics
    analyzer.categorize_frames()
    stats = analyzer.calculate_statistics()

    # Output results
    if args.output == '-':
        output_file = sys.stdout
    else:
        output_file = open(args.output, 'w')

    try:
        if args.format == 'json':
            output_json(stats, output_file)
        else:
            output_csv(stats, output_file)
    finally:
        if args.output != '-':
            output_file.close()

    print(f"\nMeasurement complete. Results written to {args.output if args.output != '-' else 'stdout'}", file=sys.stderr)


if __name__ == '__main__':
    main()
