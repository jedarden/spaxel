/**
 * Compile-time verification that the blob identity fields are
 * TypeScript-compliant (bead bf-56uk).
 *
 * This file is the TypeScript mirror of the Go serialization suites added
 * across bf-5151 / bf-2ibc / bf-5v3q / bf-3wkz / bf-4qto:
 *   - mothership/internal/signal/identity_fields_test.go
 *   - mothership/internal/automation/identity_fields_test.go
 *   - mothership/internal/tracker/identity_fields_test.go
 *   - mothership/internal/tracking/identity_fields_test.go
 *   - mothership/internal/volume/identity_fields_test.go
 *   - mothership/internal/explainability/identity_fields_test.go
 *   - mothership/internal/api/tracks_identity_test.go
 *
 * It asserts the three task-required properties of bf-56uk:
 *   1. a zero-value blob (no identity set) satisfies `Blob` — no missing-field
 *      errors; the identity fields are correctly optional;
 *   2. a resolved-identity blob carrying the camelCase keys the Go dashboard
 *      projection emits (`dashboard/internal/dashboard/hub.go` `blobJSON`:
 *      `personName` / `assignedColor` / `identityResolved`) satisfies `Blob` —
 *      no type-mismatch errors for identity fields;
 *   3. `identityResolved` is a faithful tri-state: `undefined` (not attempted),
 *      `true` (resolved), and `false` (attempted + failed) are all assignable,
 *      matching the Go `*bool` + `omitempty` wire contract (nil → omitted, so
 *      the JS value is `undefined`, never `null`).
 *
 * Run: `npm run typecheck` (== `tsc --noEmit -p tsconfig.json`).
 * This module has no runtime body — it is a pure compile-time assertion. If
 * `tsc` exits 0, every assignment below type-checks against the `Blob`
 * interface in `./spaxel.d.ts`.
 *
 * Scope note: only the identity fields are in scope for bf-56uk. The
 * pre-existing `Blob.id: string` vs. the Go wire `id` (emitted as a JSON
 * number from `blobJSON.ID int`) is intentionally NOT asserted here — it is a
 * separate, non-identity concern tracked elsewhere.
 */

import type { Blob } from './spaxel';

// ---------------------------------------------------------------------------
// Type-level assertions: the identity fields are declared on `Blob` with the
// exact names and types the Go `blobJSON` tags emit. If any name drifts, the
// corresponding `AssertTrue<…>` line becomes a compile error.
// ---------------------------------------------------------------------------

/** Compile-time "true" sentinel — only `true` satisfies it. */
type AssertTrue<T extends true> = T;

/**
 * True iff `T` declares key `K` whose type accepts a value of type `V`
 * (i.e. `V extends T[K]`). Returns `false` when `K` is absent.
 *
 * Implementation note: this is deliberately written with the concrete `V`
 * (e.g. `string`, `boolean`) on the LEFT of `extends` and the indexed `T[K]`
 * on the RIGHT. The reverse shape — `NonNullable<T[K]> extends NonNullable<V>`
 * — is intentionally NOT used: TypeScript does not reliably evaluate a utility
 * type (`NonNullable`) applied to an *indexed type parameter* `T[K]` inside a
 * nested conditional, which previously produced a spurious `false` (TS2344)
 * for the optional `boolean` `identityResolved` field. `V extends T[K]`
 * resolves correctly in every case and still catches both a missing key
 * (`false`) and a type-mismatched field (`false`).
 */
type HasField<T, K extends PropertyKey, V> = K extends keyof T
    ? V extends T[K]
        ? true
        : false
    : false;

// Canonical identity fields (bf-5151): camelCase JSON keys from hub.go blobJSON.
type _PersonNameIsString = AssertTrue<HasField<Blob, 'personName', string>>;
type _AssignedColorIsString = AssertTrue<HasField<Blob, 'assignedColor', string>>;
type _IdentityResolvedIsBool = AssertTrue<HasField<Blob, 'identityResolved', boolean>>;

