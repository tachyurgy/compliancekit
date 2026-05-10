package magiclink_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

const testSigningKey = "test-signing-key-at-least-32-bytes-long-xxxxxxxxxxx"

func newService(t *testing.T) *magiclink.Service {
	t.Helper()
	pool := testhelp.OpenDB(t)
	return magiclink.NewService(pool, testSigningKey)
}

func TestGenerate_ReturnsDifferentTokensEachCall(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	seen := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		tok, _, err := svc.Generate(ctx, magiclink.KindProviderSignin, "prov-1", "sub-1", 0)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if tok == "" {
			t.Fatal("empty token")
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token: %s", tok)
		}
		seen[tok] = struct{}{}
	}
}

func TestGenerate_PlaintextNotStored_HashStoredInstead(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	pool := testhelp.OpenDB(t)
	svc = magiclink.NewService(pool, testSigningKey)
	ctx := context.Background()

	token, _, err := svc.Generate(ctx, magiclink.KindProviderSignin, "prov-1", "", 0)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify no row in DB contains the plaintext token anywhere.
	rows, err := pool.QueryContext(ctx, `SELECT hex(token_hash) FROM magic_link_tokens`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatalf("scan: %v", err)
		}
		hashes = append(hashes, h)
	}
	if len(hashes) != 1 {
		t.Fatalf("expected exactly 1 row, got %d", len(hashes))
	}
	// The stored hash must not equal the plaintext (a trivially easy check).
	if strings.EqualFold(hashes[0], token) {
		t.Fatalf("stored hash equals plaintext token")
	}
	// Expected hash should equal the service's HashHex of the plaintext.
	expected := svc.HashHex(token)
	if !strings.EqualFold(hashes[0], expected) {
		t.Fatalf("stored hash %s does not match HashHex %s", hashes[0], expected)
	}
}

func TestConsume_MarksToken_SecondConsumeErrors(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	token, _, err := svc.Generate(ctx, magiclink.KindProviderSignin, "prov-1", "user-1", 0)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claim, err := svc.Consume(ctx, token)
	if err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if claim.ProviderID != "prov-1" {
		t.Fatalf("claim.ProviderID = %q, want prov-1", claim.ProviderID)
	}
	if claim.Kind != magiclink.KindProviderSignin {
		t.Fatalf("claim.Kind = %q, want provider_signin", claim.Kind)
	}

	if _, err := svc.Consume(ctx, token); err == nil {
		t.Fatal("expected second Consume to fail")
	}
}

func TestConsume_ExpiredTokenRejects(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	token, _, err := svc.Generate(ctx, magiclink.KindProviderSignin, "prov-1", "sub-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := svc.Consume(ctx, token); err == nil {
		t.Fatal("expected expired token to reject")
	}
}

func TestConsume_UnknownToken_Rejects(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Consume(ctx, "not-a-real-token-1234567890abcdef"); err == nil {
		t.Fatal("expected unknown token to reject")
	}
}

func TestConsume_EmptyToken_Rejects(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Consume(ctx, ""); err == nil {
		t.Fatal("expected empty token to reject")
	}
}

func TestConsume_WrongSigningKey_Rejects(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	svc1 := magiclink.NewService(pool, testSigningKey)
	svc2 := magiclink.NewService(pool, "a-totally-different-signing-key-32-bytes-long-xx")
	ctx := context.Background()

	token, _, err := svc1.Generate(ctx, magiclink.KindProviderSignin, "prov-1", "sub-1", 0)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := svc2.Consume(ctx, token); err == nil {
		t.Fatal("expected Consume with wrong signing key to reject")
	}
}

func TestConsume_SlidingTTL_ExtendsExpiresAt(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	svc := magiclink.NewService(pool, testSigningKey)
	ctx := context.Background()

	// Sliding kind: KindParentUpload
	token, _, err := svc.Generate(ctx, magiclink.KindParentUpload, "prov-1", "child-1", 1*time.Second)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var origExp string
	if err := pool.QueryRowContext(ctx, `SELECT expires_at FROM magic_link_tokens`).Scan(&origExp); err != nil {
		t.Fatalf("select exp: %v", err)
	}

	// Consuming slides the window to now + TTLFor(KindParentUpload).
	if _, err := svc.Consume(ctx, token); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	var newExp string
	if err := pool.QueryRowContext(ctx, `SELECT expires_at FROM magic_link_tokens`).Scan(&newExp); err != nil {
		t.Fatalf("select exp2: %v", err)
	}
	if newExp == origExp {
		t.Fatalf("sliding TTL did not extend: both=%s", origExp)
	}
	// Consuming a sliding token should NOT mark consumed_at, i.e. it remains re-usable.
	var consumedAt *string
	if err := pool.QueryRowContext(ctx, `SELECT consumed_at FROM magic_link_tokens`).Scan(&consumedAt); err != nil {
		t.Fatalf("select consumed_at: %v", err)
	}
	if consumedAt != nil && *consumedAt != "" {
		t.Fatalf("sliding token should not be marked consumed; got %v", *consumedAt)
	}
	// Second consume should still work.
	if _, err := svc.Consume(ctx, token); err != nil {
		t.Fatalf("second Consume on sliding token: %v", err)
	}
}

