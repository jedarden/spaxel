# WiSee: Whole-Home Gesture Recognition Using Wireless Signals

**Authors:** Qifan Pu, Sidhant Gupta, Shyamnath Gollakota, Shwetak Patel
**Venue:** ACM MobiCom 2013 **(Best Paper Award)**
**DOI:** [10.1145/2500423.2500436](https://doi.org/10.1145/2500423.2500436)
**PDF:** https://wisee.cs.washington.edu/wisee_paper.pdf
**Institution:** University of Washington

---

## Citation

```
@inproceedings{pu2013wisee,
  title     = {Whole-Home Gesture Recognition Using Wireless Signals},
  author    = {Pu, Qifan and Gupta, Sidhant and Gollakota, Shyamnath and Patel, Shwetak},
  booktitle = {Proceedings of the 19th Annual International Conference on Mobile Computing and Networking},
  series    = {MobiCom '13},
  year      = {2013},
  pages     = {27--38},
  doi       = {10.1145/2500423.2500436},
  publisher = {ACM}
}
```

---

## Abstract

> "This paper presents WiSee, a novel gesture recognition system that leverages wireless signals (e.g., Wi-Fi) to enable whole-home sensing and recognition of human gestures. Since wireless signals do not require line-of-sight and can traverse through walls, WiSee can enable whole-home gesture recognition using few wireless sources. Further, it achieves this goal without requiring instrumentation of the human body with sensing devices. We implement a proof-of-concept prototype of WiSee using USRP-N210s and evaluate it in both an office environment and a two-bedroom apartment. Our results show that WiSee can identify and classify a set of nine gestures with an average accuracy of 94%."

---

## Problem Statement

Computing is increasingly ambient — users need to interact with devices throughout a home without carrying devices or maintaining line-of-sight. Camera-based (Kinect) systems need LoS. On-body sensors (wristbands) are inconvenient. WiSee exploits existing WiFi infrastructure: signals permeate homes, traverse walls, and carry Doppler information from human motion.

Key challenge: hand gestures at 0.5 m/s produce only **17 Hz Doppler shift** on a 5 GHz WiFi signal, while WiFi bandwidth is 20 MHz — the Doppler shift is **6 orders of magnitude smaller** than the signal bandwidth. Standard Wi-Fi processing discards this information entirely.

---

## Core Technique: Narrowband Pulse Creation from OFDM

WiSee transforms the wideband 802.11 OFDM signal (20 MHz) into a narrowband signal of a few Hz bandwidth, making gesture Doppler detectable.

### Case 1 — Identical OFDM Symbols

Performing an MN-point FFT over M repeated OFDM symbols of N subcarriers:
```
X_{2l}   = 2 · Σ_{k=1}^{N} x_k · e^{−i2πkl/N}
X_{2l+1} = 0
```
Odd sub-channels cancel; even sub-channels capture N-point FFT. Each sub-channel bandwidth is **halved** per repetition. With M=1000 symbols over 1 second → 1 Hz bandwidth, enabling Hertz-resolution Doppler measurement.

### Case 2 — Arbitrary (Real) OFDM Symbols

WiSee decodes each symbol with the standard 802.11 decoder, then **re-encodes** every symbol to match the first by multiplying each sub-channel n in symbol i by `X¹_n / Xⁱ_n`. After IFFT, all symbols become identical → reduces to Case 1.

Critical: the re-encoder re-introduces phase/amplitude changes that the decoder removed, preserving gesture Doppler information while normalising the data content.

---

## Gesture Classification

### Doppler Extraction

1. Receiver computes half-second FFTs with 5 ms sliding intervals → frequency-time Doppler profile
2. Human gestures at 0.25–4 m/s → Doppler shifts of 8–134 Hz at 5 GHz
3. Segmentation: detect gesture start/end when energy ratio crosses 3 dB threshold

### Gesture Encoding

Each gesture = unique sequence of positive (+) and negative (−) Doppler shift segments:
- Positive-only (+1): motion toward receiver
- Negative-only (−1): motion away from receiver
- Mixed (+2): simultaneous toward and away (e.g., arm sweep)

**9 gestures:** push, pull, sweep, flower, circle, horizontal S-curve, vertical S-curve, left jab, right jab. Each encodes as a distinct +/− pattern independent of user speed.

Pattern matching against templates. Speed changes duration but not the +/− sequence.

### Frequency Offset Handling

Track the maximum-energy peak (DC component from non-human static paths) and correct Doppler peaks relative to it — residual carrier frequency offset shifts all peaks equally.

---

## Multi-User Isolation via MIMO Nulling

- Target user performs a repetitive "preamble" gesture sequence
- Receiver estimates the MIMO channel that maximises energy from that user's reflections
- Subsequent gestures classified using that channel estimate
- With 4-antenna receiver: handles up to 3 simultaneous other users

**False positive rate** (24-hour continuous test):
- 2-gesture preamble: 2.63 events/hour
- 4-repetition preamble: **0.07 events/hour**

---

## Results

| Metric | Value |
|---|---|
| Average gesture accuracy | **94%** |
| Gestures | 9 whole-body gestures |
| Users evaluated | 5 |
| Test instances | 900 |
| Coverage (1 Tx in living room, 4-antenna Rx) | 94% accuracy in 60% of home |
| Coverage (2 Tx, 4-antenna Rx) | 94% accuracy in **all rooms** |
| False positive rate (4-rep preamble) | 0.07 events/hour |

---

## Limitations

- Prototype uses USRP-N210 software-defined radio, not commodity WiFi — requires custom narrowband pulse creation not available in standard chips
- Classification accuracy reduces with more simultaneous users (MIMO nulling has finite capacity)
- Only 9 pre-defined gesture classes — arbitrary gestures require extensions (HMM, DTW suggested)
- Requires minimum 3% channel occupancy (transmitter must be sending packets)
- Does not identify which room the gesture occurred in
- Requires preamble gesture to lock onto target user

---

## Relevance to Spaxel

WiSee demonstrates that Doppler-based sensing through walls is achievable with a single WiFi source. The narrowband pulse creation technique reveals the sensitivity floor: 17 Hz at 5 GHz for hand gestures. At 2.4 GHz (Spaxel's band), the same gesture produces ~8 Hz. This sets the minimum sample rate and processing window length requirements. WiSee's +/− Doppler pattern encoding is a simple but effective approach to motion classification that Spaxel could adopt for gross motion event detection (entry/exit, active/passive).
