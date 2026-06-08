package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileTokenStorePlaintextRoundTripAndWarning(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	store, err := NewFileTokenStore(path, nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	if store.Encrypted() {
		t.Fatal("store without key must not report encrypted storage")
	}
	if store.PlaintextWarning() == "" {
		t.Fatal("store without key should expose explicit plaintext warning")
	}

	want := testCredentialBinding("cred-plain")
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	got, err := store.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	assertCredentialBindingEqual(t, got, want)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(raw), want.AccessToken) || !strings.Contains(string(raw), want.RefreshToken) {
		t.Fatalf("plaintext store should persist opaque tokens for dev/test round trip")
	}
}

func TestFileTokenStoreEncryptedMigratesPlaintextFileBeforeSave(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	plaintextStore, err := NewFileTokenStore(path, nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore plaintext returned error: %v", err)
	}
	tokenA := testCredentialBinding("cred-a")
	if err := plaintextStore.Save(context.Background(), tokenA); err != nil {
		t.Fatalf("plaintext Save returned error: %v", err)
	}

	encryptedStore, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore encrypted returned error: %v", err)
	}
	tokenB := testCredentialBinding("cred-b")
	if err := encryptedStore.Save(context.Background(), tokenB); err != nil {
		t.Fatalf("encrypted Save returned error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	contents := string(raw)
	for _, secret := range []string{tokenA.AccessToken, tokenA.RefreshToken, tokenB.AccessToken, tokenB.RefreshToken} {
		if strings.Contains(contents, secret) {
			t.Fatalf("encrypted migration leaked plaintext token for %s", redactedCredentialSummary(tokenA))
		}
	}
	var decoded tokenStoreFile
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal migrated token store: %v", err)
	}
	if !decoded.Encrypted {
		t.Fatal("migrated token store must be marked encrypted")
	}
	gotA, err := encryptedStore.Get(context.Background(), tokenA.ID)
	if err != nil {
		t.Fatalf("Get migrated credential A returned error: %v", err)
	}
	assertCredentialBindingEqual(t, gotA, tokenA)
	gotB, err := encryptedStore.Get(context.Background(), tokenB.ID)
	if err != nil {
		t.Fatalf("Get credential B returned error: %v", err)
	}
	assertCredentialBindingEqual(t, gotB, tokenB)
}

func TestFileTokenStoreEncryptedMigratesLegacyPlaintextRecordInEncryptedFile(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	plaintextStore, err := NewFileTokenStore(path, nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore plaintext returned error: %v", err)
	}
	tokenA := testCredentialBinding("cred-legacy-mixed-a")
	if err := plaintextStore.Save(context.Background(), tokenA); err != nil {
		t.Fatalf("plaintext Save returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var legacyMixed tokenStoreFile
	if err := json.Unmarshal(raw, &legacyMixed); err != nil {
		t.Fatalf("Unmarshal plaintext token store: %v", err)
	}
	legacyMixed.Encrypted = true
	raw, err = json.MarshalIndent(legacyMixed, "", "  ")
	if err != nil {
		t.Fatalf("Marshal legacy mixed token store: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile legacy mixed token store: %v", err)
	}

	encryptedStore, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore encrypted returned error: %v", err)
	}
	tokenB := testCredentialBinding("cred-legacy-mixed-b")
	if err := encryptedStore.Save(context.Background(), tokenB); err != nil {
		t.Fatalf("encrypted Save returned error: %v", err)
	}
	finalRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile final token store: %v", err)
	}
	for _, secret := range []string{tokenA.AccessToken, tokenA.RefreshToken, tokenB.AccessToken, tokenB.RefreshToken} {
		if strings.Contains(string(finalRaw), secret) {
			t.Fatalf("legacy mixed migration leaked plaintext token")
		}
	}
	gotA, err := encryptedStore.Get(context.Background(), tokenA.ID)
	if err != nil {
		t.Fatalf("Get legacy migrated credential returned error: %v", err)
	}
	assertCredentialBindingEqual(t, gotA, tokenA)
}

func TestFileTokenStoreEncryptedMigratesPlaintextFileOnGet(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	plaintextStore, err := NewFileTokenStore(path, nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore plaintext returned error: %v", err)
	}
	want := testCredentialBinding("cred-get-migrate")
	if err := plaintextStore.Save(context.Background(), want); err != nil {
		t.Fatalf("plaintext Save returned error: %v", err)
	}

	encryptedStore, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore encrypted returned error: %v", err)
	}
	got, err := encryptedStore.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("Get migrated credential returned error: %v", err)
	}
	assertCredentialBindingEqual(t, got, want)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.Contains(string(raw), want.AccessToken) || strings.Contains(string(raw), want.RefreshToken) {
		t.Fatalf("Get migration leaked plaintext token for %s", redactedCredentialSummary(want))
	}
}

func TestFileTokenStoreEncryptedDoesNotPersistPlaintextTokens(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	store, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	if !store.Encrypted() {
		t.Fatal("store with key must report encrypted storage")
	}
	if store.PlaintextWarning() != "" {
		t.Fatalf("encrypted store warning = %q", store.PlaintextWarning())
	}

	want := testCredentialBinding("cred-encrypted")
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.Contains(string(raw), want.AccessToken) || strings.Contains(string(raw), want.RefreshToken) {
		t.Fatalf("encrypted file leaked token secret for %s", redactedCredentialSummary(want))
	}
	got, err := store.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	assertCredentialBindingEqual(t, got, want)
}

