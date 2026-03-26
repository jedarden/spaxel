# An Overview on IEEE 802.11bf: WLAN Sensing

**Authors:** Rui Du, Hailiang Xie, Mengshi Hu, Narengerile, Yan Xin, Stephen McCann, Michael Montemurro, Tony Xiao Han, Jie Xu
**Venue:** arXiv:2207.04859 / IEEE Internet of Things Journal
**arXiv:** [2207.04859](https://arxiv.org/abs/2207.04859)

---

## Citation

```
@article{du2022overview,
  title   = {An Overview on IEEE 802.11bf: WLAN Sensing},
  author  = {Du, Rui and Xie, Hailiang and Hu, Mengshi and Narengerile and Xin, Yan and McCann, Stephen and Montemurro, Michael and Han, Tony Xiao and Xu, Jie},
  journal = {arXiv preprint arXiv:2207.04859},
  year    = {2022}
}
```

---

## Abstract

> "With recent advancements, the wireless local area network (WLAN) or wireless fidelity (Wi-Fi) technology has been successfully utilized to realize sensing functionalities such as detection, localization, and recognition. However, the WLANs standards are developed mainly for the purpose of communication, and thus may not be able to meet the stringent sensing requirements in emerging applications. To resolve this issue, a new Task Group (TG), namely IEEE 802.11bf, has been established by the IEEE 802.11 working group, with the objective of creating a new amendment to the WLAN standard to provide advanced sensing requirements while minimizing the effect on communications."

---

## Motivation: Why a Standard Was Needed

Current COTS WiFi limitations for sensing:

| Limitation | Impact |
|---|---|
| CSI unavailability | Most manufacturers keep CSI private; only Intel 5300 and Atheros AR9580 expose CSI — both discontinued |
| Transmission adaptation | Dynamic antenna config, power, AGC changes corrupt sensing measurements |
| Single-node only | No standard for multi-STA sensing cooperation |
| Communication degradation | Sensing operations may reduce throughput |

### WiFi vs. Other Sensing Technologies

| Technology | Coverage | Cost | Accuracy | Key Disadvantage |
|---|---|---|---|---|
| Visible Light | Room | Moderate | Low–moderate | LoS only |
| Ultrasound | Room | Moderate–high | Moderate–high | Interference |
| RFID | Room | Low | Low–moderate | High response time |
| UWB | Building | High | High | No existing infrastructure |
| Bluetooth | Building | Low–moderate | Low–moderate | Limited coverage |
| **Wi-Fi** | **Building** | **Low** | **Moderate–high** | **Needs standard modification** |

---

## Standardisation Timeline

| Date | Event |
|---|---|
| July 2019 | First discussion, IEEE 802.11 WNG SC |
| November 2019 | WLAN Sensing SG formed |
| September 2020 | IEEE 802.11bf PAR approved — TG officially formed |
| October 2020 | First TG meeting |
| April 2022 | Draft D0.1 released |
| September 2022 | Initial Letter Ballot (D1.0) |
| January 2023 | Recirculation LB (D2.0) |
| September 2023 | Initial SA Ballot (D4.0) |
| **May 2025** | **Final approval — IEEE 802.11bf published** |

---

## Use Cases and Key Performance Indicators (KPIs)

| Use Case | Range | Range Resolution | Velocity Resolution | Angular Resolution |
|---|---|---|---|---|
| Presence detection | ≤10–15 m | ≥0.5–2 m | ≥0.5 m/s | ≥4–6° |
| Activity recognition | ≤10 m | N/A | ≥0.1 m/s | ≥4° |
| Human localisation | ≤10 m | ≤0.2 m | ≤0.1 m/s | ≤2° |
| Respiration monitoring | ≤5 m | N/A | ≥0.01 m/s | N/A |
| 3D vision (60 GHz) | ≤5 m | ≤0.01 m | ≤0.1 m/s | ≤2° |

---

## 802.11bf Sensing Framework

### Transceiver Roles

- **Sensing Initiator (ISTA)**: requests/coordinates the sensing procedure
- **Sensing Responder (RSTA)**: participates in sensing at initiator's request
- **Sensing Transmitter**: transmits sensing signals (NDP frames)
- **Sensing Receiver**: receives and measures CSI from sensing signals

Four configurations: initiator = receiver; initiator = transmitter; initiator = both; initiator = neither (proxy).

### Five Phases (Sub-7 GHz)

1. **Sensing session setup** — capability exchange, assign session ID
2. **Sensing measurement setup** — define operational attributes, assign measurement setup IDs
3. **Sensing measurement instance** — actual CSI collection via NDP sounding
4. **Sensing measurement termination** — end measurement instance
5. **Sensing session termination** — close session

### Two Measurement Types (Sub-7 GHz)

**Non-Trigger-Based (Non-TB):**
- Initiated by sensing initiator
- Polling phase: NDPA frame announces upcoming NDP
- NDP (Null Data Packet) carries the sensing waveform (no user data)
- Responder sends back CSI feedback

**Trigger-Based (TB):**
- Uses Trigger Frames (TF) for sounding
- More flexible scheduling alongside communication traffic
- Threshold-based reporting reduces overhead

### DMG Sensing (60 GHz)

Five sensing modes:
- **Monostatic**: TX and RX collocated
- **Bistatic**: TX and RX separated
- **Multistatic**: one TX, multiple RXs
- **Monostatic with coordination**
- **Bistatic with coordination**
- **Passive sensing**: receiver only (no dedicated TX)

---

## Key Technical Features in Standard

### Waveform Design

The **ambiguity function (AF)** characterises the trade-off between range and Doppler resolution:
```
χ(τ, f_D) = ∫ s(t) · s*(t−τ) · e^{j2πf_D·t} dt
```
- Auto-ambiguity function (AAF): monostatic waveform self-correlation
- Cross-ambiguity function (CAF): bistatic TX-RX correlation

Existing 802.11 preamble sequences (HT-LTF, VHT-LTF, HE-LTF) evaluated as sensing waveforms. HE-LTF shows best ambiguity function properties.

### Feedback Types

- **CSI feedback**: Complex channel matrices per subcarrier (current WiFi convention, standardised)
- **TCIR (Truncated Channel Impulse Response)**: Time-domain equivalent — IFFT of CSI, truncated to significant taps. More compact for reporting.
- **Quantisation**: Tradeoff between CSI precision and feedback frame overhead. Typically 8-bit I/Q.

### Security and Privacy

- Authentication of sensing measurements (prevent spoofing of CSI reports)
- Access control (who may request sensing sessions)
- Protection against passive eavesdropping of sensing feedback frames
- Privacy concerns: CSI can detect presence and activity — access controls required

---

## Implications for Spaxel

802.11bf establishes official KPIs that Spaxel's design targets:
- **Presence detection KPI**: ≤15 m range, ≥0.5 m resolution — Spaxel targets this tier
- **Localisation KPI**: ≤0.2 m accuracy — achievable with 6+ nodes and better algorithms

The standardisation of sensing NDP frames means future WiFi hardware will expose sensing-quality CSI through standard APIs, eliminating the dependency on vendor-specific hacks (ESP32's `esp_wifi_set_csi`). Spaxel's architecture is designed to be forward-compatible: the mothership processes raw CSI regardless of source, making hardware upgrades transparent.
