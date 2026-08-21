package p115cipher

import (
	"crypto/md5"  // #nosec G501 -- the external protocol requires MD5 for request compatibility, not password security.
	"crypto/sha1" // #nosec G505 -- the external protocol requires SHA-1 for request compatibility, not password security.
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	errInvalidUploadPayload = errors.New("p115 upload payload invalid")
	protocolMD5Salt         = []byte("Qclm8MGWUv59TnrR0XPg")
)

// UploadPayload contains the account-scoped values needed to build one upload-init request.
// Callers must keep UserKey and derived values out of logs and persisted request records.
type UploadPayload struct {
	UserKey    string `json:"-"`
	UserID     string `json:"-"`
	FileID     string `json:"-"`
	FileName   string `json:"-"`
	Target     string `json:"-"`
	FileSize   int64  `json:"-"`
	PreID      string `json:"-"`
	SignKey    string `json:"-"`
	SignValue  string `json:"-"`
	TopUpload  string `json:"-"`
	AppVersion string `json:"-"`
}

// UploadRequest is the encrypted body and k_ec parameter accepted by upload initialization.
type UploadRequest struct {
	KEc  string `json:"-"`
	Data []byte `json:"-"`
}

// BuildUploadRequest derives the protocol signature/token and encrypts a sorted form payload.
func BuildUploadRequest(payload UploadPayload, timestamp int64) (UploadRequest, error) {
	if err := validateUploadPayload(payload, timestamp); err != nil {
		return UploadRequest{}, err
	}

	fileSize := strconv.FormatInt(payload.FileSize, 10)
	timestampValue := strconv.FormatInt(timestamp, 10)
	signature := uploadSignature(payload)
	token, err := uploadToken(payload, fileSize, timestampValue)
	if err != nil {
		return UploadRequest{}, err
	}

	values := url.Values{
		"appversion": {payload.AppVersion},
		"fileid":     {payload.FileID},
		"filename":   {payload.FileName},
		"filesize":   {fileSize},
		"sig":        {signature},
		"t":          {timestampValue},
		"target":     {payload.Target},
		"token":      {token},
		"topupload":  {payload.TopUpload},
		"userid":     {payload.UserID},
		"userkey":    {payload.UserKey},
	}
	if payload.PreID != "" {
		values.Set("preid", payload.PreID)
	}
	if payload.SignKey != "" {
		values.Set("sign_key", payload.SignKey)
	}
	if payload.SignValue != "" {
		values.Set("sign_val", payload.SignValue)
	}
	plaintext := values.Encode()
	data, err := EncryptRequest([]byte(plaintext))
	if err != nil {
		return UploadRequest{}, err
	}
	kEC, err := EncodeToken(timestamp)
	if err != nil {
		return UploadRequest{}, err
	}
	return UploadRequest{KEc: kEC, Data: data}, nil
}

func validateUploadPayload(payload UploadPayload, timestamp int64) error {
	values := []string{
		payload.UserKey,
		payload.UserID,
		payload.FileID,
		payload.Target,
		payload.PreID,
		payload.SignKey,
		payload.SignValue,
		payload.TopUpload,
		payload.AppVersion,
	}
	for _, value := range values {
		if !isASCII(value) {
			return errInvalidUploadPayload
		}
	}
	if payload.UserKey == "" || payload.UserID == "" || payload.FileID == "" || payload.FileName == "" ||
		payload.Target == "" || payload.FileSize <= 0 || payload.AppVersion == "" ||
		payload.TopUpload == "" || timestamp <= 0 {
		return errInvalidUploadPayload
	}
	if !utf8.ValidString(payload.FileName) || len(payload.FileName) > 1024 || strings.ContainsAny(payload.FileName, "\r\n") {
		return errInvalidUploadPayload
	}
	if _, err := strconv.ParseUint(payload.UserID, 10, 64); err != nil {
		return errInvalidUploadPayload
	}
	if timestamp > int64(^uint32(0)) {
		return errInvalidTimestamp
	}
	return nil
}

func uploadSignature(payload UploadPayload) string {
	inner := sha1.Sum([]byte(payload.UserID + payload.FileID + payload.Target + "0"))
	hash := sha1.New() // #nosec G401 -- protocol compatibility hash.
	_, _ = hash.Write([]byte(payload.UserKey))
	_, _ = hash.Write([]byte(hex.EncodeToString(inner[:])))
	_, _ = hash.Write([]byte("000000"))
	return strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))
}

func uploadToken(payload UploadPayload, fileSize, timestamp string) (string, error) {
	numericUserID, err := strconv.ParseUint(payload.UserID, 10, 64)
	if err != nil {
		return "", errInvalidUploadPayload
	}
	userIDHash := md5.Sum([]byte(strconv.FormatUint(numericUserID, 10))) // #nosec G401 -- protocol compatibility hash.
	hash := md5.New()                                                    // #nosec G401 -- protocol compatibility hash.
	_, _ = hash.Write(protocolMD5Salt)
	_, _ = hash.Write([]byte(payload.FileID + fileSize + payload.SignKey + payload.SignValue + payload.UserID + timestamp))
	_, _ = hash.Write([]byte(hex.EncodeToString(userIDHash[:])))
	_, _ = hash.Write([]byte(payload.AppVersion))
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func isASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] > 0x7f {
			return false
		}
	}
	return true
}
