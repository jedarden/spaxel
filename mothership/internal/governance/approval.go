// Package governance implements the offline administration surface for
// derived-use approvals.  It deliberately does not contain a private key:
// signing is delegated to a protected-key provider using only a reference.
package governance

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	ApprovalSchemaVersion   = "use-approval-v1"
	RevocationSchemaVersion = "use-approval-revocation-v1"

	approvalSignatureDomain   = "spaxel/governance/use-approval-v1\x00"
	revocationSignatureDomain = "spaxel/governance/use-approval-revocation-v1\x00"
	maxReferenceLength        = 512
	maxIdentifierLength       = 256
	maxReasonLength           = 2048
	maxPurposeLength          = 256
	maxConsumerClassLength    = 128
	maxApprovalAge            = 30 * 24 * time.Hour
)

var (
	ErrInvalidRecord       = errors.New("invalid governance record")
	ErrInvalidScope        = errors.New("approval scope mismatch")
	ErrExpired             = errors.New("approval expired")
	ErrNotYetValid         = errors.New("approval is not yet valid")
	ErrRevoked             = errors.New("approval revoked")
	ErrPolicyStale         = errors.New("approval policy is stale")
	ErrNotFound            = errors.New("governance record not found")
	ErrConflict            = errors.New("governance record conflict")
	ErrAlreadyRevoked      = errors.New("approval already revoked")
	ErrInvalidKeyReference = errors.New("invalid protected key reference")
	ErrUnauthorized        = errors.New("governance authorization failed")
)

// ProtectedKeyReference names key material held by an offline secret manager.
// It is intentionally not a key, PEM block, or serialized private-key value.
type ProtectedKeyReference string

func (r ProtectedKeyReference) Validate() error {
	v := strings.TrimSpace(string(r))
	if v == "" || len(v) > maxReferenceLength || strings.ContainsAny(v, "\r\n") {
		return ErrInvalidKeyReference
	}
	upper := strings.ToUpper(v)
	if strings.Contains(upper, "PRIVATE KEY") || strings.Contains(upper, "BEGIN ") || strings.Contains(upper, "END ") {
		return ErrInvalidKeyReference
	}
	return nil
}

// GovernanceIdentity is the public identity used by an offline administrator.
// PrivateKeyRef is never included in an approval or revocation record.
type GovernanceIdentity struct {
	TenantID      string                `json:"tenant_id"`
	KeyID         string                `json:"key_id"`
	KeyEpoch      uint64                `json:"key_epoch"`
	PrivateKeyRef ProtectedKeyReference `json:"-"`
}

func (i GovernanceIdentity) Validate() error {
	if err := validateIdentifier(i.TenantID, "tenant id"); err != nil {
		return err
	}
	if err := validateIdentifier(i.KeyID, "key id"); err != nil {
		return err
	}
	if i.KeyEpoch == 0 {
		return fmt.Errorf("%w: key epoch must be positive", ErrInvalidRecord)
	}
	if err := i.PrivateKeyRef.Validate(); err != nil {
		return err
	}
	return nil
}

// ProtectedSigner signs bytes by resolving ref inside protected storage.  The
// implementation may use an HSM, OpenBao, a TPM-backed helper, or another
// offline mechanism.  No private key is passed through this package's records.
type ProtectedSigner interface {
	SignProtected(ref ProtectedKeyReference, message []byte) ([]byte, error)
}

// SignFunc adapts a function to ProtectedSigner.  Callers should resolve the
// reference only inside the function and must not persist the returned key.
type SignFunc func(ref ProtectedKeyReference, message []byte) ([]byte, error)

func (f SignFunc) SignProtected(ref ProtectedKeyReference, message []byte) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("%w: nil signer", ErrUnauthorized)
	}
	return f(ref, message)
}

// VerificationKey is a public governance key.  Historical keys remain in the
// key set so retained approvals and revocations remain independently verifiable
// after rotation.
type VerificationKey struct {
	TenantID string
	KeyID    string
	KeyEpoch uint64
	Public   ed25519.PublicKey
}

// PublicKeyResolver resolves both current and historical governance keys.
type PublicKeyResolver interface {
	ResolveGovernanceKey(tenantID, keyID string, epoch uint64) (ed25519.PublicKey, error)
}

// PublicKeySet is an in-memory verifier registry.  Durable deployments can
// implement PublicKeyResolver over their signed control records instead.
type PublicKeySet struct {
	mu      sync.RWMutex
	keys    map[string]ed25519.PublicKey
	current map[string]keyVersion
}

type keyVersion struct {
	id    string
	epoch uint64
}