// Deprecated aliases retained on the interface for backward compatibility.
type _PersonLabelIsString = AssertTrue<HasField<Blob, 'personLabel', string>>;
type _PersonColorIsString = AssertTrue<HasField<Blob, 'personColor', string>>;
type _PersonIdIsString = AssertTrue<HasField<Blob, 'personId', string>>;

// ---------------------------------------------------------------------------
// (1) Zero-value blob: identity fields omitted entirely. Must satisfy `Blob`
//     with no missing-field errors. Mirrors the Go "zero-value blob omits all
//     three fields (undefined in JS)" assertion.
// ---------------------------------------------------------------------------

const zeroValueBlob: Blob = {
    id: '1',
    x: 0,
    y: 0,
    z: 0,
    confidence: 0,
};

// ---------------------------------------------------------------------------
// (2) Resolved-identity blob: carries the exact camelCase keys emitted by the
//     Go dashboard projection (hub.go: personName / assignedColor /
//     identityResolved). Must satisfy `Blob` with no type-mismatch errors.
// ---------------------------------------------------------------------------

const resolvedIdentityBlob: Blob = {
    id: '2',
    x: 3.2,
    y: 1.1,
    z: 0.8,
    confidence: 0.85,
    vx: 0.3,
    vy: -0.1,
    vz: 0,
    posture: 'walking',
    personName: 'Alice',
    assignedColor: '#4488ff',
    identityResolved: true,
};

// Deprecated aliases still accepted by the interface (backward compat).
const legacyAliasBlob: Blob = {
    id: '3',
    x: 0,
    y: 0,
    z: 0,
    confidence: 0,
    personId: 'alice-uuid',
    personLabel: 'Alice', // deprecated alias of personName
    personColor: '#4488ff', // deprecated alias of assignedColor
};

// ---------------------------------------------------------------------------
// (3) identityResolved tri-state, mirroring the Go `*bool` + `omitempty`
//     contract: nil → omitted (undefined), &true → true, &false → false.
// ---------------------------------------------------------------------------

const identityNotAttempted: Blob = {
    ...zeroValueBlob,
    id: '4',
};

const identityResolvedTrue: Blob = {
    ...zeroValueBlob,
    id: '4a',
    identityResolved: true,
};

const identityResolvedFalse: Blob = {
    ...zeroValueBlob,
    id: '4b',
    identityResolved: false,
};

// Explicit `undefined` is assignable (field present but unset).
const identityExplicitlyUndefined: Blob = {
    ...zeroValueBlob,
    id: '4c',
    identityResolved: undefined,
};

// ---------------------------------------------------------------------------
// Dashboard state map shape: appState.blobs is Record<string, Blob>
// (dashboard/js/state.js: appState.blobs[id]). A mixed map of zero-value and
// resolved blobs must satisfy it wholesale.
// ---------------------------------------------------------------------------

const blobMap: Record<string, Blob> = {
    [zeroValueBlob.id]: zeroValueBlob,
    [resolvedIdentityBlob.id]: resolvedIdentityBlob,
    [legacyAliasBlob.id]: legacyAliasBlob,
    [identityResolvedFalse.id]: identityResolvedFalse,
};

// `Object.assign(target, updates)` in state.js merges a server blob into an
// existing Blob entry; the updates shape must be assignable to a partial Blob.
function mergeBlobUpdate(target: Blob, updates: Partial<Blob>): Blob {
    return Object.assign(target, updates);
}

const merged = mergeBlobUpdate(zeroValueBlob, {
    personName: 'Bob',
    assignedColor: '#ff8800',
    identityResolved: true,
});

// Reference the bindings so they are not flagged as unused by tooling that
// walks runtime ASTs (tsc itself needs no this under noEmit, but it keeps the
// file honest for any future runtime pass).
export const __blobIdentityChecks = {
    zeroValueBlob,
    resolvedIdentityBlob,
    legacyAliasBlob,
    identityNotAttempted,
    identityResolvedTrue,
    identityResolvedFalse,
    identityExplicitlyUndefined,
    blobMap,
    merged,
};
