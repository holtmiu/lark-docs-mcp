package feishu

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCredentialBindingJSONOmitsTokenSecrets(t *testing.T) {
	binding := CredentialBinding{
		ID:           "cred-1",
		Provider:     ProviderFeishu,
		AuthType:     AuthTypeUser,
		UserID:       "user-1",
		OpenID:       "open-1",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		ExpiresAt:    time.Unix(1700000000, 0).UTC(),
		Scopes:       []string{"offline_access", "docs:doc:readonly"},
	}

	raw, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	jsonText := string(raw)
	if strings.Contains(jsonText, "access-secret") || strings.Contains(jsonText, "refresh-secret") {
		t.Fatalf("serialized credential leaked token secret: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"id":"cred-1"`) || !strings.Contains(jsonText, `"authType":"user"`) {
		t.Fatalf("serialized credential missing expected public fields: %s", jsonText)
	}
}

func TestActorContextJSONUsesExpectedFieldNames(t *testing.T) {
	actor := ActorContext{CredentialID: "cred-1", UserID: "user-1", OpenID: "open-1", AuthType: AuthTypeUser}
	raw, err := json.Marshal(actor)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	wantFields := []string{`"credentialId":"cred-1"`, `"userId":"user-1"`, `"openId":"open-1"`, `"authType":"user"`}
	for _, field := range wantFields {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("ActorContext JSON %s missing %s", raw, field)
		}
	}
}

func TestCredentialBindingIsExpired(t *testing.T) {
	if !(CredentialBinding{ExpiresAt: time.Now().Add(-time.Minute)}).IsExpired(time.Now()) {
		t.Fatal("expected binding with past expiry to be expired")
	}
	if (CredentialBinding{ExpiresAt: time.Now().Add(time.Hour)}).IsExpired(time.Now()) {
		t.Fatal("expected binding with future expiry not to be expired")
	}
	if !(CredentialBinding{}).IsExpired(time.Now()) {
		t.Fatal("expected zero expiry to be treated as expired")
	}
}
