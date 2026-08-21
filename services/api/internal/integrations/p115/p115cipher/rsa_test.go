package p115cipher

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type rsaVector struct {
	SourceRepository    string `json:"sourceRepository"`
	SourceCommit        string `json:"sourceCommit"`
	ModuleVersion       string `json:"moduleVersion"`
	EncryptPlaintext    string `json:"encryptPlaintext"`
	EncryptCiphertext   string `json:"encryptCiphertext"`
	DecryptCiphertext   string `json:"decryptCiphertext"`
	DecryptPlaintextHex string `json:"decryptPlaintextHex"`
}

func TestRSAEncryptMatchesPinnedP115CipherVector(t *testing.T) {
	vector := loadRSAVector(t)
	ciphertext, err := RSAEncrypt([]byte(vector.EncryptPlaintext))
	if err != nil {
		t.Fatalf("RSAEncrypt() error = %v", err)
	}
	if ciphertext != vector.EncryptCiphertext {
		t.Fatalf("RSAEncrypt() = %q, want %q", ciphertext, vector.EncryptCiphertext)
	}
}

func TestRSADecryptMatchesPinnedP115CipherTransform(t *testing.T) {
	vector := loadRSAVector(t)
	plaintext, err := RSADecrypt(vector.DecryptCiphertext)
	if err != nil {
		t.Fatalf("RSADecrypt() error = %v", err)
	}
	if got := hex.EncodeToString(plaintext); got != vector.DecryptPlaintextHex {
		t.Fatalf("RSADecrypt() = %s, want %s", got, vector.DecryptPlaintextHex)
	}
}

func TestRSARejectsMalformedInput(t *testing.T) {
	if _, err := RSAEncrypt(nil); err == nil {
		t.Fatal("RSAEncrypt() accepted empty plaintext")
	}
	for _, ciphertext := range []string{"", "not-base64", "AQID"} {
		if _, err := RSADecrypt(ciphertext); err == nil {
			t.Fatalf("RSADecrypt() accepted %q", ciphertext)
		}
	}
}

func loadRSAVector(t *testing.T) rsaVector {
	t.Helper()
	data, err := os.ReadFile("testdata/p115rsa-0.0.5.4.json")
	if err != nil {
		t.Fatalf("read RSA vector: %v", err)
	}
	var vector rsaVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode RSA vector: %v", err)
	}
	if vector.SourceRepository != "https://github.com/ChenyangGao/p115client" ||
		vector.SourceCommit != "608a44396fea08d36131a68beb245be1fe17aa6d" || vector.ModuleVersion != "0.0.5.4" {
		t.Fatalf("unexpected RSA vector provenance: %+v", vector)
	}
	return vector
}
