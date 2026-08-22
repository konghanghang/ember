package p115

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	transferChallengeSHA1 = "FEDCBA9876543210FEDCBA9876543210FEDCBA98"
	transferPlaybackSHA1  = "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
)

func TestRunTransferContractCheckCompletesChallengeAndRetainsCreatedFile(t *testing.T) {
	provider := newFakeTransferContractProvider()
	report, err := RunTransferContractCheck(context.Background(), provider, fixtureTransferCheckInput())
	if err != nil {
		t.Fatalf("RunTransferContractCheck() error = %v", err)
	}

	wantCalls := []string{
		"validate:source", "validate:playback", "upload_info:source", "upload_info:playback",
		"resolve_source", "resolve_directory", "search_playback", "search_playback",
		"hash_range:source", "init_upload", "hash_range:source", "init_upload",
		"find_target", "download:playback", "hash_range:playback",
	}
	if !reflect.DeepEqual(provider.calls, wantCalls) {
		t.Fatalf("provider calls = %v, want %v", provider.calls, wantCalls)
	}
	if !report.WriteCapable || !report.WritePerformed || report.Outcome != ContractCheckPassed || !report.TargetDirectory.Resolved {
		t.Fatalf("unexpected report header: %+v", report)
	}
	if report.Transfer.Preexisting || !report.Transfer.Created || !report.Transfer.Retained ||
		!report.Transfer.SecondCheckPerformed || report.Transfer.DatabaseLockValidated ||
		report.Transfer.InitialStatus != RapidUploadRangeChallenge || report.Transfer.FinalStatus != RapidUploadReused ||
		report.Transfer.ChallengeCount != 1 {
		t.Fatalf("unexpected transfer report: %+v", report.Transfer)
	}
	if report.Cleanup.Attempted || !report.PlaybackRange.HashComputed || report.PlaybackRange.BytesRead != contractCheckRangeBytes {
		t.Fatalf("unexpected retention/range report: cleanup=%+v range=%+v", report.Cleanup, report.PlaybackRange)
	}
	if len(provider.uploadRequests) != 2 {
		t.Fatalf("upload requests = %d", len(provider.uploadRequests))
	}
	if first := provider.uploadRequests[0]; first.PreID != contractRangeSHA1 || first.SignKey != "" || first.SignValue != "" || first.TargetParentID != "300" {
		t.Fatalf("first upload request = %+v", first)
	}
	if second := provider.uploadRequests[1]; second.PreID != contractRangeSHA1 || second.SignKey != "fixture-sign-key" || second.SignValue != transferChallengeSHA1 {
		t.Fatalf("second upload request = %+v", second)
	}
	if provider.rangeRequests[len(provider.rangeRequests)-1].UserAgent != "Infuse-Contract/1.0" {
		t.Fatalf("playback range User-Agent = %q", provider.rangeRequests[len(provider.rangeRequests)-1].UserAgent)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(report) error = %v", err)
	}
	output := string(encoded)
	for _, secret := range []string{
		contractSourceCookie, contractPlaybackCookie, contractSourceUID, contractPlaybackUID, contractUserKey,
		contractRelativePath, "/EmberPlayback", contractSourceSHA1, contractRangeSHA1, transferChallengeSHA1,
		transferPlaybackSHA1, "sourcepickcode01", "targetpickcode01", "playback.115.com/private?signed=secret",
		"300", "source-file-id", "target-file-id",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("transfer report exposed %q: %s", secret, output)
		}
	}
}

func TestRunTransferContractCheckReusesPreexistingPlaybackFileWithoutUpload(t *testing.T) {
	provider := newFakeTransferContractProvider()
	provider.searchResults = [][]File{{provider.targetFile}}

	report, err := RunTransferContractCheck(context.Background(), provider, fixtureTransferCheckInput())
	if err != nil {
		t.Fatalf("RunTransferContractCheck() error = %v", err)
	}
	if !report.Transfer.Preexisting || report.Transfer.Created || !report.Transfer.Retained || report.Cleanup.Attempted {
		t.Fatalf("unexpected preexisting report: transfer=%+v cleanup=%+v", report.Transfer, report.Cleanup)
	}
	if report.WritePerformed {
		t.Fatalf("preexisting flow reported writePerformed=true")
	}
	for _, call := range provider.calls {
		if call == "init_upload" || call == "find_target" {
			t.Fatalf("preexisting flow called %s: %v", call, provider.calls)
		}
	}
}

