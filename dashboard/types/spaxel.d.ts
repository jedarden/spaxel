/**
 * Spaxel Dashboard Type Definitions
 *
 * TypeScript type definitions for the Spaxel dashboard state and data structures.
 */

/**
 * Blob - represents a detected person/tracked object
 */
export interface Blob {
  /** Unique identifier for this blob */
  id: string;

  /** Position coordinates (meters) */
  x: number;
  y: number;
  z: number;

  /** Detection confidence (0-1) */
  confidence: number;

  /** Velocity vector (m/s) - optional */
  vx?: number;
  vy?: number;
  vz?: number;

  /** Posture state (e.g., 'standing', 'sitting', 'walking') - optional */
  posture?: string;

  /** Associated person identifier - optional */
  person?: string | null;

  /** Associated BLE device address - optional */
  ble_device?: string | null;

  /** Historical trail data - optional */
  trails?: Array<{
    x: number;
    y: number;
    z: number;
    timestamp_ms: number;
  }>;

  // ============================
  // Identity Resolution Fields
  // ============================

  /**
   * Human-readable name for the person associated with this blob.
   * This is the display name shown in the UI when identity is resolved.
   * @optional
   */
  personName?: string;

  /**
   * @deprecated Use personName instead - personLabel is maintained for backward compatibility
   * Legacy field for person label (deprecated in favor of personName)
   * @optional
   */
  personLabel?: string;

  /**
   * Person identifier for this blob (distinct from display name)
   * @optional
   */
  personId?: string;

  /**
   * Assigned display color for this blob/person.
   * Format: hex string (e.g., "#FF5733") or rgb string (e.g., "rgb(255, 87, 51)").
   * When provided, this color is used consistently across the UI for this blob.
   * @optional
   */
  assignedColor?: string;

  /**
   * @deprecated Use assignedColor instead - personColor is maintained for backward compatibility
   * Legacy field for person color (deprecated in favor of assignedColor)
   * @optional
   */
  personColor?: string;

  /**
   * Flag indicating whether this blob's identity has been resolved.
   * - true: Identity has been confidently resolved to a known person
   * - false: Identity resolution failed or blob is explicitly unknown
   * - undefined: Identity resolution has not been attempted yet
   * @optional
   */
  identityResolved?: boolean;
}

/**
 * Node - represents a hardware sensor node
 */
export interface Node {
  /** MAC address (unique identifier) */
  mac: string;

  /** Display name */
  name?: string;

  /** Position coordinates (meters) */
  pos_x?: number;
  pos_y?: number;
  pos_z?: number;

  /** Node role (e.g., 'anchor', 'tag') */
  role?: string;

  /** Firmware version */
  firmware_version?: string;

  /** Node status */
  status?: string;

  /** Signal strength */
  rssi?: number;

  /** Uptime in seconds */
  uptime_s?: number;

  /** Last seen timestamp */
  last_seen?: number;

  /** Virtual node flag */
  virtual?: boolean;
}

/**
 * Zone - represents a spatial region
 */
export interface Zone {
  /** Unique identifier */
  id: string;

  /** Zone name */
  name?: string;

  /** Position (center point) */
  x?: number;
  y?: number;
  z?: number;

  /** Dimensions */
  w?: number;  // width
  d?: number;  // depth
  h?: number;  // height

  /** Zone type */
  zone_type?: string;

  /** Current occupancy count */
  occupancy?: number;

  /** List of people currently in zone */
  people?: string[];
}

/**
 * Link - represents a node-to-node connection
 */
export interface Link {
  /** Unique identifier */
  id: string;

  /** Source node MAC address */
  node_mac: string;

  /** Peer node MAC address */
  peer_mac: string;

  /** Link quality metrics */
  delta_rms?: number;
  snr?: number;
  phase_stability?: number;
  quality?: number;
  weight?: number;
}

/**
 * Alert - represents a system alert
 */
export interface Alert {
  /** Unique identifier */
  id: string;

  /** Alert type */
  type: string;

  /** Severity level */
  severity: 'low' | 'medium' | 'high' | 'critical';

  /** Alert title */
  title: string;

