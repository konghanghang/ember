package p115

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	contractSourceCookie   = "source-cookie-secret"
	contractPlaybackCookie = "playback-cookie-secret"
	contractSourceUID      = "1111122222"
	contractPlaybackUID    = "3333344444"
	contractUserKey        = "provider-user-key-secret"
	contractRelativePath   = "private/source/fixture-video.mkv"
	contractSourceSHA1     = "0123456789ABCDEF0123456789ABCDEF01234567"
	contractRangeSHA1      = "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
)

func TestRunReadOnlyContractCheckUsesOnlyReadOperationsAndRedactsReport(t *testing.T) {
	provider := newFakeReadOnlyContractProvider()
	report, err := RunReadOnlyContractCheck(context.Background(), provider, fixtureContractCheckInput())
	if err != nil {
		t.Fatalf("RunReadOnlyContractCheck() error = %v", err)
	}

	wantCalls := []string{
		"validate:source", "validate:playback",
		"upload_info:source", "upload_info:playback",
		"resolve_source", "search_playback", "download:source", "hash_range", "download:playback",
	}
	if !reflect.DeepEqual(provider.calls, wantCalls) {
		t.Fatalf("provider calls = %v, want %v", provider.calls, wantCalls)
	}
	if !report.ReadOnly || report.SchemaVersion != 1 || report.Outcome != ContractCheckPassed || !report.Accounts.Distinct {
		t.Fatalf("unexpected report header: %+v", report)
	}
	if !report.SourceFile.Resolved || report.SourceFile.Size != 10_747_391_752 || report.SourceFile.SHA1Prefix != "01234567" {
		t.Fatalf("unexpected source report: %+v", report.SourceFile)
	}
	if !report.Playback.Found || !report.Playback.DownloadValidated || report.Playback.Download == nil || report.Playback.Download.Host != "playback.115.com" {
		t.Fatalf("unexpected playback report: %+v", report.Playback)
	}
	if report.SourceDownload.Host != "source.115.com" || report.SourceDownload.HeaderMode != DownloadHeadersSameUserAgentAndCookie {
		t.Fatalf("unexpected source download report: %+v", report.SourceDownload)
	}
	if !report.Range.HashComputed || report.Range.BytesRead != contractCheckRangeBytes || report.Range.SHA1Prefix != "89ABCDEF" {
		t.Fatalf("unexpected range report: %+v", report.Range)
	}
	if len(report.Steps) != len(wantCalls) {
		t.Fatalf("report steps = %d, want %d", len(report.Steps), len(wantCalls))
	}
	if provider.rangeRequest.Range != (ByteRange{Start: 0, End: contractCheckRangeBytes - 1}) {
		t.Fatalf("range request = %+v", provider.rangeRequest.Range)
	}
	if provider.sourceDownloadUserAgent != "source-provider-agent" || provider.playbackDownloadUserAgent != "Infuse-Contract/1.0" {
		t.Fatalf("download user agents source=%q playback=%q", provider.sourceDownloadUserAgent, provider.playbackDownloadUserAgent)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(report) error = %v", err)
	}
	output := string(encoded)
	for _, secret := range []string{
		contractSourceCookie, contractPlaybackCookie, contractSourceUID, contractPlaybackUID,
		contractUserKey, contractRelativePath, contractSourceSHA1, contractRangeSHA1,
		"source-pick-code-secret", "playback-pick-code-secret", "signed-source-secret", "signed-playback-secret",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("contract report exposed %q: %s", secret, output)
		}
	}
}

func TestRunReadOnlyContractCheckAllowsPlaybackMissWithoutPretendingDownloadWasValidated(t *testing.T) {
	provider := newFakeReadOnlyContractProvider()
	provider.playbackFiles = []File{}

	report, err := RunReadOnlyContractCheck(context.Background(), provider, fixtureContractCheckInput())
	if err != nil {
		t.Fatalf("RunReadOnlyContractCheck() error = %v", err)
	}
	if report.Outcome != ContractCheckPassed || report.Playback.Found || report.Playback.DownloadValidated {
		t.Fatalf("unexpected playback miss report: %+v", report.Playback)
	}
	if got := provider.calls[len(provider.calls)-1]; got != "hash_range" {
		t.Fatalf("last provider call = %q, want hash_range", got)
	}
}

