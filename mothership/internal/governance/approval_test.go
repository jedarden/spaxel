package governance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type governanceFixture struct {
	admin       *Administrator
	store       *FileStore
	keys        *PublicKeySet
	privateKeys map[ProtectedKeyReference]ed25519.PrivateKey
	now         *time.Time
	policy      ConsumptionPolicy
}

func newGovernanceFixture(t *testing.T) governanceFixture {
	t.Helper()
	base := time.Date(2026, time.January, 5, 12, 0, 0, 0, time.UTC)
	seed1 := bytes.Repeat([]byte{0x11}, ed25519.SeedSize)
	seed2 := bytes.Repeat([]byte{0x22}, ed25519.SeedSize)
	private1 := ed25519.NewKeyFromSeed(seed1)
	private2 := ed25519.NewKeyFromSeed(seed2)
	publicSet, err := NewPublicKeySet(VerificationKey{
		TenantID: "tenant-home",
		KeyID:    "governance-key-1",
		KeyEpoch: 1,
		Public:   private1.Public().(ed25519.PublicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy := fixturePolicy(base, "policy-v1", "policy-digest-v1")
	now := base
	privateKeys := map[ProtectedKeyReference]ed25519.PrivateKey{
		"openbao://tenant-home/governance/key-1": private1,
		"openbao://tenant-home/governance/key-2": private2,
	}
	signer := SignFunc(func(ref ProtectedKeyReference, message []byte) ([]byte, error) {
		key, ok := privateKeys[ref]
		if !ok {
			return nil, errors.New("protected reference not found")
		}
		return ed25519.Sign(key, message), nil
	})
	admin, err := NewAdministrator(
		GovernanceIdentity{
			TenantID:      "tenant-home",
			KeyID:         "governance-key-1",
			KeyEpoch:      1,
			PrivateKeyRef: "openbao://tenant-home/governance/key-1",
		},
		signer,
		publicSet,
		store,
		policy,
		AdministratorOptions{Now: func() time.Time { return now }},
	)
	if err != nil {
		t.Fatal(err)
	}
	return governanceFixture{admin: admin, store: store, keys: publicSet, privateKeys: privateKeys, now: &now, policy: policy}
}

func fixturePolicy(now time.Time, version, digest string) ConsumptionPolicy {
	return ConsumptionPolicy{
		TenantID:      "tenant-home",
		PolicyVersion: version,
		PolicyDigest:  digest,
		Assessment: AssessmentFreshnessPolicy{
			PolicyVersion:          version,
			PolicyDigest:           digest,
			AssessmentNotBefore:    now.Add(-24 * time.Hour),
			MaxAssessmentAge:       24 * time.Hour,
			AllowedClassifierKinds: []string{"rules-v1"},
			AllowedRuleSetDigests:  []string{"rules-digest-v1"},
		},
		MaximumApprovalLifetime: 12 * time.Hour,
		AllowedUses: []AllowedUse{{
			Purpose:       "room-summary",
			ConsumerClass: "offline-agent",
		}},
	}
}

func fixtureDigest(label string) string {
	return fmtSHA256(label)
}

func fmtSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmtHex(digest[:])
}

func fmtHex(value []byte) string {
	const hexDigits = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for i, b := range value {
		result[i*2] = hexDigits[b>>4]
		result[i*2+1] = hexDigits[b&0x0f]
	}
	return string(result)
}

func issueFixture(t *testing.T, fixture governanceFixture, expiry time.Duration) UseApproval {
	t.Helper()
	approval, err := fixture.admin.Issue(IssueRequest{
		ApprovalID:       "approval-fixed-1",
		EpisodeDigest:    fixtureDigest("episode-1"),
		AssessmentDigest: fixtureDigest("assessment-1"),
		Purpose:          "room-summary",
		ConsumerClass:    "offline-agent",
		Approver:         "operator:alice",
		ExpiresAt:        fixture.now.Add(expiry),
	})
	if err != nil {
		t.Fatal(err)
	}
	return approval
}

func TestUseApprovalAdministrationFixtures(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, fixture governanceFixture)
	}{
		{
			name: "scope binds episode assessment purpose and consumer",
			run: func(t *testing.T, fixture governanceFixture) {
				approval := issueFixture(t, fixture, 2*time.Hour)
				if approval.EpisodeDigest != fixtureDigest("episode-1") || approval.AssessmentDigest != fixtureDigest("assessment-1") || approval.Purpose != "room-summary" || approval.ConsumerClass != "offline-agent" {
					t.Fatalf("approval scope was not bound: %+v", approval)
				}
				if approval.PolicyVersion != fixture.policy.PolicyVersion || approval.AssessmentPolicy.MaxAssessmentAge != fixture.policy.Assessment.MaxAssessmentAge {
					t.Fatalf("current assessment policy was not copied: %+v", approval.AssessmentPolicy)
				}
				if _, err := fixture.admin.Authorize(approval.ApprovalID, approval.Scope()); err != nil {
					t.Fatalf("valid scope was rejected: %v", err)
				}
				wrong := approval.Scope()
				wrong.ConsumerClass = "different-consumer"
				if _, err := fixture.admin.Authorize(approval.ApprovalID, wrong); !errors.Is(err, ErrInvalidScope) {
					t.Fatalf("wrong consumer class error = %v, want ErrInvalidScope", err)
				}
			},
		},
		{
			name: "expiry is enforced at inspection time",
			run: func(t *testing.T, fixture governanceFixture) {
				approval := issueFixture(t, fixture, time.Hour)
				*fixture.now = fixture.now.Add(time.Hour)
				inspection, err := fixture.admin.Inspect(approval.ApprovalID)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.Status != StatusExpired {
					t.Fatalf("status = %q, want %q", inspection.Status, StatusExpired)
				}
				if _, err := fixture.admin.Authorize(approval.ApprovalID, approval.Scope()); !errors.Is(err, ErrExpired) {
					t.Fatalf("authorization error = %v, want ErrExpired", err)
				}
			},
		},
		{
			name: "policy rotation fails closed",
			run: func(t *testing.T, fixture governanceFixture) {
				approval := issueFixture(t, fixture, 2*time.Hour)
				rotated := fixturePolicy(*fixture.now, "policy-v2", "policy-digest-v2")
				if err := fixture.admin.SetPolicy(rotated); err != nil {
					t.Fatal(err)
				}
				inspection, err := fixture.admin.Inspect(approval.ApprovalID)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.Status != StatusPolicyStale || inspection.CurrentPolicy {
					t.Fatalf("inspection = %+v, want stale policy", inspection)
				}
				if _, err := fixture.admin.Authorize(approval.ApprovalID, approval.Scope()); !errors.Is(err, ErrPolicyStale) {
					t.Fatalf("authorization error = %v, want ErrPolicyStale", err)
				}
			},
		},
		{
			name: "raw export purpose is not a use approval",
			run: func(t *testing.T, fixture governanceFixture) {
				_, err := fixture.admin.Issue(IssueRequest{
					EpisodeDigest: fixtureDigest("episode-raw"), AssessmentDigest: fixtureDigest("assessment-raw"),
					Purpose: "raw-export", ConsumerClass: "offline-agent", Approver: "operator:alice", ExpiresAt: fixture.now.Add(time.Hour),
				})
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("raw purpose error = %v, want ErrUnauthorized", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { test.run(t, newGovernanceFixture(t)) })
	}
}

func TestUseApprovalRotationAndRevocationFixtures(t *testing.T) {
	fixture := newGovernanceFixture(t)
	oldApproval := issueFixture(t, fixture, 2*time.Hour)

	seed2 := bytes.Repeat([]byte{0x22}, ed25519.SeedSize)
	private2 := ed25519.NewKeyFromSeed(seed2)
	if err := fixture.keys.Rotate(VerificationKey{
		TenantID: "tenant-home", KeyID: "governance-key-2", KeyEpoch: 2, Public: private2.Public().(ed25519.PublicKey),
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.admin.Rotate(GovernanceIdentity{
		TenantID: "tenant-home", KeyID: "governance-key-2", KeyEpoch: 2,
		PrivateKeyRef: "openbao://tenant-home/governance/key-2",
	}); err != nil {
		t.Fatal(err)
	}

	newApproval, err := fixture.admin.Issue(IssueRequest{
		EpisodeDigest: fixtureDigest("episode-2"), AssessmentDigest: fixtureDigest("assessment-2"),
		Purpose: "room-summary", ConsumerClass: "offline-agent", Approver: "operator:bob", ExpiresAt: fixture.now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if newApproval.GovernanceKeyID != "governance-key-2" || newApproval.GovernanceKeyEpoch != 2 {
		t.Fatalf("rotated approval key = %s/%d, want governance-key-2/2", newApproval.GovernanceKeyID, newApproval.GovernanceKeyEpoch)
	}
	oldInspection, err := fixture.admin.Inspect(oldApproval.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if oldInspection.Status != StatusActive || !oldInspection.ApprovalSignatureOK {
		t.Fatalf("old approval after rotation = %+v", oldInspection)
	}

	revocation, err := fixture.admin.Revoke(RevokeRequest{
		ApprovalID: oldApproval.ApprovalID, Reason: "assessment superseded", RevokedBy: "operator:bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if revocation.GovernanceKeyID != "governance-key-2" {
		t.Fatalf("revocation key = %q, want rotated key", revocation.GovernanceKeyID)
	}
	inspection, err := fixture.admin.Inspect(oldApproval.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Status != StatusRevoked || len(inspection.Revocations) != 1 || !inspection.RevocationSignaturesOK {
		t.Fatalf("revoked inspection = %+v", inspection)
	}
	if _, err := fixture.admin.Authorize(oldApproval.ApprovalID, oldApproval.Scope()); !errors.Is(err, ErrRevoked) {
		t.Fatalf("authorization error = %v, want ErrRevoked", err)
	}
	if _, err := fixture.admin.Revoke(RevokeRequest{ApprovalID: oldApproval.ApprovalID, Reason: "second reason", RevokedBy: "operator:bob"}); !errors.Is(err, ErrAlreadyRevoked) {
		t.Fatalf("second revocation error = %v, want ErrAlreadyRevoked", err)
	}
	approvals, err := fixture.store.ListApprovals()
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 2 {
		t.Fatalf("approval count = %d, want 2 immutable approvals", len(approvals))
	}
}

func TestUseApprovalPrivateReferenceAndStorePermissions(t *testing.T) {
	for _, reference := range []ProtectedKeyReference{"-----BEGIN PRIVATE KEY-----", "", "openbao://tenant\n/key"} {
		if err := reference.Validate(); !errors.Is(err, ErrInvalidKeyReference) {
			t.Errorf("reference %q error = %v, want ErrInvalidKeyReference", reference, err)
		}
	}
	fixture := newGovernanceFixture(t)
	approval := issueFixture(t, fixture, time.Hour)
	data, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "openbao://") || strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatalf("approval serialized protected authority material: %s", data)
	}
	for _, name := range []string{"approvals-v1.jsonl"} {
		info, err := os.Stat(filepath.Join(fixture.store.root, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s permissions = %o, want 600", name, info.Mode().Perm())
		}
	}
}
