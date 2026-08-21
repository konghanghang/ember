// Package p115cipher implements Ember-owned compatibility primitives for the
// pinned 115 Cookie upload protocol. It does not depend on or embed the
// upstream Python implementation.
package p115cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"

	"github.com/pierrec/lz4/v4"
)

const (
	tokenRawSize            = 48
	tokenPublicKeySize      = 30
	lz4BlockOutputSize      = 0x2000
	maxDecompressedResponse = 8 * 1024 * 1024
)

var (
	errInvalidTimestamp  = errors.New("p115 cipher timestamp out of range")
	errInvalidToken      = errors.New("p115 cipher token invalid")
	errInvalidCiphertext = errors.New("p115 cipher ciphertext invalid")
	errInvalidLZ4Frame   = errors.New("p115 cipher LZ4 response invalid")

	protocolAESKey    = [16]byte{0xfb, 0x1a, 0x19, 0xd6, 0x52, 0xf5, 0xaa, 0xf7, 0xbc, 0x65, 0x1d, 0x0f, 0x69, 0xbf, 0x42, 0x2f}
	protocolAESIV     = [16]byte{0x69, 0xbf, 0x42, 0x2f, 0x49, 0x96, 0x05, 0x50, 0xa0, 0xad, 0x44, 0xec, 0x34, 0x46, 0xcb, 0x4c}
	protocolPublicKey = [tokenPublicKeySize]byte{
		0x1d, 0x03, 0x0e, 0x80, 0xa1, 0x78, 0xdc, 0xee, 0xce, 0xcd,
		0xa3, 0x77, 0xde, 0x12, 0x8d, 0x8e, 0xd9, 0xdd, 0xcf, 0x55,
		0xae, 0x61, 0xed, 0x46, 0xea, 0x12, 0x1a, 0x1c, 0xfc, 0x81,
	}
	protocolCRCSalt = []byte("^j>WD3Kr?J2gLFjD4W2y@")
)

// DecodedToken exposes only the timestamp and public material carried by k_ec.
type DecodedToken struct {
	Timestamp int64
	PublicKey [tokenPublicKeySize]byte
}

// EncodeToken creates the deterministic k_ec token used by upload initialization.
func EncodeToken(timestamp int64) (string, error) {
	if timestamp < 0 || timestamp > math.MaxUint32 {
		return "", errInvalidTimestamp
	}

	raw := make([]byte, tokenRawSize)
	copy(raw[:15], protocolPublicKey[:15])
	copy(raw[15:20], []byte{0x00, 0x73, 0x00, 0x00, 0x00})
	binary.LittleEndian.PutUint32(raw[20:24], uint32(timestamp))
	copy(raw[24:39], protocolPublicKey[15:])
	copy(raw[39:44], []byte{0x00, 0x01, 0x00, 0x00, 0x00})
	binary.LittleEndian.PutUint32(raw[44:48], tokenChecksum(raw[:44]))
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DecodeToken validates the token CRC before returning its timestamp and public material.
func DecodeToken(token string) (DecodedToken, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != tokenRawSize {
		return DecodedToken{}, errInvalidToken
	}
	wantCRC := binary.LittleEndian.Uint32(raw[44:48])
	if tokenChecksum(raw[:44]) != wantCRC {
		return DecodedToken{}, errInvalidToken
	}

	firstMask := raw[15]
	secondMask := raw[39]
	var decoded DecodedToken
	for index := range 15 {
		decoded.PublicKey[index] = raw[index] ^ firstMask
		decoded.PublicKey[index+15] = raw[index+24] ^ secondMask
	}
	timestampBytes := [4]byte{}
	for index := range timestampBytes {
		timestampBytes[index] = raw[index+20] ^ firstMask
	}
	decoded.Timestamp = int64(binary.LittleEndian.Uint32(timestampBytes[:]))
	return decoded, nil
}

// EncryptRequest AES-CBC encrypts an upload form using the pinned protocol key and IV.
func EncryptRequest(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(protocolAESKey[:])
	if err != nil {
		return nil, fmt.Errorf("create p115 AES cipher: %w", err)
	}
	padded := protocolPad(plaintext, block.BlockSize())
	if len(padded) == 0 {
		return []byte{}, nil
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, protocolAESIV[:]).CryptBlocks(ciphertext, padded)
	return ciphertext, nil
}

// DecryptResponse decrypts AES-CBC data and expands the protocol's length-prefixed LZ4 blocks.
func DecryptResponse(ciphertext []byte) ([]byte, error) {
	framed, err := decryptRequest(ciphertext)
	if err != nil {
		return nil, err
	}
	return decompressLZ4Frames(framed)
}

func decryptRequest(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(protocolAESKey[:])
	if err != nil {
		return nil, fmt.Errorf("create p115 AES cipher: %w", err)
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errInvalidCiphertext
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, protocolAESIV[:]).CryptBlocks(plaintext, ciphertext)
	return protocolUnpad(plaintext, block.BlockSize()), nil
}

func protocolPad(plaintext []byte, blockSize int) []byte {
	paddingSize := -len(plaintext) & (blockSize - 1)
	if paddingSize == 0 {
		return append([]byte(nil), plaintext...)
	}
	padded := make([]byte, len(plaintext)+paddingSize)
	copy(padded, plaintext)
	for index := len(plaintext); index < len(padded); index++ {
		padded[index] = byte(paddingSize)
	}
	return padded
}

func protocolUnpad(plaintext []byte, blockSize int) []byte {
	if len(plaintext) == 0 {
		return plaintext
	}
	paddingSize := int(plaintext[len(plaintext)-1])
	// The pinned protocol does not add or remove a full block of padding.
	if paddingSize <= 0 || paddingSize >= blockSize || paddingSize > len(plaintext) {
		return plaintext
	}
	for _, value := range plaintext[len(plaintext)-paddingSize:] {
		if int(value) != paddingSize {
			return plaintext
		}
	}
	return plaintext[:len(plaintext)-paddingSize]
}

func decompressLZ4Frames(framed []byte) ([]byte, error) {
	output := make([]byte, 0, len(framed))
	for len(framed) > 0 {
		if len(framed) < 2 {
			return nil, errInvalidLZ4Frame
		}
		compressedSize := int(binary.LittleEndian.Uint16(framed[:2]))
		framed = framed[2:]
		if compressedSize == 0 {
			if len(framed) != 0 {
				return nil, errInvalidLZ4Frame
			}
			break
		}
		if compressedSize > len(framed) {
			return nil, errInvalidLZ4Frame
		}
		blockOutput := make([]byte, lz4BlockOutputSize)
		decodedSize, err := lz4.UncompressBlock(framed[:compressedSize], blockOutput)
		if err != nil || decodedSize <= 0 {
			return nil, errInvalidLZ4Frame
		}
		if len(output)+decodedSize > maxDecompressedResponse {
			return nil, errInvalidLZ4Frame
		}
		output = append(output, blockOutput[:decodedSize]...)
		framed = framed[compressedSize:]
	}
	return output, nil
}

func tokenChecksum(payload []byte) uint32 {
	hash := crc32.NewIEEE()
	_, _ = hash.Write(protocolCRCSalt)
	_, _ = hash.Write(payload)
	return hash.Sum32()
}
