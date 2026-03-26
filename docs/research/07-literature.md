# Literature and Paper References

Academic papers and tools relevant to WiFi CSI-based presence and position detection. Ordered roughly by publication date within each category.

---

## Foundational / Enabling Infrastructure

### Intel 5300 CSI Tool

> **Halperin, D., Hu, W., Sheth, A., & Wetherall, D.** (2011).
> *Tool Release: Gathering 802.11n Traces with Channel State Information.*
> ACM SIGCOMM Computer Communication Review, 41(1), 53–53.
> [PDF](https://www.halper.in/pubs/halperin_csitool.pdf) · [Tool](https://dhalperi.github.io/linux-80211n-csitool/)

The paper that released the Intel 5300 CSI Tool and defined the field. Describes the 30-subcarrier-group format, int8 complex values, and 802.11n measurement methodology. Virtually every subsequent paper in WiFi sensing used this tool or its derivatives.

---

### Nexmon CSI

> **Gringoli, F., Schulz, M., Link, J., & Hollick, M.** (2019).
> *Free Your CSI: A Channel State Information Extraction Platform For Modern Wi-Fi Chipsets.*
> ACM WiNTECH (co-located with MobiCom).
> [Project](https://nexmon.org/csi/)

Enables CSI extraction from Broadcom chipsets (BCM4339, BCM4366) used in Raspberry Pi and Nexus 5 phones. Provides up to 256 subcarriers at HT20, higher resolution than Intel 5300.

---

### ESPARGOS

> **Schulz, M., & Hollick, M.** (2024).
> *ESPARGOS: Phase-Coherent WiFi CSI Datasets for Wireless Sensing Experiments.*
> arXiv:2408.16377.
> [arXiv](https://arxiv.org/abs/2408.16377) · [Project](https://espargos.net/)

Eight ESP32 chips sharing a 40 MHz reference clock for phase-coherent antenna array processing. Enables MUSIC-based AoA estimation with commodity hardware. The only platform achieving true phase coherence across multiple ESP32 nodes.

---

## Localization

### FILA

> **Wu, K., Xiao, J., Yi, Y., Gao, M., & Ni, L. M.** (2012).
> *FILA: Fine-grained Indoor Localization.*
> IEEE INFOCOM, 2012.
> [ResearchGate](https://www.researchgate.net/publication/254031738_FILA_Fine-grained_indoor_localization)

First CSI-based model-driven indoor localization. Extracts the LOS path from the Channel Impulse Response (CIR = IFFT of CSI) to estimate Time of Arrival, mitigating multipath bias. Uses a log-distance path loss model calibrated to CSI. Achieves sub-metre average accuracy — a major improvement over RSSI-based ranging. Established that CSI > RSSI for ranging.

---

### SpotFi

> **Kotaru, M., Joshi, K., Bharadia, D., & Katti, S.** (2015).
> *SpotFi: Decimeter Level Localization Using WiFi.*
> ACM SIGCOMM, 2015.
> [PDF](https://web.stanford.edu/~skatti/pubs/sigcomm15-spotfi.pdf)

Joint AoA-ToF estimation using 2D MUSIC with the SpotFi subcarrier extension: treats each (antenna, subcarrier) pair as a virtual array element, enabling joint angle and time-of-flight estimation from a single commodity AP. Introduces spatial smoothing to handle coherent multipath. Achieves **40 cm median localization error**. One of the most-cited papers in WiFi localization.

---

### ArrayTrack

> **Xiong, J., & Jamieson, K.** (2013).
> *ArrayTrack: A Fine-Grained Indoor Location System.*
> USENIX NSDI, 2013.

AoA-based tracking using commercial 802.11n APs. Uses antenna arrays at the AP to estimate AoA of packets from mobile devices. Achieves ~23 cm median error with a 4-antenna AP.

---

### Widar

> **Qian, K., Wu, C., Yang, Z., Liu, Y., & Jamieson, K.** (2017).
> *Widar: Decimeter-Level Passive Tracking via Velocity Monitoring with Commodity Wi-Fi.*
> ACM MobiHoc, 2017.

Extracts Doppler Frequency Shift (DFS) from CSI time series to estimate the velocity vector of a passive (non-instrumented) target. Achieves decimeter-level passive tracking for moving targets. Tsinghua University TNS Lab.

---

### Widar2.0

> **Qian, K., Wu, C., Yang, Z., Liu, Y., Vasisht, D., & Jamieson, K.** (2018).
> *Widar2.0: Passive Human Tracking with a Single Wi-Fi Link.*
> ACM MobiSys, 2018.
> [PDF](https://www.cswu.me/papers/mobisys18_widar2.0_paper.pdf)

First single-link passive localization system. Jointly estimates AoA, ToF, and DFS from one TX-RX link using a unified signal model. Key insight: range resolution improves from 0.3 m to 0.05 m by combining ToF with cumulative DFS-derived path length changes. Results: **0.75 m median error** (1 link), **0.63 m** (2 links). Directly relevant to Spaxel's geometric approach.

---

### IndoTrack

> **Li, X., Li, S., Zhang, D., Xiong, J., Wang, Y., & Mei, H.** (2017).
> *IndoTrack: Device-Free Indoor Human Tracking with Commodity Wi-Fi.*
> ACM IMWUT (UbiComp), 1(3), 2017.
> [ACM DL](https://dl.acm.org/doi/10.1145/3130940)

Applies MUSIC to the Doppler frequency domain (Doppler-MUSIC) to resolve velocity components of multiple simultaneous people. Combined with probabilistic AoA estimation for trajectory reconstruction. Requires minimum 3 antennas. Achieves **35 cm median trajectory error** with commodity hardware. Among the most accurate commodity CSI tracking results.

---

## Gesture and Activity Recognition

### WiSee

> **Pu, Q., Gupta, S., Gollakota, S., & Patel, S.** (2013).
> *Whole-Home Gesture Recognition Using Wireless Signals.*
> ACM MobiCom, 2013.
> [PDF](https://wisee.cs.washington.edu/wisee_paper.pdf) · [Project](https://wisee.cs.washington.edu/)

Whole-home gesture recognition using WiFi Doppler shifts without CSI — converts narrowband WiFi into a Doppler radar. Classifies 9 whole-body gestures with **94% average accuracy**, demonstrated through walls. University of Washington. One of the first high-impact WiFi sensing papers.

---

### CARM

> **Wang, W., Liu, A. X., Shahzad, M., Ling, K., & Lu, S.** (2015).
> *Understanding and Modeling of WiFi Signal Based Human Activity Recognition.*
> ACM MobiCom, 2015.

Proposes a model relating physical motion to CSI changes via a CSI-speed model and CSI-activity model. Achieves high accuracy for 9 activities. Notable for providing a theoretical underpinning that most activity recognition papers lack.

---

### EI (Emotion / Intent)

> **Zhao, M., et al.** (2018).
> *Through-Wall Human Pose Estimation Using Radio Signals.*
> IEEE CVPR, 2018.

Uses FMCW radar (not WiFi) for through-wall pose estimation. Relevant as context for what dedicated sensing hardware can achieve that commodity WiFi cannot. Sets the ceiling for the "pose estimation" problem space.

---

## Pose and Body Sensing

### DensePose from WiFi

> **Geng, J., Huang, D., & De la Torre, F.** (2022/2023).
> *DensePose From WiFi.*
> arXiv:2301.00250.
> [arXiv](https://arxiv.org/abs/2301.00250)

CMU Robotics Institute. Maps WiFi phase and amplitude to UV body surface coordinates via a deep neural network, enabling body pose estimation using 3 antennas and 30 subcarriers. Uses a two-branch encoder-decoder with transformer and attention mechanisms. The academic precursor to RuView's overclaims. Requires massive labelled training data and controlled conditions — not replicable with commodity hardware without the same training setup.

---

### Person-in-WiFi

> **Wang, F., Zhou, S., Panev, S., Han, J., & Huang, D.** (2019).
> *Person-in-WiFi: Fine-Grained Person Perception Using WiFi.*
> IEEE ICCV, 2019.

Estimates 2D human body segmentation and joint locations from WiFi signals using a neural network. Uses 3 transmitters and 3 receivers in a controlled lab setting.

---

## Background Subtraction and Passive Detection

### DEFT

> **Xiao, J., Wu, K., Yi, Y., Wang, L., & Ni, L. M.** (2013).
> *FIFS: Fine-grained Indoor Fingerprinting System.*
> IEEE ICCCN, 2013.

Early fingerprinting-based approach. Uses amplitude variance across subcarriers as the spatial signature. Relevant as a baseline comparison for geometric methods.

---

### PCA-Kalman

> **Wang, Y., et al.** (2018).
> *PCA-Kalman: Device-Free Human Behavior Detection with Commodity Wi-Fi.*
> EURASIP Journal on Wireless Communications and Networking.
> [Link](https://jwcn-eurasipjournals.springeropen.com/articles/10.1186/s13638-018-1230-2)

Combines PCA-based signal cleaning with Kalman filtering for device-free detection of walking, sitting, falling. Demonstrates the combination of background subtraction and trajectory smoothing that Spaxel uses.

---

### UKF with Self-Attention

> **Zhang, X., et al.** (2023).
> *Device-Free Human Tracking and Activity Recognition Using Commodity Wi-Fi: A Survey.*
> MDPI Sensors, 23(12), 5527.
> [Link](https://www.mdpi.com/1424-8220/23/12/5527)

Combines Unscented Kalman Filter with self-attention mechanisms for WiFi CSI tracking. Demonstrates that modern ML attention mechanisms can replace hand-engineered motion models in the Kalman process noise. Relevant for Spaxel's trajectory smoother.

---

## Multi-Person Sensing

### WiMANS

> **Chen, W., et al.** (2022).
> *WiMANS: A Benchmark Dataset for WiFi-based Multi-user Activity and Number Sensing.*
> arXiv:2208.05506.
> [arXiv](https://arxiv.org/abs/2208.05506)

Benchmark dataset specifically for multi-user WiFi sensing. Contains synchronized CSI and video for 1–5 simultaneous users. Documents the accuracy degradation curve as person count increases: +15.4% localization error and −25.7% activity recognition accuracy from 1 to 5 people. Useful ground truth for calibrating Spaxel's multi-person limitations.

---

## Standardisation

### IEEE 802.11bf Overview

> **Hernandez, S. M., & Kosek-Szott, K.** (2022).
> *A Survey of IEEE 802.11bf Wi-Fi Sensing.*
> arXiv:2207.04859.
> [arXiv](https://arxiv.org/abs/2207.04859)

Comprehensive overview of the IEEE 802.11bf amendment for WLAN sensing. Covers sub-7 GHz and 60 GHz (DMG) operation, standardised sensing measurement acquisition frames, bistatic and multistatic procedures, and coexistence with communication traffic. Approved May 2025. The first formal IEEE standardisation of WiFi sensing.

> **IEEE P802.11 Task Group BF (WLAN Sensing)**
> [Status page](https://www.ieee802.org/11/Reports/tgbf_update.htm)

---

## Surveys

### WiFi Sensing with CSI: A Survey

> **Ma, Y., Zhou, G., & Wang, S.** (2019).
> *WiFi Sensing with Channel State Information: A Survey.*
> ACM Computing Surveys, 52(3), Article 46.

Comprehensive review of CSI-based sensing across activity recognition, gesture detection, localization, and vital signs. Organises the field by sensing task, signal model, and hardware platform. The standard reference survey prior to 2020.

---

### Commodity WiFi Sensing in 10 Years

> **Yousefi, S., et al.** (2021).
> *A Survey of Commodity WiFi Sensing in 10 Years: Status, Challenges, and Opportunities.*
> IEEE Internet of Things Journal.
> [arXiv](https://arxiv.org/abs/2111.07038)

Categorises a decade of work into: activity recognition, object sensing, and localization. Documents key limitations including stationary detection difficulty, multi-person degradation, and domain dependency (trained models failing in new environments).

---

### Awesome-WiFi-CSI-Sensing

> **NTUMARS Lab.** (2022–ongoing).
> *Awesome-WiFi-CSI-Sensing.*
> GitHub: [NTUMARS/Awesome-WiFi-CSI-Sensing](https://github.com/NTUMARS/Awesome-WiFi-CSI-Sensing)

Curated list of papers, datasets, and tools for WiFi CSI sensing. Useful as a living index of the field.

---

## ESP32-Specific Tools and Papers

### ESP-CSI (Espressif Official)

> **Espressif Systems.** (2021–ongoing).
> *ESP-CSI: Wi-Fi CSI (Channel State Information) Application.*
> GitHub: [espressif/esp-csi](https://github.com/espressif/esp-csi)

Official Espressif example project demonstrating CSI capture on ESP32 family chips. Includes human detection demo, data collection tools, and Python visualisation scripts. Canonical reference for `wifi_csi_config_t` usage.

---

### ESP32 CSI Toolkit

> **Hernandez, S. M., & Bulut, E.** (2020).
> *Wi-ESP: A Tool for CSI-based Device-Free Wi-Fi Sensing (DFWS).*
> Journal of Computational Design and Engineering, 7(5), 644–656.
> [Oxford Academic](https://academic.oup.com/jcde/article/7/5/644/5837600)

Open-source ESP32 CSI data collection toolkit with active/passive sensing modes, SD card logging, and serial streaming. Includes Python visualisation. Widely cited in ESP32 sensing papers.

> [Tool: stevenmhernandez.github.io/ESP32-CSI-Tool](https://stevenmhernandez.github.io/ESP32-CSI-Tool/)

---

### Tsinghua WiFi Sensing Tutorial

> **TNS Lab, Tsinghua University.** (2022–ongoing).
> *Hands-on Wireless Sensing with Wi-Fi.*
> [tns.thss.tsinghua.edu.cn/wst](https://tns.thss.tsinghua.edu.cn/wst/)

Comprehensive online tutorial covering: CSI fundamentals, sanitisation, feature extraction, algorithms, and hands-on experiments. Maintained by the group that produced the Widar series. Includes worked examples for phase correction, STFT-based DFS extraction, and MUSIC implementation.

---

## Fresnel Zone and Physical Modelling

### Fresnel Zone to CSI-Ratio Model

> **Shi, S., et al.** (2021).
> *From Fresnel Diffraction Model to Fine-grained Human Respiration Sensing with Commodity Wi-Fi Devices.*
> Springer CCF Transactions on Networking.
> [Springer](https://link.springer.com/article/10.1007/s42486-021-00077-z)

Derives a closed-form expression relating Fresnel zone geometry to the CSI amplitude ratio between affected and unaffected subcarriers. Provides the theoretical basis for Fresnel zone weighted localization. Directly applicable to Spaxel's positioning engine.

---

### Fresnel Zone Contactless Sensing

> **Wang, X., et al.** (2021).
> *Fresnel Zone Based Theories for Contactless Sensing.*
> In: *Contactless Human Activity Analysis.* Springer.
> [Springer](https://link.springer.com/chapter/10.1007/978-3-030-68590-4_5)

Reviews the application of Fresnel zone theory to contactless sensing across radar, ultrasound, and WiFi. Covers ellipsoidal constraint intersection for multi-link localization. Good reference for the geometric model underlying Spaxel's voxel accumulator.

---

## Human Body Electromagnetic Properties

### Gabriel Tissue Database

> **Gabriel, S., Lau, R. W., & Gabriel, C.** (1996).
> *The dielectric properties of biological tissues: II. Measurements in the frequency range 10 Hz to 20 GHz.*
> Physics in Medicine and Biology, 41(11), 2251.

The foundational reference for human tissue dielectric properties. Provides the 4-Cole-Cole model coefficients for all major tissue types from 10 Hz to 20 GHz. The source of the ε' ≈ 52, σ ≈ 2.4 S/m values for muscle at 2.4 GHz cited throughout the literature.

> IFAC-CNR interactive database: [niremf.ifac.cnr.it/tissprop](https://niremf.ifac.cnr.it/tissprop/)
> IT'IS Foundation database: [itis.swiss/tissue-properties](https://itis.swiss/virtual-population/tissue-properties/database/dielectric-properties/)

---

### Human Body Shadowing at 2.4 GHz

> **Ghaddar, M., et al.** (2018).
> *Analysis of Human Body Shadowing at 2.4 GHz.*
> MDPI Sensors, 18(10), 3412.
> [MDPI](https://www.mdpi.com/1424-8220/18/10/3412)

Empirical measurements of human body-induced WiFi shadowing: 3–12 dB additional attenuation depending on body orientation and link geometry. Validates the "saltwater bag" absorption model for 2.4 GHz sensing.
