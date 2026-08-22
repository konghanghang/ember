package p115cipher

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"testing"
)

type cipherVector struct {
	SourceRepository    string `json:"sourceRepository"`
	SourceCommit        string `json:"sourceCommit"`
	ModuleVersion       string `json:"moduleVersion"`
	Timestamp           int64  `json:"timestamp"`
	Token               string `json:"token"`
	DecodedPublicKeyHex string `json:"decodedPublicKeyHex"`
	RequestPlaintext    string `json:"requestPlaintext"`
	RequestCipherHex    string `json:"requestCipherHex"`
	ResponsePlaintext   string `json:"responsePlaintext"`
	ResponseCipherHex   string `json:"responseCipherHex"`
	Upload              struct {
		UserKey           string `json:"userKey"`
		UserID            string `json:"userID"`
		FileID            string `json:"fileID"`
		FileName          string `json:"fileName"`
		Target            string `json:"target"`
		FileSize          int64  `json:"fileSize"`
		PreID             string `json:"preID"`
		SignKey           string `json:"signKey"`
		SignValue         string `json:"signValue"`
		TopUpload         string `json:"topUpload"`
		AppVersion        string `json:"appVersion"`
		ExpectedSignature string `json:"expectedSignature"`
		ExpectedToken     string `json:"expectedToken"`
		ExpectedPlaintext string `json:"expectedPlaintext"`
		ExpectedDataHex   string `json:"expectedDataHex"`
	} `json:"upload"`
}

func TestVectorPinsReviewedUpstreamVersion(t *testing.T) {
	vector := loadCipherVector(t)
	if vector.SourceRepository != "https://github.com/ChenyangGao/p115client" ||
		vector.SourceCommit != "608a44396fea08d36131a68beb245be1fe17aa6d" ||
		vector.ModuleVersion != "0.0.5.4" {
		t.Fatalf("unexpected vector provenance: repository=%q commit=%q module=%q",
			vector.SourceRepository, vector.SourceCommit, vector.ModuleVersion)
	}
}

func TestTokenMatchesPinnedP115CipherVector(t *testing.T) {
	vector := loadCipherVector(t)
	token, err := EncodeToken(vector.Timestamp)
	if err != nil {
		t.Fatalf("EncodeToken() error = %v", err)
	}
	if token != vector.Token {
		t.Fatalf("EncodeToken() = %q, want %q", token, vector.Token)
	}

	decoded, err := DecodeToken(token)
	if err != nil {
		t.Fatalf("DecodeToken() error = %v", err)
	}
	if decoded.Timestamp != vector.Timestamp || hex.EncodeToString(decoded.PublicKey[:]) != vector.DecodedPublicKeyHex {
		t.Fatalf("DecodeToken() = %+v, want timestamp=%d publicKey=%s", decoded, vector.Timestamp, vector.DecodedPublicKeyHex)
	}
}

func TestDecodeTokenRejectsTamperedCRC(t *testing.T) {
	vector := loadCipherVector(t)
	raw, err := base64.StdEncoding.DecodeString(vector.Token)
	if err != nil {
		t.Fatalf("decode fixture token: %v", err)
	}
	raw[len(raw)-1] ^= 0x01
	if _, err := DecodeToken(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("DecodeToken() accepted a token with a tampered CRC")
	}
}

func TestTokenRejectsInvalidInput(t *testing.T) {
	if _, err := EncodeToken(-1); err == nil {
		t.Fatal("EncodeToken() accepted a negative timestamp")
	}
	if _, err := EncodeToken(int64(math.MaxUint32) + 1); err == nil {
		t.Fatal("EncodeToken() accepted a timestamp larger than uint32")
	}
	if _, err := DecodeToken("not-base64"); err == nil {
		t.Fatal("DecodeToken() accepted malformed base64")
	}
}

func TestEncryptRequestMatchesPinnedP115CipherVector(t *testing.T) {
	vector := loadCipherVector(t)
	ciphertext, err := EncryptRequest([]byte(vector.RequestPlaintext))
	if err != nil {
		t.Fatalf("EncryptRequest() error = %v", err)
	}
	if got := hex.EncodeToString(ciphertext); got != vector.RequestCipherHex {
		t.Fatalf("EncryptRequest() = %s, want %s", got, vector.RequestCipherHex)
	}
}