  /** Detailed message */
  message: string;

  /** Timestamp (milliseconds since epoch) */
  timestamp_ms: number;

  /** Acknowledgment flag */
  acknowledged?: boolean;
}

/**
 * Event - represents a timeline event
 */
export interface Event {
  /** Unique identifier */
  id?: string;

  /** Timestamp (milliseconds since epoch) */
  timestamp_ms: number;

  /** Event type */
  type: string;

  /** Associated zone (optional) */
  zone?: string;

  /** Associated person (optional) */
  person?: string;

  /** Associated blob ID (optional) */
  blob_id?: string;

  /** Additional event details */
  detail_json?: any;

  /** Event severity */
  severity?: string;
}

/**
 * BLE Device - represents a Bluetooth Low Energy device
 */
export interface BLEDevice {
  /** Device address */
  addr: string;

  /** Display label */
  label?: string;

  /** Device type */
  type?: string;

  /** Display color */
  color?: string;

  /** Icon identifier */
  icon?: string;

  /** Auto-rotate flag */
  auto_rotate?: boolean;

  /** First seen timestamp */
  first_seen?: number;

  /** Last seen timestamp */
  last_seen?: number;

  /** Last RSSI value */
  last_rssi?: number;
}

/**
 * Trigger - represents an automation trigger
 */
export interface Trigger {
  /** Unique identifier */
  id: string;

  /** Trigger name */
  name?: string;

  /** Shape definition (JSON) */
  shape_json?: any;

  /** Condition expression */
  condition?: string;

  /** Condition parameters (JSON) */
  condition_params_json?: any;

  /** Time constraints (JSON) */
  time_constraint_json?: any;

  /** Actions to execute (JSON) */
  actions_json?: any;

  /** Enabled flag */
  enabled?: boolean;

  /** Last fired timestamp */
  last_fired?: number;
}

/**
 * Portal - represents a connection between zones
 */
export interface Portal {
  /** Unique identifier */
  id: string;

  /** Portal name */
  name?: string;

  /** First zone ID */
  zone_a_id?: string;

  /** Second zone ID */
  zone_b_id?: string;

  /** Portal points (JSON) */
  points_json?: any;
}

/**
 * System information
 */
export interface SystemInfo {
  version: string | null;
  uptime_s: number;
  detection_quality: number;
  confidence: number;
  security_mode: boolean;
  nodes_online: number;
  nodes_total: number;
}

/**
 * Prediction - represents a predicted state
 */
export interface Prediction {
  /** Associated person */
  person?: string;

  /** Predicted zone */
  zone?: string;

  /** Prediction probability (0-1) */
  probability?: number;

  /** Prediction horizon (minutes) */
  horizon_min?: number;
}

/**
 * Connection state
 */
export interface ConnectionState {
  connected: boolean;
  connecting: boolean;
  last_disconnect_time: number | null;
}

/**
 * Application settings
 */
export interface Settings {
  delta_rms_threshold: number;
  fusion_rate_hz: number;
  grid_cell_m: number;
  fresnel_decay: number;
  n_subcarriers: number;
  tau_s: number;
  breathing_sensitivity: number;
}

/**
 * Spaxel application state
 */
export interface SpaxelState {
  /** Map of MAC -> Node */
  nodes: Record<string, Node>;

  /** Map of blob_id -> Blob */
  blobs: Record<string, Blob>;

  /** Map of zone_id -> Zone */
  zones: Record<string, Zone>;

  /** Map of link_id -> Link */
  links: Record<string, Link>;

  /** Array of active alerts */
  alerts: Alert[];

  /** Array of recent events */
  events: Event[];

  /** Map of addr -> BLEDevice */
  ble_devices: Record<string, BLEDevice>;

  /** Map of trigger_id -> Trigger */
  triggers: Record<string, Trigger>;

  /** Map of portal_id -> Portal */
  portals: Record<string, Portal>;

  /** System information */
  system: SystemInfo;

  /** Array of predictions */
  predictions: Prediction[];

  /** Connection state */
  connection: ConnectionState;

  /** Application settings */
  settings: Settings;
}