func TestRunReadOnlyContractCheckFailsClosedBeforeProviderOnInvalidInput(t *testing.T) {
	provider := newFakeReadOnlyContractProvider()
	input := fixtureContractCheckInput()
	input.ExpectedSourceSize = 0

	_, err := RunReadOnlyContractCheck(context.Background(), provider, input)
	stage, code := ContractCheckFailure(err)
	if stage != "input" || code != "invalid_input" {
		t.Fatalf("ContractCheckFailure() stage=%q code=%q error=%v", stage, code, err)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("invalid input reached provider: %v", provider.calls)
	}
}

func TestRunReadOnlyContractCheckRejectsSameProviderAccountAndUploadIdentityMismatch(t *testing.T) {
	t.Run("same provider account", func(t *testing.T) {
		provider := newFakeReadOnlyContractProvider()
		provider.playbackIdentity.ProviderUserID = contractSourceUID

		_, err := RunReadOnlyContractCheck(context.Background(), provider, fixtureContractCheckInput())
		stage, code := ContractCheckFailure(err)
		if stage != "account_identity" || code != "accounts_same" {
			t.Fatalf("ContractCheckFailure() stage=%q code=%q error=%v", stage, code, err)
		}
		if len(provider.calls) != 2 {
			t.Fatalf("provider calls = %v", provider.calls)
		}
	})

	t.Run("upload identity mismatch", func(t *testing.T) {
		provider := newFakeReadOnlyContractProvider()
		provider.sourceUploadInfo.UserID = "99999"

		_, err := RunReadOnlyContractCheck(context.Background(), provider, fixtureContractCheckInput())
		stage, code := ContractCheckFailure(err)
		if stage != "source_upload_info" || code != "identity_mismatch" {
			t.Fatalf("ContractCheckFailure() stage=%q code=%q error=%v", stage, code, err)
		}
	})
}

func TestRunReadOnlyContractCheckMapsProviderErrorsWithoutProviderText(t *testing.T) {
	provider := newFakeReadOnlyContractProvider()
	provider.validateErr = ErrCredentialRejected

	_, err := RunReadOnlyContractCheck(context.Background(), provider, fixtureContractCheckInput())
	stage, code := ContractCheckFailure(err)
	if stage != "source_credential" || code != "credential_rejected" {
		t.Fatalf("ContractCheckFailure() stage=%q code=%q error=%v", stage, code, err)
	}
}

type fakeReadOnlyContractProvider struct {
	calls                     []string
	sourceIdentity            AccountIdentity
	playbackIdentity          AccountIdentity
	sourceUploadInfo          UploadInfo
	playbackUploadInfo        UploadInfo
	sourceFile                File
	playbackFiles             []File
	sourceDownload            DownloadURLResult
	playbackDownload          DownloadURLResult
	rangeHash                 FileRangeHash
	validateErr               error
	rangeRequest              FileRangeRequest
	sourceDownloadUserAgent   string
	playbackDownloadUserAgent string
}