func TestDecryptResponseMatchesPinnedP115CipherVector(t *testing.T) {
	vector := loadCipherVector(t)
	ciphertext, err := hex.DecodeString(vector.ResponseCipherHex)
	if err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	plaintext, err := DecryptResponse(ciphertext)
	if err != nil {
		t.Fatalf("DecryptResponse() error = %v", err)
	}
	if string(plaintext) != vector.ResponsePlaintext {
		t.Fatalf("DecryptResponse() = %q, want %q", plaintext, vector.ResponsePlaintext)
	}
}

func TestDecryptResponseIgnoresTrailingPartialAESBlockLikePinnedP115Cipher(t *testing.T) {
	vector := loadCipherVector(t)
	ciphertext, err := hex.DecodeString(vector.ResponseCipherHex)
	if err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	// The pinned PyCryptodome path decrypts only complete AES blocks. A real
	// upload response contained the same protocol shape plus a 12-byte suffix.
	ciphertext = append(ciphertext, []byte("tail-12-byte")...)

	plaintext, err := DecryptResponse(ciphertext)
	if err != nil {
		t.Fatalf("DecryptResponse() error = %v", err)
	}
	if string(plaintext) != vector.ResponsePlaintext {
		t.Fatalf("DecryptResponse() = %q, want %q", plaintext, vector.ResponsePlaintext)
	}
}

func TestDecryptResponseMatchesPinnedLZ4TerminationSemantics(t *testing.T) {
	vector := loadCipherVector(t)
	ciphertext, err := hex.DecodeString(vector.ResponseCipherHex)
	if err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	framed, err := decryptRequest(ciphertext)
	if err != nil {
		t.Fatalf("decrypt response fixture: %v", err)
	}

	tests := []struct {
		name   string
		suffix []byte
	}{
		{name: "one trailing byte", suffix: []byte{0x7f}},
		{name: "two trailing bytes", suffix: []byte{0x7f, 0x7f}},
		{name: "zero terminator with tail", suffix: []byte{0x00, 0x00, 0x7f}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := EncryptRequest(append(append([]byte(nil), framed...), test.suffix...))
			if err != nil {
				t.Fatalf("EncryptRequest() error = %v", err)
			}
			plaintext, err := DecryptResponse(response)
			if err != nil {
				t.Fatalf("DecryptResponse() error = %v", err)
			}
			if string(plaintext) != vector.ResponsePlaintext {
				t.Fatalf("DecryptResponse() = %q, want %q", plaintext, vector.ResponsePlaintext)
			}
		})
	}
}

func TestDecryptResponseRejectsMalformedCiphertextAndLZ4Frame(t *testing.T) {
	_, err := DecryptResponse([]byte("not-aligned"))
	var decryptErr *ResponseDecryptError
	if !errors.As(err, &decryptErr) || decryptErr.Phase != ResponseDecryptPhaseAES {
		t.Fatalf("DecryptResponse() error = %v, want AES phase", err)
	}
	if err == nil {
		t.Fatal("DecryptResponse() accepted ciphertext without a complete AES block")
	}
	truncatedFrame, err := EncryptRequest([]byte{0x05, 0x00, 0x01})
	if err != nil {
		t.Fatalf("EncryptRequest() truncated frame error = %v", err)
	}
	_, err = DecryptResponse(truncatedFrame)
	if !errors.As(err, &decryptErr) || decryptErr.Phase != ResponseDecryptPhaseLZ4 {
		t.Fatalf("DecryptResponse() error = %v, want LZ4 phase", err)
	}
	if err == nil {
		t.Fatal("DecryptResponse() accepted a truncated LZ4 frame")
	}
}