func NewPublicKeySet(keys ...VerificationKey) (*PublicKeySet, error) {
	r := &PublicKeySet{
		keys:    make(map[string]ed25519.PublicKey),
		current: make(map[string]keyVersion),
	}
	for _, key := range keys {
		if err := r.Add(key); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func keyMapKey(tenantID, keyID string, epoch uint64) string {
	return tenantID + "\x00" + keyID + "\x00" + fmt.Sprint(epoch)
}

func (r *PublicKeySet) Add(key VerificationKey) error {
	if r == nil {
		return fmt.Errorf("%w: nil key set", ErrInvalidRecord)
	}
	if err := validateIdentifier(key.TenantID, "tenant id"); err != nil {
		return err
	}
	if err := validateIdentifier(key.KeyID, "key id"); err != nil {
		return err
	}
	if key.KeyEpoch == 0 || len(key.Public) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid verification key", ErrInvalidRecord)
	}
	keyCopy := append(ed25519.PublicKey(nil), key.Public...)
	r.mu.Lock()
	defer r.mu.Unlock()
	k := keyMapKey(key.TenantID, key.KeyID, key.KeyEpoch)
	if existing, ok := r.keys[k]; ok {
		if string(existing) != string(keyCopy) {
			return fmt.Errorf("%w: verification key changed", ErrConflict)
		}
		return nil
	}
	r.keys[k] = keyCopy
	current, ok := r.current[key.TenantID]
	if !ok || key.KeyEpoch > current.epoch {
		r.current[key.TenantID] = keyVersion{id: key.KeyID, epoch: key.KeyEpoch}
	}
	return nil
}

// Rotate adds a strictly newer key and makes it the current issuance key.
func (r *PublicKeySet) Rotate(key VerificationKey) error {
	if r == nil {
		return fmt.Errorf("%w: nil key set", ErrInvalidRecord)
	}
	r.mu.RLock()
	current, ok := r.current[key.TenantID]
	r.mu.RUnlock()
	if ok && key.KeyEpoch <= current.epoch {
		return fmt.Errorf("%w: key epoch must increase", ErrConflict)
	}
	return r.Add(key)
}

func (r *PublicKeySet) ResolveGovernanceKey(tenantID, keyID string, epoch uint64) (ed25519.PublicKey, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil key set", ErrUnauthorized)
	}
	r.mu.RLock()
	key, ok := r.keys[keyMapKey(tenantID, keyID, epoch)]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: governance key %s/%d unavailable", ErrUnauthorized, keyID, epoch)
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

func (r *PublicKeySet) Current(tenantID string) (VerificationKey, error) {
	if r == nil {
		return VerificationKey{}, fmt.Errorf("%w: nil key set", ErrUnauthorized)
	}
	r.mu.RLock()
	current, ok := r.current[tenantID]
	key, keyOK := r.keys[keyMapKey(tenantID, current.id, current.epoch)]
	r.mu.RUnlock()
	if !ok || !keyOK {
		return VerificationKey{}, fmt.Errorf("%w: no current governance key", ErrUnauthorized)
	}
	return VerificationKey{TenantID: tenantID, KeyID: current.id, KeyEpoch: current.epoch, Public: append(ed25519.PublicKey(nil), key...)}, nil
}

// AssessmentFreshnessPolicy is copied into every approval.  This snapshot
// prevents a verifier from silently evaluating the approval under a different
// freshness rule than the approver saw.
type AssessmentFreshnessPolicy struct {
	PolicyVersion          string
	PolicyDigest           string
	AssessmentNotBefore    time.Time
	MaxAssessmentAge       time.Duration
	AllowedClassifierKinds []string
	AllowedRuleSetDigests  []string
}

type assessmentFreshnessPolicyJSON struct {
	PolicyVersion           string   `json:"policy_version"`
	PolicyDigest            string   `json:"policy_digest"`
	AssessmentNotBefore     string   `json:"assessment_not_before"`
	MaxAssessmentAgeSeconds int64    `json:"max_assessment_age_seconds"`
	AllowedClassifierKinds  []string `json:"allowed_classifier_kinds"`
	AllowedRuleSetDigests   []string `json:"allowed_rule_set_digests"`
}

func (p AssessmentFreshnessPolicy) MarshalJSON() ([]byte, error) {
	return json.Marshal(assessmentFreshnessPolicyJSON{
		PolicyVersion:           p.PolicyVersion,
		PolicyDigest:            p.PolicyDigest,
		AssessmentNotBefore:     canonicalTime(p.AssessmentNotBefore),
		MaxAssessmentAgeSeconds: int64(p.MaxAssessmentAge / time.Second),
		AllowedClassifierKinds:  append([]string(nil), p.AllowedClassifierKinds...),
		AllowedRuleSetDigests:   append([]string(nil), p.AllowedRuleSetDigests...),
	})
}

func (p *AssessmentFreshnessPolicy) UnmarshalJSON(data []byte) error {
	var wire assessmentFreshnessPolicyJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	when, err := time.Parse(time.RFC3339Nano, wire.AssessmentNotBefore)
	if err != nil {
		return err
	}
	p.PolicyVersion = wire.PolicyVersion
	p.PolicyDigest = wire.PolicyDigest
	p.AssessmentNotBefore = when.UTC()
	p.MaxAssessmentAge = time.Duration(wire.MaxAssessmentAgeSeconds) * time.Second
	p.AllowedClassifierKinds = append([]string(nil), wire.AllowedClassifierKinds...)
	p.AllowedRuleSetDigests = append([]string(nil), wire.AllowedRuleSetDigests...)
	return nil
}

// AllowedUse is a policy scope pair.  Both fields are required at issuance and
// authorization; a purpose cannot be reused with another consumer class.
type AllowedUse struct {
	Purpose       string
	ConsumerClass string
}

// ConsumptionPolicy is the current tenant policy supplied to the offline
// administrator.  The policy is expected to have been signed and selected by a
// higher-level control-plane workflow; this package binds its supplied digest.
type ConsumptionPolicy struct {
	TenantID                string
	PolicyVersion           string
	PolicyDigest            string
	Assessment              AssessmentFreshnessPolicy
	MaximumApprovalLifetime time.Duration
	AllowedUses             []AllowedUse
}

func (p ConsumptionPolicy) Validate() error {
	if err := validateIdentifier(p.TenantID, "tenant id"); err != nil {
		return err
	}
	if err := validateIdentifier(p.PolicyVersion, "policy version"); err != nil {
		return err
	}
	if strings.TrimSpace(p.PolicyDigest) == "" {
		return fmt.Errorf("%w: policy digest is required", ErrInvalidRecord)
	}
	if err := p.Assessment.validate(); err != nil {
		return err
	}
	if p.Assessment.PolicyVersion != p.PolicyVersion || p.Assessment.PolicyDigest != p.PolicyDigest {
		return fmt.Errorf("%w: assessment policy does not identify current policy", ErrInvalidRecord)
	}
	if p.MaximumApprovalLifetime <= 0 || p.MaximumApprovalLifetime > maxApprovalAge || p.MaximumApprovalLifetime%time.Second != 0 {
		return fmt.Errorf("%w: maximum approval lifetime must be between one second and thirty days", ErrInvalidRecord)
	}
	seen := make(map[string]struct{}, len(p.AllowedUses))
	for _, allowed := range p.AllowedUses {
		if err := validatePurpose(allowed.Purpose); err != nil {
			return err
		}
		if err := validateConsumerClass(allowed.ConsumerClass); err != nil {
			return err
		}
		key := allowed.Purpose + "\x00" + allowed.ConsumerClass
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate allowed use", ErrInvalidRecord)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (p ConsumptionPolicy) Allows(purpose, consumerClass string) bool {
	for _, allowed := range p.AllowedUses {
		if allowed.Purpose == purpose && allowed.ConsumerClass == consumerClass {
			return true
		}
	}
	return false
}

func (p AssessmentFreshnessPolicy) validate() error {
	if err := validateIdentifier(p.PolicyVersion, "policy version"); err != nil {
		return err
	}
	if strings.TrimSpace(p.PolicyDigest) == "" || p.AssessmentNotBefore.IsZero() {
		return fmt.Errorf("%w: incomplete assessment freshness policy", ErrInvalidRecord)
	}
	if p.MaxAssessmentAge <= 0 || p.MaxAssessmentAge > maxApprovalAge || p.MaxAssessmentAge%time.Second != 0 {
		return fmt.Errorf("%w: assessment age must be between one second and thirty days", ErrInvalidRecord)
	}
	if len(p.AllowedClassifierKinds) == 0 || len(p.AllowedRuleSetDigests) == 0 {
		return fmt.Errorf("%w: assessment allowlists cannot be empty", ErrInvalidRecord)
	}
	if err := validateStringList(p.AllowedClassifierKinds, "classifier kind"); err != nil {
		return err
	}
	return validateStringList(p.AllowedRuleSetDigests, "rule-set digest")
}

// ApprovalScope is the exact derived-use scope that an agent-facing consumer
// must present.  It is intentionally separate from raw archive export scopes.
type ApprovalScope struct {
	EpisodeDigest    string
	AssessmentDigest string
	Purpose          string
	ConsumerClass    string
}

func (s ApprovalScope) validate() error {
	if err := validateDigest(s.EpisodeDigest, "episode digest"); err != nil {
		return err
	}
	if err := validateDigest(s.AssessmentDigest, "assessment digest"); err != nil {
		return err
	}
	if err := validatePurpose(s.Purpose); err != nil {
		return err
	}
	return validateConsumerClass(s.ConsumerClass)
}

func (s ApprovalScope) equal(other ApprovalScope) bool {
	return s.EpisodeDigest == other.EpisodeDigest && s.AssessmentDigest == other.AssessmentDigest && s.Purpose == other.Purpose && s.ConsumerClass == other.ConsumerClass
}

// UseApproval is the signed use-approval-v1 record.  All fields except
// Signature are covered by SigningBytes.
type UseApproval struct {
	SchemaVersion      string                    `json:"schema_version"`
	ApprovalID         string                    `json:"approval_id"`
	TenantID           string                    `json:"tenant_id"`
	EpisodeDigest      string                    `json:"episode_digest"`
	AssessmentDigest   string                    `json:"assessment_digest"`
	Purpose            string                    `json:"purpose"`
	ConsumerClass      string                    `json:"consumer_class"`
	PolicyVersion      string                    `json:"policy_version"`
	AssessmentPolicy   AssessmentFreshnessPolicy `json:"assessment_freshness_policy"`
	Approver           string                    `json:"approver"`
	IssuedAt           time.Time                 `json:"issued_at"`
	ExpiresAt          time.Time                 `json:"expires_at"`
	GovernanceKeyID    string                    `json:"governance_key_id"`
	GovernanceKeyEpoch uint64                    `json:"governance_key_epoch"`
	Signature          string                    `json:"signature"`
}

func (a UseApproval) Scope() ApprovalScope {
	return ApprovalScope{EpisodeDigest: a.EpisodeDigest, AssessmentDigest: a.AssessmentDigest, Purpose: a.Purpose, ConsumerClass: a.ConsumerClass}
}

func (a UseApproval) Validate() error {
	if a.SchemaVersion != ApprovalSchemaVersion {
		return fmt.Errorf("%w: unsupported approval schema %q", ErrInvalidRecord, a.SchemaVersion)
	}
	if err := validateIdentifier(a.ApprovalID, "approval id"); err != nil {
		return err
	}
	if err := validateIdentifier(a.TenantID, "tenant id"); err != nil {
		return err
	}
	if err := a.Scope().validate(); err != nil {
		return err
	}
	if err := validateIdentifier(a.PolicyVersion, "policy version"); err != nil {
		return err
	}
	if err := a.AssessmentPolicy.validate(); err != nil {
		return err
	}
	if a.AssessmentPolicy.PolicyVersion != a.PolicyVersion {
		return fmt.Errorf("%w: policy version mismatch", ErrInvalidRecord)
	}
	if err := validateIdentifier(a.Approver, "approver"); err != nil {
		return err
	}
	if a.IssuedAt.IsZero() || a.ExpiresAt.IsZero() || !a.ExpiresAt.After(a.IssuedAt) {
		return fmt.Errorf("%w: invalid issuance or expiry", ErrInvalidRecord)
	}
	if a.ExpiresAt.Sub(a.IssuedAt) > maxApprovalAge {
		return fmt.Errorf("%w: approval lifetime exceeds thirty days", ErrInvalidRecord)
	}
	if err := validateIdentifier(a.GovernanceKeyID, "governance key id"); err != nil {
		return err
	}
	if a.GovernanceKeyEpoch == 0 {
		return fmt.Errorf("%w: governance key epoch must be positive", ErrInvalidRecord)
	}
	if strings.TrimSpace(a.Signature) == "" {
		return fmt.Errorf("%w: signature is required", ErrInvalidRecord)
	}
	return nil
}

// SigningBytes returns the domain-separated canonical JSON signed by the
// governance identity.  Standard-library JSON is deterministic for this
// struct, and no map appears in the signed record.
func (a UseApproval) SigningBytes() ([]byte, error) {
	copy := a
	copy.Signature = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	return append([]byte(approvalSignatureDomain), data...), nil
}

func (a *UseApproval) Sign(signer ProtectedSigner, ref ProtectedKeyReference) error {
	if signer == nil {
		return fmt.Errorf("%w: nil protected signer", ErrUnauthorized)
	}
	if err := a.ValidateUnsigned(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	message, err := a.SigningBytes()
	if err != nil {
		return err
	}
	signature, err := signer.SignProtected(ref, message)
	if err != nil {
		return fmt.Errorf("%w: sign approval: %v", ErrUnauthorized, err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signer returned invalid signature size", ErrUnauthorized)
	}
	a.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (a UseApproval) ValidateUnsigned() error {
	copy := a
	copy.Signature = "unsigned"
	return copy.Validate()
}

func (a UseApproval) Verify(public ed25519.PublicKey) error {
	if err := a.Validate(); err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(a.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: malformed approval signature", ErrInvalidRecord)
	}
	message, err := a.SigningBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, message, signature) {
		return fmt.Errorf("%w: approval signature verification failed", ErrUnauthorized)
	}
	return nil
}

// ApprovalRevocation is a signed, append-only revocation event.  The original
// approval is never modified or deleted.
type ApprovalRevocation struct {
	SchemaVersion      string    `json:"schema_version"`
	RevocationID       string    `json:"revocation_id"`
	TenantID           string    `json:"tenant_id"`
	ApprovalID         string    `json:"approval_id"`
	Reason             string    `json:"reason"`
	RevokedBy          string    `json:"revoked_by"`
	RevokedAt          time.Time `json:"revoked_at"`
	GovernanceKeyID    string    `json:"governance_key_id"`
	GovernanceKeyEpoch uint64    `json:"governance_key_epoch"`
	Signature          string    `json:"signature"`
}

func (r ApprovalRevocation) Validate() error {
	if r.SchemaVersion != RevocationSchemaVersion {
		return fmt.Errorf("%w: unsupported revocation schema %q", ErrInvalidRecord, r.SchemaVersion)
	}
	for value, name := range map[string]string{
		r.RevocationID:    "revocation id",
		r.TenantID:        "tenant id",
		r.ApprovalID:      "approval id",
		r.RevokedBy:       "revoked by",
		r.GovernanceKeyID: "governance key id",
	} {
		if err := validateIdentifier(value, name); err != nil {
			return err
		}
	}
	if strings.TrimSpace(r.Reason) == "" || len(r.Reason) > maxReasonLength || strings.ContainsAny(r.Reason, "\r\n") {
		return fmt.Errorf("%w: invalid revocation reason", ErrInvalidRecord)
	}
	if r.RevokedAt.IsZero() || r.GovernanceKeyEpoch == 0 || strings.TrimSpace(r.Signature) == "" {
		return fmt.Errorf("%w: incomplete revocation", ErrInvalidRecord)
	}
	return nil
}

func (r ApprovalRevocation) SigningBytes() ([]byte, error) {
	copy := r
	copy.Signature = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	return append([]byte(revocationSignatureDomain), data...), nil
}

func (r *ApprovalRevocation) Sign(signer ProtectedSigner, ref ProtectedKeyReference) error {
	if signer == nil {
		return fmt.Errorf("%w: nil protected signer", ErrUnauthorized)
	}
	copy := *r
	copy.Signature = "unsigned"
	if err := copy.Validate(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	message, err := r.SigningBytes()
	if err != nil {
		return err
	}
	signature, err := signer.SignProtected(ref, message)
	if err != nil {
		return fmt.Errorf("%w: sign revocation: %v", ErrUnauthorized, err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signer returned invalid signature size", ErrUnauthorized)
	}
	r.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (r ApprovalRevocation) Verify(public ed25519.PublicKey) error {
	if err := r.Validate(); err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: malformed revocation signature", ErrInvalidRecord)
	}
	message, err := r.SigningBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, message, signature) {
		return fmt.Errorf("%w: revocation signature verification failed", ErrUnauthorized)
	}
	return nil
}

// ApprovalStore is intentionally narrower than a general database.  It cannot
// update or delete an approval, and revocation is only an append operation.
type ApprovalStore interface {
	PutApproval(UseApproval) error
	GetApproval(approvalID string) (UseApproval, error)
	ListApprovals() ([]UseApproval, error)
	AppendRevocation(ApprovalRevocation) error
	ListRevocations(approvalID string) ([]ApprovalRevocation, error)
}

// FileStore is an offline, mode-restricted JSONL store.  approvals-v1.jsonl is
// immutable after append; revocations-v1.jsonl is the append-only revocation
// ledger.  The format is content-address-independent and easy to inspect
// without loading private authority material.
type FileStore struct {
	mu              sync.Mutex
	root            string
	approvalsPath   string
	revocationsPath string
}

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty store path", ErrInvalidRecord)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{
		root:            root,
		approvalsPath:   filepath.Join(root, "approvals-v1.jsonl"),
		revocationsPath: filepath.Join(root, "revocations-v1.jsonl"),
	}, nil
}

func (s *FileStore) PutApproval(approval UseApproval) error {
	if s == nil {
		return fmt.Errorf("%w: nil store", ErrInvalidRecord)
	}
	if err := approval.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals, err := s.readApprovalsLocked()
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	for _, existing := range approvals {
		if existing.ApprovalID != approval.ApprovalID {
			continue
		}
		existingBytes, marshalErr := json.Marshal(existing)
		if marshalErr == nil && string(existingBytes) == string(canonical) {
			return nil
		}
		return fmt.Errorf("%w: approval id already contains different bytes", ErrConflict)
	}
	return appendJSONLine(s.approvalsPath, canonical)
}

func (s *FileStore) GetApproval(approvalID string) (UseApproval, error) {
	if err := validateIdentifier(approvalID, "approval id"); err != nil {
		return UseApproval{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals, err := s.readApprovalsLocked()
	if err != nil {
		return UseApproval{}, err
	}
	for _, approval := range approvals {
		if approval.ApprovalID == approvalID {
			return approval, nil
		}
	}
	return UseApproval{}, ErrNotFound
}

func (s *FileStore) ListApprovals() ([]UseApproval, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: nil store", ErrInvalidRecord)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals, err := s.readApprovalsLocked()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(approvals, func(i, j int) bool {
		if approvals[i].IssuedAt.Equal(approvals[j].IssuedAt) {
			return approvals[i].ApprovalID < approvals[j].ApprovalID
		}
		return approvals[i].IssuedAt.Before(approvals[j].IssuedAt)
	})
	return approvals, nil
}

func (s *FileStore) AppendRevocation(revocation ApprovalRevocation) error {
	if s == nil {
		return fmt.Errorf("%w: nil store", ErrInvalidRecord)
	}
	if err := revocation.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revocations, err := s.readRevocationsLocked()
	if err != nil {
		return err
	}
	for _, existing := range revocations {
		if existing.ApprovalID == revocation.ApprovalID {
			return ErrAlreadyRevoked
		}
		if existing.RevocationID == revocation.RevocationID {
			return fmt.Errorf("%w: revocation id already exists", ErrConflict)
		}
	}
	canonical, err := json.Marshal(revocation)
	if err != nil {
		return err
	}
	return appendJSONLine(s.revocationsPath, canonical)
}

func (s *FileStore) ListRevocations(approvalID string) ([]ApprovalRevocation, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: nil store", ErrInvalidRecord)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revocations, err := s.readRevocationsLocked()
	if err != nil {
		return nil, err
	}
	if approvalID == "" {
		return revocations, nil
	}
	filtered := make([]ApprovalRevocation, 0, len(revocations))
	for _, revocation := range revocations {
		if revocation.ApprovalID == approvalID {
			filtered = append(filtered, revocation)
		}
	}
	return filtered, nil
}

func (s *FileStore) readApprovalsLocked() ([]UseApproval, error) {
	var approvals []UseApproval
	err := readJSONLines(s.approvalsPath, func(data []byte) error {
		var approval UseApproval
		if err := json.Unmarshal(data, &approval); err != nil {
			return fmt.Errorf("%w: decode approval: %v", ErrInvalidRecord, err)
		}
		if err := approval.Validate(); err != nil {
			return err
		}
		approvals = append(approvals, approval)
		return nil
	})
	return approvals, err
}

func (s *FileStore) readRevocationsLocked() ([]ApprovalRevocation, error) {
	var revocations []ApprovalRevocation
	err := readJSONLines(s.revocationsPath, func(data []byte) error {
		var revocation ApprovalRevocation
		if err := json.Unmarshal(data, &revocation); err != nil {
			return fmt.Errorf("%w: decode revocation: %v", ErrInvalidRecord, err)
		}
		if err := revocation.Validate(); err != nil {
			return err
		}
		revocations = append(revocations, revocation)
		return nil
	})
	return revocations, err
}

func appendJSONLine(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func readJSONLines(path string, consume func([]byte) error) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	for scanner.Scan() {
		line := bytesTrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := consume(line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

// Administrator issues and revokes records with one currently selected
// governance identity.  SetPolicy and Rotate are control-plane operations and
// do not mutate already-issued records.
type Administrator struct {
	mu       sync.RWMutex
	identity GovernanceIdentity
	signer   ProtectedSigner
	keys     PublicKeyResolver
	store    ApprovalStore
	policy   ConsumptionPolicy
	now      func() time.Time
}

type AdministratorOptions struct {
	Now func() time.Time
}

func NewAdministrator(identity GovernanceIdentity, signer ProtectedSigner, keys PublicKeyResolver, store ApprovalStore, policy ConsumptionPolicy, options AdministratorOptions) (*Administrator, error) {
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	if signer == nil || keys == nil || store == nil {
		return nil, fmt.Errorf("%w: signer, key resolver, and store are required", ErrInvalidRecord)
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if policy.TenantID != identity.TenantID {
		return nil, fmt.Errorf("%w: identity and policy tenant differ", ErrInvalidRecord)
	}
	if _, err := keys.ResolveGovernanceKey(identity.TenantID, identity.KeyID, identity.KeyEpoch); err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Administrator{identity: identity, signer: signer, keys: keys, store: store, policy: policy, now: now}, nil
}

func (a *Administrator) SetPolicy(policy ConsumptionPolicy) error {
	if a == nil {
		return fmt.Errorf("%w: nil administrator", ErrInvalidRecord)
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if policy.TenantID != a.identity.TenantID {
		return fmt.Errorf("%w: policy tenant differs from identity", ErrInvalidRecord)
	}
	a.policy = policy
	return nil
}

// Rotate switches issuance to a newer key while retaining old public keys for
// verification.  It does not rewrite approvals or revocations.
func (a *Administrator) Rotate(identity GovernanceIdentity) error {
	if a == nil {
		return fmt.Errorf("%w: nil administrator", ErrInvalidRecord)
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if identity.TenantID != a.identity.TenantID || identity.KeyEpoch <= a.identity.KeyEpoch {
		return fmt.Errorf("%w: rotation must retain tenant and increase key epoch", ErrConflict)
	}
	if _, err := a.keys.ResolveGovernanceKey(identity.TenantID, identity.KeyID, identity.KeyEpoch); err != nil {
		return err
	}
	a.identity = identity
	return nil
}

type IssueRequest struct {
	ApprovalID       string
	EpisodeDigest    string
	AssessmentDigest string
	Purpose          string
	ConsumerClass    string
	Approver         string
	ExpiresAt        time.Time
	IssuedAt         time.Time // zero means the administrator clock.
}

func (a *Administrator) Issue(request IssueRequest) (UseApproval, error) {
	if a == nil {
		return UseApproval{}, fmt.Errorf("%w: nil administrator", ErrInvalidRecord)
	}
	a.mu.RLock()
	identity, policy, signer, store, nowFunc := a.identity, a.policy, a.signer, a.store, a.now
	a.mu.RUnlock()
	if err := policy.Validate(); err != nil {
		return UseApproval{}, err
	}
	scope := ApprovalScope{EpisodeDigest: request.EpisodeDigest, AssessmentDigest: request.AssessmentDigest, Purpose: request.Purpose, ConsumerClass: request.ConsumerClass}
	if err := scope.validate(); err != nil {
		return UseApproval{}, err
	}
	if !policy.Allows(request.Purpose, request.ConsumerClass) {
		return UseApproval{}, fmt.Errorf("%w: purpose and consumer class are not allowed by current policy", ErrUnauthorized)
	}
	now := nowFunc().UTC()
	issuedAt := request.IssuedAt.UTC()
	if request.IssuedAt.IsZero() {
		issuedAt = now
	}
	if issuedAt.After(now.Add(5 * time.Minute)) {
		return UseApproval{}, fmt.Errorf("%w: issuance is too far in the future", ErrInvalidRecord)
	}
	expiresAt := request.ExpiresAt.UTC()
	if expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		return UseApproval{}, fmt.Errorf("%w: expiry must be after issuance", ErrInvalidRecord)
	}
	if expiresAt.Sub(issuedAt) > policy.MaximumApprovalLifetime {
		return UseApproval{}, fmt.Errorf("%w: expiry exceeds current policy maximum", ErrInvalidRecord)
	}
	approvalID := request.ApprovalID
	if approvalID == "" {
		approvalID = uuid.NewString()
	}
	approval := UseApproval{
		SchemaVersion:      ApprovalSchemaVersion,
		ApprovalID:         approvalID,
		TenantID:           identity.TenantID,
		EpisodeDigest:      request.EpisodeDigest,
		AssessmentDigest:   request.AssessmentDigest,
		Purpose:            request.Purpose,
		ConsumerClass:      request.ConsumerClass,
		PolicyVersion:      policy.PolicyVersion,
		AssessmentPolicy:   cloneFreshnessPolicy(policy.Assessment),
		Approver:           request.Approver,
		IssuedAt:           issuedAt,
		ExpiresAt:          expiresAt,
		GovernanceKeyID:    identity.KeyID,
		GovernanceKeyEpoch: identity.KeyEpoch,
	}
	if err := approval.ValidateUnsigned(); err != nil {
		return UseApproval{}, err
	}
	if err := approval.Sign(signer, identity.PrivateKeyRef); err != nil {
		return UseApproval{}, err
	}
	if err := store.PutApproval(approval); err != nil {
		return UseApproval{}, err
	}
	return approval, nil
}

type RevokeRequest struct {
	RevocationID string
	ApprovalID   string
	Reason       string
	RevokedBy    string
	RevokedAt    time.Time // zero means the administrator clock.
}

func (a *Administrator) Revoke(request RevokeRequest) (ApprovalRevocation, error) {
	if a == nil {
		return ApprovalRevocation{}, fmt.Errorf("%w: nil administrator", ErrInvalidRecord)
	}
	a.mu.RLock()
	identity, signer, store, nowFunc := a.identity, a.signer, a.store, a.now
	a.mu.RUnlock()
	approval, err := store.GetApproval(request.ApprovalID)
	if err != nil {
		return ApprovalRevocation{}, err
	}
	if approval.TenantID != identity.TenantID {
		return ApprovalRevocation{}, fmt.Errorf("%w: approval tenant differs from identity", ErrUnauthorized)
	}
	existing, err := store.ListRevocations(request.ApprovalID)
	if err != nil {
		return ApprovalRevocation{}, err
	}
	if len(existing) != 0 {
		return ApprovalRevocation{}, ErrAlreadyRevoked
	}
	revokedAt := request.RevokedAt.UTC()
	if request.RevokedAt.IsZero() {
		revokedAt = nowFunc().UTC()
	}
	if revokedAt.After(nowFunc().UTC().Add(5 * time.Minute)) {
		return ApprovalRevocation{}, fmt.Errorf("%w: revocation is too far in the future", ErrInvalidRecord)
	}
	revocationID := request.RevocationID
	if revocationID == "" {
		revocationID = uuid.NewString()
	}
	revocation := ApprovalRevocation{
		SchemaVersion:      RevocationSchemaVersion,
		RevocationID:       revocationID,
		TenantID:           identity.TenantID,
		ApprovalID:         approval.ApprovalID,
		Reason:             request.Reason,
		RevokedBy:          request.RevokedBy,
		RevokedAt:          revokedAt,
		GovernanceKeyID:    identity.KeyID,
		GovernanceKeyEpoch: identity.KeyEpoch,
	}
	if err := revocation.Sign(signer, identity.PrivateKeyRef); err != nil {
		return ApprovalRevocation{}, err
	}
	if err := store.AppendRevocation(revocation); err != nil {
		return ApprovalRevocation{}, err
	}
	return revocation, nil
}

type ApprovalStatus string

const (
	StatusActive            ApprovalStatus = "active"
	StatusExpired           ApprovalStatus = "expired"
	StatusRevoked           ApprovalStatus = "revoked"
	StatusNotYetValid       ApprovalStatus = "not_yet_valid"
	StatusPolicyStale       ApprovalStatus = "policy_stale"
	StatusInvalidSignature  ApprovalStatus = "invalid_signature"
	StatusInvalidRevocation ApprovalStatus = "invalid_revocation"
)

type ApprovalInspection struct {
	Approval               UseApproval
	Revocations            []ApprovalRevocation
	Status                 ApprovalStatus
	ApprovalSignatureOK    bool
	RevocationSignaturesOK bool
	CurrentPolicy          bool
}

func (a *Administrator) Inspect(approvalID string) (ApprovalInspection, error) {
	if a == nil {
		return ApprovalInspection{}, fmt.Errorf("%w: nil administrator", ErrInvalidRecord)
	}
	a.mu.RLock()
	keys, store, policy, nowFunc := a.keys, a.store, a.policy, a.now
	a.mu.RUnlock()
	approval, err := store.GetApproval(approvalID)
	if err != nil {
		return ApprovalInspection{}, err
	}
	revocations, err := store.ListRevocations(approvalID)
	if err != nil {
		return ApprovalInspection{}, err
	}
	inspection := ApprovalInspection{Approval: approval, Revocations: revocations, RevocationSignaturesOK: true}
	public, keyErr := keys.ResolveGovernanceKey(approval.TenantID, approval.GovernanceKeyID, approval.GovernanceKeyEpoch)
	if keyErr == nil && approval.Verify(public) == nil {
		inspection.ApprovalSignatureOK = true
	}
	if !inspection.ApprovalSignatureOK {
		inspection.Status = StatusInvalidSignature
		return inspection, nil
	}
	inspection.CurrentPolicy = policy.TenantID == approval.TenantID && policy.PolicyVersion == approval.PolicyVersion && sameFreshnessPolicy(policy.Assessment, approval.AssessmentPolicy)
	if !inspection.CurrentPolicy {
		inspection.Status = StatusPolicyStale
		return inspection, nil
	}
	now := nowFunc().UTC()
	for _, revocation := range revocations {
		if revocation.TenantID != approval.TenantID || revocation.ApprovalID != approval.ApprovalID {
			inspection.RevocationSignaturesOK = false
			break
		}
		public, keyErr = keys.ResolveGovernanceKey(revocation.TenantID, revocation.GovernanceKeyID, revocation.GovernanceKeyEpoch)
		if keyErr != nil || revocation.Verify(public) != nil {
			inspection.RevocationSignaturesOK = false
			break
		}
		if !revocation.RevokedAt.After(now) {
			inspection.Status = StatusRevoked
			return inspection, nil
		}
	}
	if !inspection.RevocationSignaturesOK {
		inspection.Status = StatusInvalidRevocation
		return inspection, nil
	}
	if now.Before(approval.IssuedAt) {
		inspection.Status = StatusNotYetValid
	} else if !now.Before(approval.ExpiresAt) {
		inspection.Status = StatusExpired
	} else {
		inspection.Status = StatusActive
	}
	return inspection, nil
}

func (a *Administrator) Authorize(approvalID string, scope ApprovalScope) (UseApproval, error) {
	if err := scope.validate(); err != nil {
		return UseApproval{}, err
	}
	inspection, err := a.Inspect(approvalID)
	if err != nil {
		return UseApproval{}, err
	}
	switch inspection.Status {
	case StatusExpired:
		return UseApproval{}, ErrExpired
	case StatusRevoked:
		return UseApproval{}, ErrRevoked
	case StatusPolicyStale:
		return UseApproval{}, ErrPolicyStale
	case StatusNotYetValid, StatusInvalidSignature, StatusInvalidRevocation:
		return UseApproval{}, ErrUnauthorized
	case StatusActive:
		if !inspection.Approval.Scope().equal(scope) {
			return UseApproval{}, ErrInvalidScope
		}
		return inspection.Approval, nil
	default:
		return UseApproval{}, ErrUnauthorized
	}
}

func cloneFreshnessPolicy(policy AssessmentFreshnessPolicy) AssessmentFreshnessPolicy {
	policy.AssessmentNotBefore = policy.AssessmentNotBefore.UTC()
	policy.AllowedClassifierKinds = append([]string(nil), policy.AllowedClassifierKinds...)
	policy.AllowedRuleSetDigests = append([]string(nil), policy.AllowedRuleSetDigests...)
	return policy
}

func sameFreshnessPolicy(left, right AssessmentFreshnessPolicy) bool {
	return left.PolicyVersion == right.PolicyVersion && left.PolicyDigest == right.PolicyDigest && left.AssessmentNotBefore.UTC().Equal(right.AssessmentNotBefore.UTC()) && left.MaxAssessmentAge == right.MaxAssessmentAge && equalStrings(left.AllowedClassifierKinds, right.AllowedClassifierKinds) && equalStrings(left.AllowedRuleSetDigests, right.AllowedRuleSetDigests)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateIdentifier(value, name string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxIdentifierLength || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%w: invalid %s", ErrInvalidRecord, name)
	}
	return nil
}

func validatePurpose(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxPurposeLength || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%w: invalid purpose", ErrInvalidRecord)
	}
	if strings.EqualFold(value, "raw") || strings.EqualFold(value, "raw-export") || strings.EqualFold(value, "export") {
		return fmt.Errorf("%w: raw archive export requires a separate approval type", ErrUnauthorized)
	}
	return nil
}

func validateConsumerClass(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxConsumerClassLength || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%w: invalid consumer class", ErrInvalidRecord)
	}
	return nil
}

func validateStringList(values []string, name string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateIdentifier(value, name); err != nil {
			return err
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%w: duplicate %s", ErrInvalidRecord, name)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateDigest(value, name string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%w: %s must be a SHA-256 hex digest", ErrInvalidRecord, name)
	}
	if _, err := hex.DecodeString(value); err != nil || value != strings.ToLower(value) {
		return fmt.Errorf("%w: %s must be lowercase SHA-256 hex", ErrInvalidRecord, name)
	}
	return nil
}

func canonicalTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