func TestRunTransferContractCheckRejectsOrdinaryUploadAndRepeatedChallenge(t *testing.T) {
	t.Run("ordinary upload", func(t *testing.T) {
		provider := newFakeTransferContractProvider()
		provider.uploadResults = []RapidUploadResult{{Status: RapidUploadOrdinaryUploadRequired}}

		_, err := RunTransferContractCheck(context.Background(), provider, fixtureTransferCheckInput())
		stage, code, fileMayExist := TransferContractCheckFailure(err)
		if stage != "rapid_upload" || code != "ordinary_upload_required" || fileMayExist {
			t.Fatalf("TransferContractCheckFailure() stage=%q code=%q fileMayExist=%t error=%v", stage, code, fileMayExist, err)
		}
	})

	t.Run("repeated challenge", func(t *testing.T) {
		provider := newFakeTransferContractProvider()
		provider.uploadResults = []RapidUploadResult{
			{Status: RapidUploadRangeChallenge, Challenge: &RapidUploadChallenge{Range: ByteRange{Start: 1024, End: 2047}, SignKey: "fixture-sign-key"}},
			{Status: RapidUploadRangeChallenge, Challenge: &RapidUploadChallenge{Range: ByteRange{Start: 2048, End: 3071}, SignKey: "second-sign-key"}},
		}

		_, err := RunTransferContractCheck(context.Background(), provider, fixtureTransferCheckInput())
		stage, code, fileMayExist := TransferContractCheckFailure(err)
		if stage != "rapid_upload" || code != "repeated_challenge" || fileMayExist {
			t.Fatalf("TransferContractCheckFailure() stage=%q code=%q fileMayExist=%t error=%v", stage, code, fileMayExist, err)
		}
	})
}

func TestRunTransferContractCheckReportsRetainedRiskAfterReusedStatus(t *testing.T) {
	provider := newFakeTransferContractProvider()
	provider.findErr = ErrTargetFileNotVisible

	_, err := RunTransferContractCheck(context.Background(), provider, fixtureTransferCheckInput())
	stage, code, fileMayExist := TransferContractCheckFailure(err)
	if stage != "target_verify" || code != "target_file_not_visible" || !fileMayExist {
		t.Fatalf("TransferContractCheckFailure() stage=%q code=%q fileMayExist=%t error=%v", stage, code, fileMayExist, err)
	}
}

func TestRunTransferContractCheckRejectsInvalidInputBeforeProvider(t *testing.T) {
	provider := newFakeTransferContractProvider()
	input := fixtureTransferCheckInput()
	input.TargetDirectory.RelativePath = "/"

	_, err := RunTransferContractCheck(context.Background(), provider, input)
	stage, code, fileMayExist := TransferContractCheckFailure(err)
	if stage != "input" || code != "invalid_input" || fileMayExist || len(provider.calls) != 0 {
		t.Fatalf("invalid input stage=%q code=%q fileMayExist=%t calls=%v", stage, code, fileMayExist, provider.calls)
	}
}

type fakeTransferContractProvider struct {
	calls              []string
	sourceIdentity     AccountIdentity
	playbackIdentity   AccountIdentity
	sourceUploadInfo   UploadInfo
	playbackUploadInfo UploadInfo
	sourceFile         File
	targetDirectory    Directory
	targetFile         File
	searchResults      [][]File
	uploadResults      []RapidUploadResult
	uploadRequests     []RapidUploadRequest
	rangeRequests      []FileRangeRequest
	findErr            error
}

func newFakeTransferContractProvider() *fakeTransferContractProvider {
	return &fakeTransferContractProvider{
		sourceIdentity:     AccountIdentity{ProviderUserID: contractSourceUID},
		playbackIdentity:   AccountIdentity{ProviderUserID: contractPlaybackUID},
		sourceUploadInfo:   UploadInfo{UserID: contractSourceUID, UserKey: contractUserKey},
		playbackUploadInfo: UploadInfo{UserID: contractPlaybackUID, UserKey: contractUserKey},
		sourceFile: File{
			ID: "source-file-id", PickCode: "sourcepickcode01", ParentID: "100", Name: "fixture-video.mkv",
			SHA1: contractSourceSHA1, Size: 10_747_391_752,
		},
		targetDirectory: Directory{ID: "300", ParentID: "0", Name: "EmberPlayback", Path: "/EmberPlayback"},
		targetFile: File{
			ID: "target-file-id", PickCode: "targetpickcode01", ParentID: "300", Name: "fixture-video.mkv",
			SHA1: contractSourceSHA1, Size: 10_747_391_752,
		},
		searchResults: [][]File{{}, {}},
		uploadResults: []RapidUploadResult{
			{Status: RapidUploadRangeChallenge, Challenge: &RapidUploadChallenge{Range: ByteRange{Start: 1024, End: 2047}, SignKey: "fixture-sign-key"}},
			{Status: RapidUploadReused},
		},
	}
}

