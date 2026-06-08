package feishu

import "time"

type Provider string

const (
	ProviderFeishu Provider = "feishu"
	ProviderLark   Provider = "lark"
)

type AuthType string

const (
	AuthTypeTenant AuthType = "tenant"
	AuthTypeUser   AuthType = "user"
)

type CredentialBinding struct {
	ID           string    `json:"id"`
	Provider     Provider  `json:"provider"`
	AuthType     AuthType  `json:"authType"`
	TenantKey    string    `json:"tenantKey,omitempty"`
	UserID       string    `json:"userId,omitempty"`
	OpenID       string    `json:"openId,omitempty"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Scopes       []string  `json:"scopes"`
}

func (b CredentialBinding) IsExpired(now time.Time) bool {
	return b.ExpiresAt.IsZero() || !b.ExpiresAt.After(now)
}

type ActorContext struct {
	CredentialID string   `json:"credentialId,omitempty"`
	UserID       string   `json:"userId,omitempty"`
	OpenID       string   `json:"openId,omitempty"`
	AuthType     AuthType `json:"authType,omitempty"`
}

type ResourceType string

const (
	ResourceDocx      ResourceType = "docx"
	ResourceWiki      ResourceType = "wiki"
	ResourceDriveFile ResourceType = "drive_file"
	ResourceUnknown   ResourceType = "unknown"
)

type DocumentIdentity struct {
	Provider      Provider     `json:"provider"`
	ResourceType  ResourceType `json:"resourceType"`
	Token         string       `json:"token"`
	OriginalURL   string       `json:"originalUrl,omitempty"`
	NormalizedURL string       `json:"normalizedUrl,omitempty"`
	TenantKey     string       `json:"tenantKey,omitempty"`
}

type PermissionSnapshot struct {
	CanRead    bool   `json:"canRead"`
	CanWrite   bool   `json:"canWrite"`
	CanComment bool   `json:"canComment,omitempty"`
	Visibility string `json:"visibility,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type DocumentMetadata struct {
	DocumentID   string              `json:"documentId"`
	Title        string              `json:"title"`
	URL          string              `json:"url,omitempty"`
	OwnerID      string              `json:"ownerId,omitempty"`
	CreatedTime  string              `json:"createdTime,omitempty"`
	UpdatedTime  string              `json:"updatedTime,omitempty"`
	RevisionID   string              `json:"revisionId,omitempty"`
	ResourceType ResourceType        `json:"resourceType"`
	Permissions  *PermissionSnapshot `json:"permissions,omitempty"`
}

type NormalizedBlock struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Attrs    map[string]any         `json:"attrs,omitempty"`
	Children []NormalizedBlock      `json:"children,omitempty"`
	Source   *NormalizedBlockSource `json:"source,omitempty"`
}

type NormalizedBlockSource struct {
	Provider Provider `json:"provider"`
	RawType  string   `json:"rawType"`
	Raw      any      `json:"raw,omitempty"`
}

type ReadOptions struct {
	Format                string `json:"format,omitempty"`
	MaxBlocks             int    `json:"maxBlocks,omitempty"`
	MaxDepth              int    `json:"maxDepth,omitempty"`
	IncludeUnsupportedRaw bool   `json:"includeUnsupportedRaw,omitempty"`
}

type DocumentReadResult struct {
	Metadata  DocumentMetadata  `json:"metadata"`
	Blocks    []NormalizedBlock `json:"blocks"`
	Markdown  string            `json:"markdown,omitempty"`
	Warnings  []string          `json:"warnings,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

type CreateDocumentRequest struct {
	Title       string `json:"title"`
	FolderToken string `json:"folderToken,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
	DryRun      *bool  `json:"dryRun,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}

type AppendRequest struct {
	Markdown     string            `json:"markdown,omitempty"`
	Blocks       []NormalizedBlock `json:"blocks,omitempty"`
	AfterBlockID string            `json:"afterBlockId,omitempty"`
	DryRun       *bool             `json:"dryRun,omitempty"`
	OperationID  string            `json:"operationId,omitempty"`
}

type DocumentWriteResult struct {
	OperationID   string         `json:"operationId"`
	DocumentID    string         `json:"documentId"`
	ChangedBlocks []string       `json:"changedBlocks"`
	URL           string         `json:"url,omitempty"`
	DryRun        bool           `json:"dryRun"`
	Request       map[string]any `json:"request,omitempty"`
	Warnings      []string       `json:"warnings,omitempty"`
}
