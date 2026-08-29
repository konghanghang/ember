package p115

import (
	"context"
	"time"
)

// Credential contains the decrypted account credential passed only to a Provider call.
type Credential struct {
	AccountID string `json:"accountId"`
	Cookie    string `json:"-"`
	AppType   string `json:"appType"`
	UserAgent string `json:"userAgent"`
}

// AccountIdentity is the non-secret identity returned by credential validation.
type AccountIdentity struct {
	ProviderUserID string `json:"providerUserId"`
	DisplayName    string `json:"displayName"`
}

// UploadInfo contains account-scoped values required by rapid-upload initialization.
type UploadInfo struct {
	UserID  string `json:"userId"`
	UserKey string `json:"-"`
}

// FileQuery identifies expected content and an optional target directory.
type FileQuery struct {
	SHA1     string `json:"-"`
	Size     int64  `json:"size"`
	ParentID string `json:"parentId,omitempty"`
}

// FilePathQuery resolves one relative file path below an explicit Provider root.
type FilePathQuery struct {
	RootID       string `json:"rootId"`
	RelativePath string `json:"-"`
}

// DirectoryPathQuery resolves one root-relative directory path.
type DirectoryPathQuery struct {
	RootID       string `json:"rootId"`
	RelativePath string `json:"-"`
}

// Directory is the Provider-neutral identity returned after exact path resolution.
type Directory struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}

// File is the Provider-neutral identity of a 115 file candidate.
type File struct {
	ID          string `json:"id"`
	PickCode    string `json:"-"`
	ParentID    string `json:"parentId"`
	Name        string `json:"name"`
	SHA1        string `json:"-"`
	Size        int64  `json:"size"`
	IsDirectory bool   `json:"isDirectory"`
}

// ByteRange is an inclusive byte range requested by a rapid-upload challenge.
type ByteRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// RapidUploadChallenge carries the validated range and opaque sign key for retry.
type RapidUploadChallenge struct {
	Range   ByteRange `json:"range"`
	SignKey string    `json:"-"`
}

// RapidUploadRequest contains content identity and optional challenge data.
type RapidUploadRequest struct {
	FileName       string `json:"fileName"`
	SHA1           string `json:"-"`
	Size           int64  `json:"size"`
	TargetParentID string `json:"targetParentId"`
	PreID          string `json:"-"`
	SignKey        string `json:"-"`
	SignValue      string `json:"-"`
}

// RapidUploadStatus is Ember's stable interpretation of external upload states.
type RapidUploadStatus string

const (
	RapidUploadReused                 RapidUploadStatus = "reused"
	RapidUploadRangeChallenge         RapidUploadStatus = "range_challenge"
	RapidUploadOrdinaryUploadRequired RapidUploadStatus = "ordinary_upload_required"
	RapidUploadProviderRejected       RapidUploadStatus = "provider_rejected"
)

// RapidUploadResult maps raw Provider responses into Ember-owned status semantics.
type RapidUploadResult struct {
	Status       RapidUploadStatus     `json:"status"`
	File         *File                 `json:"file,omitempty"`
	Challenge    *RapidUploadChallenge `json:"challenge,omitempty"`
	ProviderCode string                `json:"providerCode,omitempty"`
}

// DownloadURLRequest identifies a file and the actual playback client user agent.
type DownloadURLRequest struct {
	PickCode  string `json:"-"`
	UserAgent string `json:"userAgent"`
}

// DownloadHeaderMode describes headers the final playback client must preserve.
type DownloadHeaderMode string

const (
	DownloadHeadersNone                   DownloadHeaderMode = "none"
	DownloadHeadersSameUserAgent          DownloadHeaderMode = "same_user_agent"
	DownloadHeadersSameUserAgentAndCookie DownloadHeaderMode = "same_user_agent_and_cookie"
)

// DownloadURLResult describes a URL and its client-side header constraints.
type DownloadURLResult struct {
	URL                 string             `json:"-"`
	ExpiresAt           time.Time          `json:"expiresAt"`
	HeaderMode          DownloadHeaderMode `json:"headerMode"`
	ConcurrentOpenLimit int64              `json:"concurrentOpenLimit"`
}

// FileRangeRequest identifies one bounded source-file range to hash inside the Provider boundary.
type FileRangeRequest struct {
	File      File      `json:"file"`
	Range     ByteRange `json:"range"`
	UserAgent string    `json:"userAgent,omitempty"`
}

// FileRangeHash returns only the protocol hash and byte count, never the source bytes.
type FileRangeHash struct {
	SHA1      string `json:"-"`
	BytesRead int64  `json:"bytesRead"`
}

// CredentialValidator is the minimal account-validation boundary shared by Provider adapters.
type CredentialValidator interface {
	ValidateCredential(ctx context.Context, credential Credential) (AccountIdentity, error)
}

// Provider is the protocol boundary implemented by complete Cookie and future OpenAPI adapters.
type Provider interface {
	CredentialValidator
	GetUploadInfo(ctx context.Context, credential Credential) (UploadInfo, error)
	SearchBySHA1(ctx context.Context, credential Credential, query FileQuery) ([]File, error)
	ResolveFileByPath(ctx context.Context, credential Credential, query FilePathQuery) (*File, error)
	ResolveDirectoryByPath(ctx context.Context, credential Credential, query DirectoryPathQuery) (*Directory, error)
	InitRapidUpload(ctx context.Context, credential Credential, request RapidUploadRequest) (RapidUploadResult, error)
	GetDownloadURL(ctx context.Context, credential Credential, request DownloadURLRequest) (DownloadURLResult, error)
	FindTargetFile(ctx context.Context, credential Credential, query FileQuery) (*File, error)
	HashFileRange(ctx context.Context, credential Credential, request FileRangeRequest) (FileRangeHash, error)
	DeleteFile(ctx context.Context, credential Credential, fileID string) error
}