func (provider *fakeTransferContractProvider) ValidateCredential(_ context.Context, credential Credential) (AccountIdentity, error) {
	provider.calls = append(provider.calls, "validate:"+credential.AccountID)
	if credential.AccountID == "source" {
		return provider.sourceIdentity, nil
	}
	return provider.playbackIdentity, nil
}

func (provider *fakeTransferContractProvider) GetUploadInfo(_ context.Context, credential Credential) (UploadInfo, error) {
	provider.calls = append(provider.calls, "upload_info:"+credential.AccountID)
	if credential.AccountID == "source" {
		return provider.sourceUploadInfo, nil
	}
	return provider.playbackUploadInfo, nil
}

func (provider *fakeTransferContractProvider) ResolveFileByPath(_ context.Context, _ Credential, _ FilePathQuery) (*File, error) {
	provider.calls = append(provider.calls, "resolve_source")
	file := provider.sourceFile
	return &file, nil
}

func (provider *fakeTransferContractProvider) ResolveDirectoryByPath(_ context.Context, _ Credential, _ DirectoryPathQuery) (*Directory, error) {
	provider.calls = append(provider.calls, "resolve_directory")
	directory := provider.targetDirectory
	return &directory, nil
}

func (provider *fakeTransferContractProvider) SearchBySHA1(_ context.Context, _ Credential, _ FileQuery) ([]File, error) {
	provider.calls = append(provider.calls, "search_playback")
	if len(provider.searchResults) == 0 {
		return []File{}, nil
	}
	result := provider.searchResults[0]
	provider.searchResults = provider.searchResults[1:]
	return result, nil
}

func (provider *fakeTransferContractProvider) InitRapidUpload(_ context.Context, _ Credential, request RapidUploadRequest) (RapidUploadResult, error) {
	provider.calls = append(provider.calls, "init_upload")
	provider.uploadRequests = append(provider.uploadRequests, request)
	if len(provider.uploadResults) == 0 {
		return RapidUploadResult{}, errors.New("unexpected upload call")
	}
	result := provider.uploadResults[0]
	provider.uploadResults = provider.uploadResults[1:]
	return result, nil
}

func (provider *fakeTransferContractProvider) FindTargetFile(_ context.Context, _ Credential, _ FileQuery) (*File, error) {
	provider.calls = append(provider.calls, "find_target")
	if provider.findErr != nil {
		return nil, provider.findErr
	}
	file := provider.targetFile
	return &file, nil
}

func (provider *fakeTransferContractProvider) GetDownloadURL(_ context.Context, credential Credential, _ DownloadURLRequest) (DownloadURLResult, error) {
	provider.calls = append(provider.calls, "download:"+credential.AccountID)
	return DownloadURLResult{
		URL: "https://playback.115.com/private?signed=secret", ExpiresAt: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
		HeaderMode: DownloadHeadersSameUserAgent, ConcurrentOpenLimit: 2,
	}, nil
}

func (provider *fakeTransferContractProvider) HashFileRange(_ context.Context, credential Credential, request FileRangeRequest) (FileRangeHash, error) {
	provider.calls = append(provider.calls, "hash_range:"+credential.AccountID)
	provider.rangeRequests = append(provider.rangeRequests, request)
	if credential.AccountID == "playback" {
		return FileRangeHash{SHA1: transferPlaybackSHA1, BytesRead: request.Range.End - request.Range.Start + 1}, nil
	}
	if request.Range.Start == 0 {
		return FileRangeHash{SHA1: contractRangeSHA1, BytesRead: request.Range.End - request.Range.Start + 1}, nil
	}
	return FileRangeHash{SHA1: transferChallengeSHA1, BytesRead: request.Range.End - request.Range.Start + 1}, nil
}

func fixtureTransferCheckInput() TransferContractCheckInput {
	return TransferContractCheckInput{
		SourceCredential:    Credential{AccountID: "source", Cookie: contractSourceCookie, UserAgent: "source-provider-agent"},
		PlaybackCredential:  Credential{AccountID: "playback", Cookie: contractPlaybackCookie, UserAgent: "playback-provider-agent"},
		SourceFile:          FilePathQuery{RootID: "0", RelativePath: contractRelativePath, Size: 10_747_391_752},
		TargetDirectory:     DirectoryPathQuery{RootID: "0", RelativePath: "/EmberPlayback"},
		TestClientUserAgent: "Infuse-Contract/1.0",
	}
}

var _ TransferContractProvider = (*fakeTransferContractProvider)(nil)
