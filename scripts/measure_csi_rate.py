#!/usr/bin/env python3
"""
CSI Frame Rate Measurement Script for Ambient Traffic Analysis

Connects to the Spaxel mothership WebSocket endpoint, collects CSI frame statistics,
and categorizes frames as beacon (from AP) vs data (from other nodes).

Usage:
    python scripts/measure_csi_rate.py [OPTIONS]

Options:
    --duration SECONDS    Measurement duration in seconds (default: 300 = 5 minutes)
    --output FILE         Output file path (default: stdout)
    --format FORMAT       Output format: json or csv (default: json)
    --ap-bssid MAC        AP BSSID to classify beacon frames (auto-detected if not set)
    --url URL             WebSocket URL (default: ws://localhost:8080/ws/node)
    --help                Show this help message

Output (JSON):
    {
        "start_time": "2024-03-15T14:32:05Z",
        "end_time": "2024-03-15T14:37:05Z",
        "duration_s": 300,
        "total_frames": 15234,
        "frames_per_second": 50.78,
        "beacon_frames": 12045,
        "beacon_rate_hz": 40.15,
        "data_frames": 3189,
        "data_rate_hz": 10.63,
        "ap_bssid": "AA:BB:CC:DD:EE:FF",
        "links": {
            "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55": {
                "peer_mac": "00:11:22:33:44:55",
                "frame_count": 15234,
                "avg_rssi_dbm": -45.2,
                "avg_channel": 6
            }
        },
        "frame_timestamps": [1710505925.123, 1710505925.145, ...]
    }

Output (CSV):
    timestamp,frame_type,peer_mac,rssi_dbm,channel,link_id
    2024-03-15T14:32:05.123Z,beacon,AA:BB:CC:DD:EE:FF,-45,6,AA:BB:CC:DD:EE:FF:00:11:22:33:44:55
    ...

This is part of ADR-003 / bead spaxel-76601bae.
"""

import argparse
import asyncio
import json
import sys
import csv
from datetime import datetime, timezone
from collections import defaultdict
from typing import Dict, List, Optional

try:
    import websockets
    from websockets.exceptions import ConnectionClosed
except ImportError:
    print("Error: websockets package required. Install with: pip install websockets", file=sys.stderr)
    sys.exit(1)