func newFakeReadOnlyContractProvider() *fakeReadOnlyContractProvider {
	return &fakeReadOnlyContractProvider{
		sourceIdentity:     AccountIdentity{ProviderUserID: contractSourceUID},
		playbackIdentity:   AccountIdentity{ProviderUserID: contractPlaybackUID},
		sourceUploadInfo:   UploadInfo{UserID: contractSourceUID, UserKey: contractUserKey},
		playbackUploadInfo: UploadInfo{UserID: contractPlaybackUID, UserKey: contractUserKey},
		sourceFile: File{
			ID: "source-file-id", PickCode: "source-pick-code-secret", ParentID: "parent-id", Name: "fixture-video.mkv",
			SHA1: contractSourceSHA1, Size: 10_747_391_752,
		},
		playbackFiles: []File{{
			ID: "playback-file-id", PickCode: "playback-pick-code-secret", ParentID: "target-id", Name: "fixture-video.mkv",
			SHA1: contractSourceSHA1, Size: 10_747_391_752,
		}},
		sourceDownload: DownloadURLResult{
			URL: "https://source.115.com/file?signed-source-secret=1", ExpiresAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
			HeaderMode: DownloadHeadersSameUserAgentAndCookie, ConcurrentOpenLimit: 1,
		},
		playbackDownload: DownloadURLResult{
			URL: "https://playback.115.com/file?signed-playback-secret=1", ExpiresAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
			HeaderMode: DownloadHeadersSameUserAgent, ConcurrentOpenLimit: 2,
		},
		rangeHash: FileRangeHash{SHA1: contractRangeSHA1, BytesRead: contractCheckRangeBytes},
	}
}

func (provider *fakeReadOnlyContractProvider) ValidateCredential(_ context.Context, credential Credential) (AccountIdentity, error) {
	provider.calls = append(provider.calls, "validate:"+credential.AccountID)
	if provider.validateErr != nil {
		return AccountIdentity{}, provider.validateErr
	}
	if credential.AccountID == "source" {
		return provider.sourceIdentity, nil
	}
	return provider.playbackIdentity, nil
}

func (provider *fakeReadOnlyContractProvider) GetUploadInfo(_ context.Context, credential Credential) (UploadInfo, error) {
	provider.calls = append(provider.calls, "upload_info:"+credential.AccountID)
	if credential.AccountID == "source" {
		return provider.sourceUploadInfo, nil
	}
	return provider.playbackUploadInfo, nil
}

func (provider *fakeReadOnlyContractProvider) ResolveFileByPath(_ context.Context, _ Credential, _ FilePathQuery) (*File, error) {
	provider.calls = append(provider.calls, "resolve_source")
	file := provider.sourceFile
	return &file, nil
}

func (provider *fakeReadOnlyContractProvider) SearchBySHA1(_ context.Context, _ Credential, _ FileQuery) ([]File, error) {
	provider.calls = append(provider.calls, "search_playback")
	return provider.playbackFiles, nil
}

func (provider *fakeReadOnlyContractProvider) GetDownloadURL(_ context.Context, credential Credential, request DownloadURLRequest) (DownloadURLResult, error) {
	provider.calls = append(provider.calls, "download:"+credential.AccountID)
	if credential.AccountID == "source" {
		provider.sourceDownloadUserAgent = request.UserAgent
		return provider.sourceDownload, nil
	}
	provider.playbackDownloadUserAgent = request.UserAgent
	return provider.playbackDownload, nil
}

func (provider *fakeReadOnlyContractProvider) HashFileRange(_ context.Context, _ Credential, request FileRangeRequest) (FileRangeHash, error) {
	provider.calls = append(provider.calls, "hash_range")
	provider.rangeRequest = request
	return provider.rangeHash, nil
}

func fixtureContractCheckInput() ReadOnlyContractCheckInput {
	return ReadOnlyContractCheckInput{
		SourceCredential:    Credential{AccountID: "source", Cookie: contractSourceCookie, UserAgent: "source-provider-agent"},
		PlaybackCredential:  Credential{AccountID: "playback", Cookie: contractPlaybackCookie, UserAgent: "playback-provider-agent"},
		SourceFile:          FilePathQuery{RootID: "0", RelativePath: contractRelativePath},
		ExpectedSourceSize:  10_747_391_752,
		TestClientUserAgent: "Infuse-Contract/1.0",
	}
}

var _ ReadOnlyContractProvider = (*fakeReadOnlyContractProvider)(nil)
