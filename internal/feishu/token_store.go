package feishu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const plaintextTokenStoreWarning = "token store is plaintext; configure an AES-GCM key for encrypted storage"

type TokenStore interface {
	Save(ctx context.Context, binding CredentialBinding) error
	Get(ctx context.Context, id string) (CredentialBinding, error)
	Delete(ctx context.Context, id string) error
}

// FileTokenStore persists OAuth credential bindings to a JSON file.
//
// Its mutex only synchronizes callers using the same FileTokenStore instance in
// this process. It does not provide cross-process or multi-instance file
// locking; callers that share a token store path across processes must add an
// external lock.
type FileTokenStore struct {
	path             string
	mu               sync.Mutex
	aead             cipher.AEAD
	plaintextWarning string
}

type tokenStoreFile struct {
	Version     int                         `json:"version"`
	Encrypted   bool                        `json:"encrypted"`
	Credentials map[string]tokenStoreRecord `json:"credentials"`
}

type tokenStoreRecord struct {
	ID           string   `json:"id"`
	Provider     Provider `json:"provider"`
	AuthType     AuthType `json:"authType"`
	TenantKey    string   `json:"tenantKey,omitempty"`
	UserID       string   `json:"userId,omitempty"`
	OpenID       string   `json:"openId,omitempty"`
	AccessToken  string   `json:"accessToken,omitempty"`
	RefreshToken string   `json:"refreshToken,omitempty"`
	ExpiresAt    string   `json:"expiresAt"`
	Scopes       []string `json:"scopes"`
}

func NewFileTokenStore(path string, key []byte) (*FileTokenStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, newError(ErrInvalidInput, "token store path is required", nil)
	}
	store := &FileTokenStore{path: path, plaintextWarning: plaintextTokenStoreWarning}
	if len(key) > 0 {
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, newError(ErrInvalidInput, "invalid token encryption key; AES key must be 16, 24, or 32 bytes", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, newError(ErrInvalidInput, "failed to initialize token encryption", err)
		}
		store.aead = aead
		store.plaintextWarning = ""
	}
	return store, nil
}

func (s *FileTokenStore) Path() string { return s.path }

func (s *FileTokenStore) Encrypted() bool { return s != nil && s.aead != nil }

func (s *FileTokenStore) PlaintextWarning() string {
	if s == nil {
		return ""
	}
	return s.plaintextWarning
}

