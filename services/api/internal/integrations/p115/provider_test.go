package p115

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type providerContractStub struct{}

func (providerContractStub) ValidateCredential(context.Context, Credential) (AccountIdentity, error) {
	return AccountIdentity{}, nil
}

func (providerContractStub) GetUploadInfo(context.Context, Credential) (UploadInfo, error) {
	return UploadInfo{}, nil
}

func (providerContractStub) SearchBySHA1(context.Context, Credential, FileQuery) ([]File, error) {
	return nil, nil
}

func (providerContractStub) InitRapidUpload(context.Context, Credential, RapidUploadRequest) (RapidUploadResult, error) {
	return RapidUploadResult{}, nil
}

func (providerContractStub) GetDownloadURL(context.Context, Credential, DownloadURLRequest) (DownloadURLResult, error) {
	return DownloadURLResult{}, nil
}

func (providerContractStub) FindTargetFile(context.Context, Credential, FileQuery) (*File, error) {
	return nil, nil
}

func (providerContractStub) DeleteFile(context.Context, Credential, string) error {
	return nil
}

var _ Provider = providerContractStub{}

func TestSecretProviderFieldsAreNotJSONSerializable(t *testing.T) {
	payload, err := json.Marshal(struct {
		Credential  Credential           `json:"credential"`
		UploadInfo  UploadInfo           `json:"uploadInfo"`
		Challenge   RapidUploadChallenge `json:"challenge"`
		Upload      RapidUploadRequest   `json:"upload"`
		DownloadURL DownloadURLResult    `json:"downloadUrl"`
		Query       FileQuery            `json:"query"`
		File        File                 `json:"file"`
	}{
		Credential:  Credential{AccountID: "account_1", Cookie: "cookie-secret"},
		UploadInfo:  UploadInfo{UserID: "user_1", UserKey: "user-key-secret"},
		Challenge:   RapidUploadChallenge{SignKey: "sign-key-secret"},
		Upload:      RapidUploadRequest{SignKey: "sign-key-secret", SignValue: "sign-value-secret"},
		DownloadURL: DownloadURLResult{URL: "https://download.example/signed-url-secret"},
		Query:       FileQuery{SHA1: "query-sha1-secret"},
		File:        File{PickCode: "pick-code-secret", SHA1: "file-sha1-secret"},
	})
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	encoded := string(payload)
	for _, secret := range []string{"cookie-secret", "user-key-secret", "sign-key-secret", "sign-value-secret", "signed-url-secret", "query-sha1-secret", "pick-code-secret", "file-sha1-secret"} {
		if !strings.Contains(encoded, secret) {
			continue
		}
		t.Fatalf("serialized Provider data exposed secret fields: %s", encoded)
	}
}

func TestRapidUploadStatusValuesAreStable(t *testing.T) {
	tests := map[RapidUploadStatus]string{
		RapidUploadReused:                 "reused",
		RapidUploadRangeChallenge:         "range_challenge",
		RapidUploadOrdinaryUploadRequired: "ordinary_upload_required",
		RapidUploadProviderRejected:       "provider_rejected",
	}
	for status, want := range tests {
		if string(status) != want {
			t.Fatalf("RapidUploadStatus %q = %q, want %q", status, status, want)
		}
	}
}

func TestDownloadHeaderModeValuesAreStable(t *testing.T) {
	tests := map[DownloadHeaderMode]string{
		DownloadHeadersNone:                   "none",
		DownloadHeadersSameUserAgent:          "same_user_agent",
		DownloadHeadersSameUserAgentAndCookie: "same_user_agent_and_cookie",
	}
	for mode, want := range tests {
		if string(mode) != want {
			t.Fatalf("DownloadHeaderMode %q = %q, want %q", mode, mode, want)
		}
	}
}