func TestFileTokenStoreMissingIDReturnsAuthRequiredError(t *testing.T) {
	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	_, err = store.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected missing credential error")
	}
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) {
		t.Fatalf("error = %T %v, want ConnectorError", err, err)
	}
	if connectorErr.Code != ErrAuthRequired {
		t.Fatalf("error code = %s, want %s", connectorErr.Code, ErrAuthRequired)
	}
}

func TestFileTokenStoreDeleteRemovesBinding(t *testing.T) {
	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", nil)
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	binding := testCredentialBinding("cred-delete")
	if err := store.Save(context.Background(), binding); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := store.Delete(context.Background(), binding.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := store.Get(context.Background(), binding.ID); err == nil {
		t.Fatal("expected deleted credential to be missing")
	}
}

func TestFileTokenStoreRejectsInvalidAESKeyLength(t *testing.T) {
	_, err := NewFileTokenStore(t.TempDir()+"/tokens.json", []byte("too-short"))
	if err == nil {
		t.Fatal("expected invalid AES key length error")
	}
	var connectorErr *ConnectorError
	if !errors.As(err, &connectorErr) {
		t.Fatalf("error = %T %v, want ConnectorError", err, err)
	}
	if connectorErr.Code != ErrInvalidInput {
		t.Fatalf("error code = %s, want %s", connectorErr.Code, ErrInvalidInput)
	}
}

func TestFileTokenStoreWritesFileMode0600(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	store, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	if err := store.Save(context.Background(), testCredentialBinding("cred-mode")); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token store file mode = %o, want 600", got)
	}
}

func TestFileTokenStoreResaveUsesFreshCiphertextNonce(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	store, err := NewFileTokenStore(path, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	binding := testCredentialBinding("cred-nonce")
	if err := store.Save(context.Background(), binding); err != nil {
		t.Fatalf("first Save returned error: %v", err)
	}
	first := encryptedRecordForID(t, path, binding.ID)
	if err := store.Save(context.Background(), binding); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}
	second := encryptedRecordForID(t, path, binding.ID)
	if first.AccessToken == second.AccessToken || first.RefreshToken == second.RefreshToken {
		t.Fatal("resaving same credential reused ciphertext; want fresh nonce-derived ciphertext")
	}
}

func TestFileTokenStoreConcurrentSaveGetDoesNotCorruptJSON(t *testing.T) {
	store, err := NewFileTokenStore(t.TempDir()+"/tokens.json", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewFileTokenStore returned error: %v", err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		id := "cred-concurrent-" + string(rune('a'+i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			binding := testCredentialBinding(id)
			if err := store.Save(ctx, binding); err != nil {
				t.Errorf("Save(%s) returned error: %v", id, err)
				return
			}
			if got, err := store.Get(ctx, id); err == nil && got.ID != id {
				t.Errorf("Get(%s) got ID %q", id, got.ID)
			}
		}()
	}
	wg.Wait()

	reopened, err := NewFileTokenStore(store.Path(), []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	for _, id := range []string{"cred-concurrent-a", "cred-concurrent-z"} {
		if _, err := reopened.Get(ctx, id); err != nil {
			t.Fatalf("reopened Get(%s) returned error: %v", id, err)
		}
	}
}

func testCredentialBinding(id string) CredentialBinding {
	return CredentialBinding{
		ID:           id,
		Provider:     ProviderFeishu,
		AuthType:     AuthTypeUser,
		TenantKey:    "tenant-1",
		UserID:       "user-1",
		OpenID:       "open-1",
		AccessToken:  "access-secret-" + id,
		RefreshToken: "refresh-secret-" + id,
		ExpiresAt:    time.Unix(1700000000, 0).UTC(),
		Scopes:       []string{"offline_access", "docs:doc:readonly"},
	}
}

func assertCredentialBindingEqual(t *testing.T, got, want CredentialBinding) {
	t.Helper()
	if got.ID != want.ID || got.Provider != want.Provider || got.AuthType != want.AuthType || got.TenantKey != want.TenantKey || got.UserID != want.UserID || got.OpenID != want.OpenID || got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || !got.ExpiresAt.Equal(want.ExpiresAt) || strings.Join(got.Scopes, ",") != strings.Join(want.Scopes, ",") {
		t.Fatalf("binding mismatch got %s want %s", redactedCredentialSummary(got), redactedCredentialSummary(want))
	}
}

func encryptedRecordForID(t *testing.T, path, id string) tokenStoreRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var file tokenStoreFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("Unmarshal token store: %v", err)
	}
	record, ok := file.Credentials[id]
	if !ok {
		t.Fatalf("credential %q not found in token store", id)
	}
	return record
}

func redactedCredentialSummary(binding CredentialBinding) string {
	return "id=" + binding.ID + " provider=" + string(binding.Provider) + " authType=" + string(binding.AuthType)
}