func (s *FileTokenStore) Save(ctx context.Context, binding CredentialBinding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(binding.ID) == "" {
		return newError(ErrInvalidInput, "credential id is required", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := s.migratePlaintextRecordsLocked(&file); err != nil {
		return err
	}
	record, err := s.recordFromBinding(binding)
	if err != nil {
		return err
	}
	file.Credentials[binding.ID] = record
	return s.writeLocked(file)
}

func (s *FileTokenStore) Get(ctx context.Context, id string) (CredentialBinding, error) {
	if err := ctx.Err(); err != nil {
		return CredentialBinding{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return CredentialBinding{}, newError(ErrInvalidInput, "credential id is required", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return CredentialBinding{}, err
	}
	if err := s.migratePlaintextRecordsLocked(&file); err != nil {
		return CredentialBinding{}, err
	}
	record, ok := file.Credentials[id]
	if !ok {
		return CredentialBinding{}, newError(ErrAuthRequired, "credential binding not found", nil)
	}
	return s.bindingFromRecord(record)
}

func (s *FileTokenStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return newError(ErrInvalidInput, "credential id is required", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := s.migratePlaintextRecordsLocked(&file); err != nil {
		return err
	}
	delete(file.Credentials, id)
	return s.writeLocked(file)
}

func (s *FileTokenStore) loadLocked() (tokenStoreFile, error) {
	file := tokenStoreFile{Version: 1, Encrypted: s.Encrypted(), Credentials: map[string]tokenStoreRecord{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, newError(ErrUpstream, "failed to read token store", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return file, nil
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return file, newError(ErrUpstream, "failed to decode token store", err)
	}
	if file.Credentials == nil {
		file.Credentials = map[string]tokenStoreRecord{}
	}
	if file.Encrypted && !s.Encrypted() {
		return file, newError(ErrAuthRequired, "token store is encrypted; configure token encryption key", nil)
	}
	return file, nil
}

func (s *FileTokenStore) migratePlaintextRecordsLocked(file *tokenStoreFile) error {
	if file == nil || !s.Encrypted() {
		return nil
	}
	changed := !file.Encrypted
	for id, record := range file.Credentials {
		accessToken, migrated, err := s.encryptIfPlaintextLegacy(record.AccessToken, file.Encrypted)
		if err != nil {
			return err
		}
		refreshToken, refreshMigrated, err := s.encryptIfPlaintextLegacy(record.RefreshToken, file.Encrypted)
		if err != nil {
			return err
		}
		if migrated || refreshMigrated {
			record.AccessToken = accessToken
			record.RefreshToken = refreshToken
			file.Credentials[id] = record
			changed = true
		}
	}
	file.Encrypted = true
	if !changed {
		return nil
	}
	return s.writeLocked(*file)
}

func (s *FileTokenStore) encryptIfPlaintextLegacy(value string, fileMarkedEncrypted bool) (string, bool, error) {
	if value == "" {
		return value, false, nil
	}
	if !fileMarkedEncrypted {
		encrypted, err := s.encrypt(value)
		return encrypted, true, err
	}
	decryptable, err := s.canDecrypt(value)
	if err != nil {
		return "", false, err
	}
	if decryptable {
		return value, false, nil
	}
	encrypted, err := s.encrypt(value)
	return encrypted, true, err
}

func (s *FileTokenStore) canDecrypt(value string) (bool, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false, nil
	}
	if len(raw) < s.aead.NonceSize() {
		return false, nil
	}
	nonce, encrypted := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	if _, err := s.aead.Open(nil, nonce, encrypted, nil); err != nil {
		return false, newError(ErrAuthRequired, "failed to decrypt token secret", err)
	}
	return true, nil
}

func (s *FileTokenStore) writeLocked(file tokenStoreFile) error {
	file.Version = 1
	file.Encrypted = s.Encrypted()
	if file.Credentials == nil {
		file.Credentials = map[string]tokenStoreRecord{}
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return newError(ErrUpstream, "failed to encode token store", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return newError(ErrUpstream, "failed to create token store directory", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".tokens-*.tmp")
	if err != nil {
		return newError(ErrUpstream, "failed to create token store temp file", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(raw)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpName)
		return newError(ErrUpstream, "failed to write token store", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return newError(ErrUpstream, "failed to close token store", closeErr)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return newError(ErrUpstream, "failed to set token store permissions", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return newError(ErrUpstream, "failed to replace token store", err)
	}
	return nil
}

func (s *FileTokenStore) recordFromBinding(binding CredentialBinding) (tokenStoreRecord, error) {
	accessToken, refreshToken := binding.AccessToken, binding.RefreshToken
	var err error
	if s.Encrypted() {
		accessToken, err = s.encrypt(accessToken)
		if err != nil {
			return tokenStoreRecord{}, err
		}
		refreshToken, err = s.encrypt(refreshToken)
		if err != nil {
			return tokenStoreRecord{}, err
		}
	}
	return tokenStoreRecord{ID: binding.ID, Provider: binding.Provider, AuthType: binding.AuthType, TenantKey: binding.TenantKey, UserID: binding.UserID, OpenID: binding.OpenID, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: binding.ExpiresAt.Format(timeFormatRFC3339Nano), Scopes: append([]string(nil), binding.Scopes...)}, nil
}

func (s *FileTokenStore) bindingFromRecord(record tokenStoreRecord) (CredentialBinding, error) {
	accessToken, refreshToken := record.AccessToken, record.RefreshToken
	var err error
	if s.Encrypted() {
		accessToken, err = s.decrypt(accessToken)
		if err != nil {
			return CredentialBinding{}, err
		}
		refreshToken, err = s.decrypt(refreshToken)
		if err != nil {
			return CredentialBinding{}, err
		}
	}
	expiresAt, err := parseTokenTime(record.ExpiresAt)
	if err != nil {
		return CredentialBinding{}, err
	}
	return CredentialBinding{ID: record.ID, Provider: record.Provider, AuthType: record.AuthType, TenantKey: record.TenantKey, UserID: record.UserID, OpenID: record.OpenID, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt, Scopes: append([]string(nil), record.Scopes...)}, nil
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func parseTokenTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(timeFormatRFC3339Nano, value)
	if err != nil {
		return time.Time{}, newError(ErrUpstream, "failed to decode token expiry", err)
	}
	return parsed, nil
}

func (s *FileTokenStore) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", newError(ErrUpstream, "failed to generate token nonce", err)
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *FileTokenStore) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", newError(ErrAuthRequired, "failed to decode token secret", err)
	}
	if len(raw) < s.aead.NonceSize() {
		return "", newError(ErrAuthRequired, "token secret ciphertext is invalid", fmt.Errorf("ciphertext too short"))
	}
	nonce, encrypted := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", newError(ErrAuthRequired, "failed to decrypt token secret", err)
	}
	return string(plaintext), nil
}
