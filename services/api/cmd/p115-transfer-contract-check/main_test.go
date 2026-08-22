package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
)

func TestLoadCommandInputRequiresRetainedWriteAcknowledgementAndRejectsCI(t *testing.T) {
	environment := validCommandEnvironment()
	delete(environment, "P115_TRANSFER_CONTRACT_CHECK_ACK")
	_, err := loadCommandInput(mapEnvironment(environment))
	if commandConfigCode(err) != "ack_required" {
		t.Fatalf("loadCommandInput() code = %q", commandConfigCode(err))
	}

	environment = validCommandEnvironment()
	environment["CI"] = "true"
	_, err = loadCommandInput(mapEnvironment(environment))
	if commandConfigCode(err) != "ci_forbidden" {
		t.Fatalf("loadCommandInput() CI code = %q", commandConfigCode(err))
	}
}

func TestWriteTransferFailurePrintsSafeRapidUploadProtocolEvidence(t *testing.T) {
	protocolErr := &p115integration.RapidUploadProtocolError{
		Phase:        p115integration.RapidUploadPhaseResponseDecrypt,
		DecryptPhase: p115integration.RapidUploadDecryptPhaseLZ4,
		ContentType:  "application/json",
		BodyShape:    "json_object",
		BodyBytes:    57,
	}
	var stderr bytes.Buffer
	writeTransferFailure(&stderr, fmt.Errorf("wrapped: %w", protocolErr))

	output := stderr.String()
	if !strings.Contains(output, "protocolPhase=response_decrypt decryptPhase=lz4 contentType=application/json bodyShape=json_object bodyBytes=57") {
		t.Fatalf("writeTransferFailure() output = %q", output)
	}
	for _, secret := range []string{"cookie", "errno", "response-body"} {
		if strings.Contains(output, secret) {
			t.Fatalf("writeTransferFailure() exposed %q: %s", secret, output)
		}
	}
}

func TestLoadCommandInputMapsSourceAndPlaybackPathsWithoutEchoing(t *testing.T) {
	input, err := loadCommandInput(mapEnvironment(validCommandEnvironment()))
	if err != nil {
		t.Fatalf("loadCommandInput() error = %v", err)
	}
	if input.SourceFile.RelativePath != "safe/source/video.mkv" || input.SourceFile.Size != 10_747_391_752 {
		t.Fatalf("source input = %+v", input.SourceFile)
	}
	if input.TargetDirectory.RootID != "0" || input.TargetDirectory.RelativePath != "/EmberPlayback" {
		t.Fatalf("target input = %+v", input.TargetDirectory)
	}
}

func TestRunCommandRefusesBeforeNetworkAndDoesNotEchoSecrets(t *testing.T) {
	environment := validCommandEnvironment()
	delete(environment, "P115_TRANSFER_CONTRACT_CHECK_ACK")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCommand(&stdout, &stderr, mapEnvironment(environment))
	if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code=ack_required") {
		t.Fatalf("runCommand() exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"source-cookie-secret", "playback-cookie-secret", "safe/source/video.mkv", "/EmberPlayback"} {
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("runCommand() exposed %q: %s", secret, stderr.String())
		}
	}
}

func TestLoadCommandInputRejectsSameCookiesAndMissingTargetPath(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantCode string
	}{
		{name: "same cookies", mutate: func(env map[string]string) { env["P115_PLAYBACK_COOKIE"] = env["P115_SOURCE_COOKIE"] }, wantCode: "cookies_same"},
		{name: "target missing", mutate: func(env map[string]string) { delete(env, "P115_PLAYBACK_TARGET_PATH") }, wantCode: "p115_playback_target_path_missing"},
		{name: "size invalid", mutate: func(env map[string]string) { env["P115_SOURCE_SIZE"] = "10GB" }, wantCode: "source_size_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := validCommandEnvironment()
			test.mutate(environment)
			_, err := loadCommandInput(mapEnvironment(environment))
			if code := commandConfigCode(err); code != test.wantCode {
				t.Fatalf("loadCommandInput() code=%q, want %q", code, test.wantCode)
			}
		})
	}
}

func validCommandEnvironment() map[string]string {
	return map[string]string{
		"P115_TRANSFER_CONTRACT_CHECK_ACK": transferCheckAcknowledgement,
		"P115_SOURCE_COOKIE":               "source-cookie-secret",
		"P115_PLAYBACK_COOKIE":             "playback-cookie-secret",
		"P115_SOURCE_USER_AGENT":           "source-provider-agent",
		"P115_PLAYBACK_USER_AGENT":         "playback-provider-agent",
		"P115_SOURCE_ROOT_ID":              "0",
		"P115_SOURCE_RELATIVE_PATH":        "safe/source/video.mkv",
		"P115_SOURCE_SIZE":                 "10747391752",
		"P115_PLAYBACK_ROOT_ID":            "0",
		"P115_PLAYBACK_TARGET_PATH":        "/EmberPlayback",
		"P115_TEST_CLIENT_USER_AGENT":      "Infuse-Contract/1.0",
	}
}

func mapEnvironment(values map[string]string) func(string) string {
	return func(name string) string {
		return values[name]
	}
}

func commandConfigCode(err error) string {
	configErr, ok := err.(*commandConfigError)
	if !ok {
		return ""
	}
	return configErr.code
}