func TestBuildUploadRequestMatchesPinnedP115CipherVector(t *testing.T) {
	vector := loadCipherVector(t)
	input := fixtureUploadPayload(vector)
	request, err := BuildUploadRequest(input, vector.Timestamp)
	if err != nil {
		t.Fatalf("BuildUploadRequest() error = %v", err)
	}
	if request.KEc != vector.Token {
		t.Fatalf("BuildUploadRequest() k_ec = %q, want %q", request.KEc, vector.Token)
	}
	plaintext, err := decryptRequest(request.Data)
	if err != nil {
		t.Fatalf("decryptRequest() error = %v", err)
	}
	if string(plaintext) != vector.Upload.ExpectedPlaintext {
		t.Fatalf("BuildUploadRequest() plaintext = %q, want %q", plaintext, vector.Upload.ExpectedPlaintext)
	}
	values, err := url.ParseQuery(string(plaintext))
	if err != nil {
		t.Fatalf("parse upload plaintext: %v", err)
	}
	if values.Get("sig") != vector.Upload.ExpectedSignature || values.Get("token") != vector.Upload.ExpectedToken {
		t.Fatalf("BuildUploadRequest() derived values sig=%q token=%q", values.Get("sig"), values.Get("token"))
	}
	if got := hex.EncodeToString(request.Data); got != vector.Upload.ExpectedDataHex {
		t.Fatalf("BuildUploadRequest() data = %s, want %s", got, vector.Upload.ExpectedDataHex)
	}
}

func TestBuildUploadRequestChangesWhenOneInputByteChanges(t *testing.T) {
	vector := loadCipherVector(t)
	input := fixtureUploadPayload(vector)
	baseline, err := BuildUploadRequest(input, vector.Timestamp)
	if err != nil {
		t.Fatalf("BuildUploadRequest() baseline error = %v", err)
	}
	input.FileID = "1123456789ABCDEF0123456789ABCDEF01234567"
	changed, err := BuildUploadRequest(input, vector.Timestamp)
	if err != nil {
		t.Fatalf("BuildUploadRequest() changed error = %v", err)
	}
	baselinePlaintext, err := decryptRequest(baseline.Data)
	if err != nil {
		t.Fatalf("decrypt baseline request: %v", err)
	}
	changedPlaintext, err := decryptRequest(changed.Data)
	if err != nil {
		t.Fatalf("decrypt changed request: %v", err)
	}
	baselineValues, _ := url.ParseQuery(string(baselinePlaintext))
	changedValues, _ := url.ParseQuery(string(changedPlaintext))
	if changedValues.Get("sig") == baselineValues.Get("sig") || changedValues.Get("token") == baselineValues.Get("token") || string(changed.Data) == string(baseline.Data) {
		t.Fatalf("one-byte FileID change did not alter all derived values")
	}
}

func TestBuildUploadRequestRejectsInvalidPayloads(t *testing.T) {
	vector := loadCipherVector(t)
	valid := fixtureUploadPayload(vector)
	tests := []struct {
		name      string
		mutate    func(*UploadPayload)
		timestamp int64
	}{
		{name: "missing user key", mutate: func(payload *UploadPayload) { payload.UserKey = "" }, timestamp: vector.Timestamp},
		{name: "missing file name", mutate: func(payload *UploadPayload) { payload.FileName = "" }, timestamp: vector.Timestamp},
		{name: "non numeric user id", mutate: func(payload *UploadPayload) { payload.UserID = "fixture-user" }, timestamp: vector.Timestamp},
		{name: "non ASCII protocol value", mutate: func(payload *UploadPayload) { payload.Target = "目录" }, timestamp: vector.Timestamp},
		{name: "non positive file size", mutate: func(payload *UploadPayload) { payload.FileSize = 0 }, timestamp: vector.Timestamp},
		{name: "non positive timestamp", mutate: func(*UploadPayload) {}, timestamp: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.mutate(&payload)
			if _, err := BuildUploadRequest(payload, test.timestamp); err == nil {
				t.Fatal("BuildUploadRequest() accepted invalid payload")
			}
		})
	}
}

func fixtureUploadPayload(vector cipherVector) UploadPayload {
	return UploadPayload{
		UserKey:    vector.Upload.UserKey,
		UserID:     vector.Upload.UserID,
		FileID:     vector.Upload.FileID,
		FileName:   vector.Upload.FileName,
		Target:     vector.Upload.Target,
		FileSize:   vector.Upload.FileSize,
		PreID:      vector.Upload.PreID,
		SignKey:    vector.Upload.SignKey,
		SignValue:  vector.Upload.SignValue,
		TopUpload:  vector.Upload.TopUpload,
		AppVersion: vector.Upload.AppVersion,
	}
}

func loadCipherVector(t *testing.T) cipherVector {
	t.Helper()
	data, err := os.ReadFile("testdata/p115cipher-0.0.5.4.json")
	if err != nil {
		t.Fatalf("read cipher vector: %v", err)
	}
	var vector cipherVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode cipher vector: %v", err)
	}
	return vector
}