func TestGenerate_EmptyTTL_UsesKindDefault(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	svc := magiclink.NewService(pool, testSigningKey)
	ctx := context.Background()

	for _, k := range []magiclink.Kind{
		magiclink.KindProviderSignup,
		magiclink.KindProviderSignin,
		magiclink.KindDocumentSign,
	} {
		_, _, err := svc.Generate(ctx, k, "prov-1", "sub-1", 0)
		if err != nil {
			t.Fatalf("Generate(%s): %v", k, err)
		}
	}
}

func TestRoundtrip_EveryKind(t *testing.T) {
	t.Parallel()
	cases := []magiclink.Kind{
		magiclink.KindProviderSignup,
		magiclink.KindProviderSignin,
		magiclink.KindParentUpload,
		magiclink.KindStaffUpload,
		magiclink.KindDocumentSign,
	}
	for _, k := range cases {
		k := k
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()
			pool := testhelp.OpenDB(t)
			svc := magiclink.NewService(pool, testSigningKey)
			ctx := context.Background()

			token, urlPath, err := svc.Generate(ctx, k, "prov-1", "sub-1", 5*time.Minute)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if token == "" {
				t.Fatal("empty token")
			}
			// Either ?t= (legacy paths) or /u/<token> (REQ-005 unified portal).
			// Both forms carry the token; the middleware accepts ?t= and the
			// frontend extracts the path-token before re-fetching with ?t=.
			if !strings.Contains(urlPath, "?t=") && !strings.Contains(urlPath, "/u/") {
				t.Fatalf("path missing token query: %s", urlPath)
			}
			claim, err := svc.Consume(ctx, token)
			if err != nil {
				t.Fatalf("Consume: %v", err)
			}
			if claim.Kind != k {
				t.Fatalf("kind = %s, want %s", claim.Kind, k)
			}
			if claim.ProviderID != "prov-1" {
				t.Fatalf("ProviderID = %q, want prov-1", claim.ProviderID)
			}
			if claim.SubjectID != "sub-1" {
				t.Fatalf("SubjectID = %q, want sub-1", claim.SubjectID)
			}
		})
	}
}

func TestRevoke_MarksAllConsumed(t *testing.T) {
	t.Parallel()
	svc := newService(t)
	pool := testhelp.OpenDB(t)
	svc = magiclink.NewService(pool, testSigningKey)
	ctx := context.Background()

	// 3 live tokens for the same subject.
	for i := 0; i < 3; i++ {
		if _, _, err := svc.Generate(ctx, magiclink.KindParentUpload, "prov-1", "child-1", 0); err != nil {
			t.Fatalf("Generate: %v", err)
		}
	}
	if err := svc.Revoke(ctx, magiclink.KindParentUpload, "child-1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	var live int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM magic_link_tokens WHERE consumed_at IS NULL AND kind=? AND subject_id=?`,
		string(magiclink.KindParentUpload), "child-1").Scan(&live); err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 0 {
		t.Fatalf("expected 0 live tokens after Revoke, got %d", live)
	}
}

func TestPathFor_AllKinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    magiclink.Kind
		want string
	}{
		{magiclink.KindProviderSignup, "/auth/signup?t=abc"},
		{magiclink.KindProviderSignin, "/auth/signin?t=abc"},
		{magiclink.KindParentUpload, "/u/abc"},
		{magiclink.KindStaffUpload, "/u/abc"},
		{magiclink.KindDocumentSign, "/sign?t=abc"},
		{magiclink.Kind("weird"), "/?t=abc"},
	}
	for _, tc := range cases {
		got := magiclink.PathFor(tc.k, "abc")
		if got != tc.want {
			t.Fatalf("PathFor(%s) = %s, want %s", tc.k, got, tc.want)
		}
	}
}

func TestTTLFor_AllKinds(t *testing.T) {
	t.Parallel()
	if magiclink.TTLFor(magiclink.KindProviderSignup) != 15*time.Minute {
		t.Fatal("signup TTL mismatch")
	}
	if magiclink.TTLFor(magiclink.KindProviderSignin) != 15*time.Minute {
		t.Fatal("signin TTL mismatch")
	}
	if magiclink.TTLFor(magiclink.KindParentUpload) != 7*24*time.Hour {
		t.Fatal("parent TTL mismatch")
	}
	if magiclink.TTLFor(magiclink.KindStaffUpload) != 7*24*time.Hour {
		t.Fatal("staff TTL mismatch")
	}
	if magiclink.TTLFor(magiclink.KindDocumentSign) != 72*time.Hour {
		t.Fatal("sign TTL mismatch")
	}
	if magiclink.TTLFor(magiclink.Kind("unknown")) != 1*time.Hour {
		t.Fatal("unknown TTL mismatch")
	}
}

func TestIsSliding(t *testing.T) {
	t.Parallel()
	if !magiclink.IsSliding(magiclink.KindParentUpload) {
		t.Fatal("parent should slide")
	}
	if !magiclink.IsSliding(magiclink.KindStaffUpload) {
		t.Fatal("staff should slide")
	}
	if magiclink.IsSliding(magiclink.KindProviderSignin) {
		t.Fatal("signin should not slide")
	}
}