class CSIMeasurer:
    """Collects and categorizes CSI frame statistics from WebSocket stream."""

    def __init__(self, ap_bssid: Optional[str] = None):
        self.ap_bssid = ap_bssid  # AP BSSID to classify beacon frames
        self.start_time: Optional[datetime] = None
        self.end_time: Optional[datetime] = None

        # Statistics
        self.total_frames = 0
        self.beacon_frames = 0
        self.data_frames = 0
        self.frame_timestamps: List[float] = []

        # Per-link statistics
        self.link_stats: Dict[str, Dict] = defaultdict(lambda: {
            "frame_count": 0,
            "rssi_sum": 0.0,
            "channels": defaultdict(int),
            "peer_mac": ""
        })

        # Track peer MACs for auto-detection
        self.peer_macs: set = set()
        self.ap_candidates: Dict[str, int] = defaultdict(int)  # peer_mac -> frame count

    async def measure(self, url: str, duration: int, output_file: Optional[str] = None,
                    output_format: str = "json") -> None:
        """Run the measurement for specified duration."""
        print(f"Starting CSI measurement for {duration} seconds...", file=sys.stderr)
        print(f"Connecting to: {url}", file=sys.stderr)

        try:
            async with websockets.connect(url) as websocket:
                self.start_time = datetime.now(timezone.utc)
                print(f"Connected at {self.start_time.isoformat()}", file=sys.stderr)

                # Collection loop
                await self._collect_frames(websocket, duration)

                self.end_time = datetime.now(timezone.utc)

                # Auto-detect AP BSSID if not set
                if self.ap_bssid is None and self.ap_candidates:
                    # AP is likely the peer MAC with the most frames (beacon source)
                    self.ap_bssid = max(self.ap_candidates.items(), key=lambda x: x[1])[0]
                    print(f"Auto-detected AP BSSID: {self.ap_bssid}", file=sys.stderr)

                # Generate output
                results = self._generate_results()
                self._write_results(results, output_file, output_format)

                print(f"Measurement complete. Total frames: {self.total_frames}", file=sys.stderr)
                print(f"  Beacon frames: {self.beacon_frames} ({self.beacon_frames/duration:.2f} Hz)", file=sys.stderr)
                print(f"  Data frames: {self.data_frames} ({self.data_frames/duration:.2f} Hz)", file=sys.stderr)

        except ConnectionClosed as e:
            print(f"Connection closed: {e}", file=sys.stderr)
            sys.exit(1)
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)
            sys.exit(1)

    async def _collect_frames(self, websocket, duration: int) -> None:
        """Collect CSI frames for the specified duration."""
        start_timestamp = asyncio.get_event_loop().time()

        while True:
            # Check timeout
            elapsed = asyncio.get_event_loop().time() - start_timestamp
            if elapsed >= duration:
                break

            # Set receive timeout with small slack
            remaining = duration - elapsed
            try:
                data = await asyncio.wait_for(
                    websocket.recv(),
                    timeout=min(1.0, remaining + 0.1)
                )
            except asyncio.TimeoutError:
                continue

            # Process frame
            if isinstance(data, bytes):
                self._process_frame(data)
            # Ignore JSON messages (hello, health, ble, etc.)

    def _process_frame(self, data: bytes) -> None:
        """Parse and categorize a CSI binary frame."""
        if len(data) < 24:
            return  # Too short to be valid

        # Parse header (24 bytes)
        node_mac = data[0:6]
        peer_mac = data[6:12]
        timestamp_us = int.from_bytes(data[12:20], 'little')
        rssi = int.from_bytes(data[20:21], 'signed', byteorder='little')
        channel = data[22]
        n_sub = data[23]

        # Validate
        if channel == 0 or channel > 14:
            return  # Invalid channel

        expected_len = 24 + n_sub * 2
        if len(data) != expected_len:
            return  # Payload mismatch

        # Format MACs
        node_mac_str = self._format_mac(node_mac)
        peer_mac_str = self._format_mac(peer_mac)
        link_id = f"{node_mac_str}:{peer_mac_str}"

        # Timestamp in seconds (node boot time, approximate)
        # Use current time for wall-clock timestamp
        wall_time = datetime.now(timezone.utc).timestamp()

        # Update statistics
        self.total_frames += 1
        self.frame_timestamps.append(wall_time)
        self.peer_macs.add(peer_mac_str)
        self.ap_candidates[peer_mac_str] += 1

        # Categorize as beacon or data
        is_beacon = (self.ap_bssid and peer_mac_str == self.ap_bssid)

        if is_beacon:
            self.beacon_frames += 1
        else:
            self.data_frames += 1

        # Update link stats
        self.link_stats[link_id]["frame_count"] += 1
        self.link_stats[link_id]["rssi_sum"] += rssi
        self.link_stats[link_id]["channels"][channel] += 1
        self.link_stats[link_id]["peer_mac"] = peer_mac_str

    def _format_mac(self, mac_bytes: bytes) -> str:
        """Format 6-byte MAC as uppercase colon-separated hex."""
        return ":".join(f"{b:02X}" for b in mac_bytes)

    def _generate_results(self) -> Dict:
        """Generate the results dictionary."""
        duration_s = (self.end_time - self.start_time).total_seconds()

        # Calculate per-link averages
        links = {}
        for link_id, stats in self.link_stats.items():
            avg_rssi = stats["rssi_sum"] / stats["frame_count"] if stats["frame_count"] > 0 else 0
            most_common_channel = max(stats["channels"].items(), key=lambda x: x[1])[0] if stats["channels"] else 0

            links[link_id] = {
                "peer_mac": stats["peer_mac"],
                "frame_count": stats["frame_count"],
                "avg_rssi_dbm": round(avg_rssi, 2),
                "avg_channel": most_common_channel
            }

        return {
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat(),
            "duration_s": duration_s,
            "total_frames": self.total_frames,
            "frames_per_second": round(self.total_frames / duration_s, 2) if duration_s > 0 else 0,
            "beacon_frames": self.beacon_frames,
            "beacon_rate_hz": round(self.beacon_frames / duration_s, 2) if duration_s > 0 else 0,
            "data_frames": self.data_frames,
            "data_rate_hz": round(self.data_frames / duration_s, 2) if duration_s > 0 else 0,
            "ap_bssid": self.ap_bssid or "auto-detected-from-missing",
            "links": links,
            "frame_timestamps": self.frame_timestamps  # Wall-clock times
        }

    def _write_results(self, results: Dict, output_file: Optional[str], format: str) -> None:
        """Write results to file or stdout."""
        if format == "csv":
            self._write_csv(results, output_file)
        else:
            self._write_json(results, output_file)

    def _write_json(self, results: Dict, output_file: Optional[str]) -> None:
        """Write results as JSON."""
        output = json.dumps(results, indent=2)
        if output_file:
            with open(output_file, 'w') as f:
                f.write(output)
            print(f"Results written to {output_file}", file=sys.stderr)
        else:
            print(output)

    def _write_csv(self, results: Dict, output_file: Optional[str]) -> None:
        """Write results as CSV (one row per timestamp)."""
        # Reconstruct per-frame data from timestamps and stats
        # This is an approximation since we didn't store per-frame detail

        writer = None
        output_f = open(output_file, 'w') if output_file else sys.stdout

        try:
            writer = csv.writer(output_f)
            writer.writerow(["timestamp", "frame_type", "peer_mac", "rssi_dbm", "channel", "link_id"])

            # Distribute frame types proportionally across timestamps
            beacon_ratio = results["beacon_frames"] / results["total_frames"] if results["total_frames"] > 0 else 0

            # Use link stats to write representative rows
            for link_id, link_stats in results["links"].items():
                peer_mac = link_stats["peer_mac"]
                avg_rssi = link_stats["avg_rssi_dbm"]
                avg_channel = link_stats["avg_channel"]
                frame_count = link_stats["frame_count"]

                # Determine if this is a beacon link
                is_beacon = (results["ap_bssid"] and peer_mac == results["ap_bssid"])
                frame_type = "beacon" if is_beacon else "data"

                # Write one row per frame (approximate - distribute timestamps)
                frames_for_link = min(frame_count, len(results["frame_timestamps"]))
                for i in range(frames_for_link):
                    ts = results["frame_timestamps"][i] if i < len(results["frame_timestamps"]) else ""
                    if ts:
                        ts_dt = datetime.fromtimestamp(ts, timezone.utc).isoformat()
                    else:
                        ts_dt = ""

                    writer.writerow([ts_dt, frame_type, peer_mac, avg_rssi, avg_channel, link_id])

            if output_file:
                print(f"CSV written to {output_file}", file=sys.stderr)

        finally:
            if output_file and output_f != sys.stdout:
                output_f.close()


def main():
    parser = argparse.ArgumentParser(
        description="Measure CSI frame rate and categorize frames (beacon vs data)",
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
        "--ap-bssid",
        type=str,
        help="AP BSSID to classify beacon frames (format: AA:BB:CC:DD:EE:FF). Auto-detected if not set."
    )
    parser.add_argument(
        "--url",
        type=str,
        default="ws://localhost:8080/ws/node",
        help="WebSocket URL to connect to (default: ws://localhost:8080/ws/node)"
    )

    args = parser.parse_args()

    measurer = CSIMeasurer(ap_bssid=args.ap_bssid)

    try:
        asyncio.run(measurer.measure(
            url=args.url,
            duration=args.duration,
            output_file=args.output,
            output_format=args.format
        ))
    except KeyboardInterrupt:
        print("\nMeasurement interrupted by user", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
