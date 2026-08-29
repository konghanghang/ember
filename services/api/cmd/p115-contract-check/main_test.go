package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
)

func TestLoadCommandInputRequiresExplicitAcknowledgementAndRejectsCI(t *testing.T) {
	environment := validCommandEnvironment()
	delete(environment, "P115_CONTRACT_CHECK_ACK")
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

func TestWriteContractFailurePrintsOnlySafeDownloadPolicyEvidence(t *testing.T) {
	policyErr := &p115integration.DownloadURLPolicyError{
		Reason: p115integration.DownloadURLPolicyHostNotAllowed,
		Scheme: "https",
		Host:   "download.example",
	}
	var stderr bytes.Buffer
	writeContractFailure(&stderr, fmt.Errorf("wrapped: %w", policyErr))

	output := stderr.String()
	if !strings.Contains(output, "reason=host_not_allowed scheme=https host=download.example") {
		t.Fatalf("writeContractFailure() output = %q", output)
	}
	for _, secret := range []string{"/private/video.mkv", "signed=query-secret", "https://download.example/private"} {
		if strings.Contains(output, secret) {
			t.Fatalf("writeContractFailure() exposed %q: %s", secret, output)
		}
	}
}

func TestWriteContractFailureRedactsMalformedTypedEvidence(t *testing.T) {
	policyErr := &p115integration.DownloadURLPolicyError{
		Reason: "unexpected\nreason",
		Scheme: "https\nsecret",
		Host:   "download.example/private?signed=query-secret",
	}
	var stderr bytes.Buffer
	writeContractFailure(&stderr, policyErr)

	output := stderr.String()
	if !strings.Contains(output, "reason=unknown scheme=unknown host=redacted") {
		t.Fatalf("writeContractFailure() output = %q", output)
	}
	for _, secret := range []string{"private", "signed", "query-secret", "unexpected"} {
		if strings.Contains(output, secret) {
			t.Fatalf("writeContractFailure() exposed %q: %s", secret, output)
		}
	}
}

func TestLoadCommandInputReadsSecretsOnlyFromEnvironment(t *testing.T) {
	environment := validCommandEnvironment()
	input, err := loadCommandInput(mapEnvironment(environment))
	if err != nil {
		t.Fatalf("loadCommandInput() error = %v", err)
	}
	if input.SourceCredential.Cookie != "source-cookie-secret" || input.PlaybackCredential.Cookie != "playback-cookie-secret" {
		t.Fatalf("loadCommandInput() did not preserve Cookie values")
	}
	if input.SourceFile.RootID != "0" || input.SourceFile.RelativePath != "safe/source/video.mkv" || input.ExpectedSourceSize != 10_747_391_752 {
		t.Fatalf("loadCommandInput() source file = %+v", input.SourceFile)
	}
}

func TestLoadCommandInputRejectsMissingFieldsInvalidSizeAndSameCookies(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantCode string
	}{
		{name: "source cookie missing", mutate: func(env map[string]string) { delete(env, "P115_SOURCE_COOKIE") }, wantCode: "p115_source_cookie_missing"},
		{name: "size invalid", mutate: func(env map[string]string) { env["P115_SOURCE_SIZE"] = "10GB" }, wantCode: "source_size_invalid"},
		{name: "cookies same", mutate: func(env map[string]string) { env["P115_PLAYBACK_COOKIE"] = env["P115_SOURCE_COOKIE"] }, wantCode: "cookies_same"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := validCommandEnvironment()
			test.mutate(environment)
			_, err := loadCommandInput(mapEnvironment(environment))
			if code := commandConfigCode(err); code != test.wantCode {
				t.Fatalf("loadCommandInput() code = %q, want %q", code, test.wantCode)
			}
		})
	}
}

func TestRunCommandRefusesBeforeNetworkAndDoesNotEchoSecrets(t *testing.T) {
	environment := validCommandEnvironment()
	delete(environment, "P115_CONTRACT_CHECK_ACK")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCommand(&stdout, &stderr, mapEnvironment(environment))
	if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code=ack_required") {
		t.Fatalf("runCommand() exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"source-cookie-secret", "playback-cookie-secret", "safe/source/video.mkv"} {
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("runCommand() exposed secret %q: %s", secret, stderr.String())
		}
	}
}

func validCommandEnvironment() map[string]string {
	return map[string]string{
		"P115_CONTRACT_CHECK_ACK":     realCheckAcknowledgement,
		"P115_SOURCE_COOKIE":          "source-cookie-secret",
		"P115_PLAYBACK_COOKIE":        "playback-cookie-secret",
		"P115_SOURCE_USER_AGENT":      "source-provider-agent",
		"P115_PLAYBACK_USER_AGENT":    "playback-provider-agent",
		"P115_SOURCE_ROOT_ID":         "0",
		"P115_SOURCE_RELATIVE_PATH":   "safe/source/video.mkv",
		"P115_SOURCE_SIZE":            "10747391752",
		"P115_TEST_CLIENT_USER_AGENT": "Infuse-Contract/1.0",
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
