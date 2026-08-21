package p115cipher

import (
	"encoding/base64"
	"errors"
	"math/big"
)

const (
	rsaBlockSize     = 128
	rsaPlainBlockMax = rsaBlockSize - 11
)

var (
	errInvalidRSAInput = errors.New("p115 RSA input invalid")
	rsaExponent        = big.NewInt(0x10001)
	rsaModulus         = mustBigInt("8686980c0f5a24c4b9d43020cd2c22703ff3f450756529058b1cf88f09b86021" +
		"36477198a6e2683149659bd122c33592fdb5ad47944ad1ea4d36c6b172aad633" +
		"8c3bb6ac6227502d010993ac967d1aef00f0c8e038de2e4d3bc2ec368af2e9f1" +
		"0a6f1eda4f7262f136420c07c331b871bf139f74f3010e3c4fe57df3afb71683")
	rsaDefaultKey = []byte{0x8d, 0xa5, 0xa5, 0x8d}
	rsaLongKey    = []byte{0x78, 0x06, 0xad, 0x4c, 0x33, 0x86, 0x5d, 0x18, 0x4c, 0x01, 0x3f, 0x46}
	rsaKeyTable   = []byte{
		0xf0, 0xe5, 0x69, 0xae, 0xbf, 0xdc, 0xbf, 0x8a, 0x1a, 0x45, 0xe8, 0xbe, 0x7d, 0xa6, 0x73, 0xb8,
		0xde, 0x8f, 0xe7, 0xc4, 0x45, 0xda, 0x86, 0xc4, 0x9b, 0x64, 0x8b, 0x14, 0x6a, 0xb4, 0xf1, 0xaa,
		0x38, 0x01, 0x35, 0x9e, 0x26, 0x69, 0x2c, 0x86, 0x00, 0x6b, 0x4f, 0xa5, 0x36, 0x34, 0x62, 0xa6,
		0x2a, 0x96, 0x68, 0x18, 0xf2, 0x4a, 0xfd, 0xbd, 0x6b, 0x97, 0x8f, 0x4d, 0x8f, 0x89, 0x13, 0xb7,
		0x6c, 0x8e, 0x93, 0xed, 0x0e, 0x0d, 0x48, 0x3e, 0xd7, 0x2f, 0x88, 0xd8, 0xfe, 0xfe, 0x7e, 0x86,
		0x50, 0x95, 0x4f, 0xd1, 0xeb, 0x83, 0x26, 0x34, 0xdb, 0x66, 0x7b, 0x9c, 0x7e, 0x9d, 0x7a, 0x81,
		0x32, 0xea, 0xb6, 0x33, 0xde, 0x3a, 0xa9, 0x59, 0x34, 0x66, 0x3b, 0xaa, 0xba, 0x81, 0x60, 0x48,
		0xb9, 0xd5, 0x81, 0x9c, 0xf8, 0x6c, 0x84, 0x77, 0xff, 0x54, 0x78, 0x26, 0x5f, 0xbe, 0xe8, 0x1e,
		0x36, 0x9f, 0x34, 0x80, 0x5c, 0x45, 0x2c, 0x9b, 0x76, 0xd5, 0x1b, 0x8f, 0xcc, 0xc3, 0xb8, 0xf5,
	}
)

// RSAEncrypt applies the pinned 115 request transform and returns base64 ciphertext.
func RSAEncrypt(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", errInvalidRSAInput
	}
	transformed := xorProtocol(plaintext, rsaDefaultKey)
	reverseBytes(transformed)
	transformed = xorProtocol(transformed, rsaLongKey)
	payload := make([]byte, 16+len(transformed))
	copy(payload[16:], transformed)

	ciphertext := make([]byte, 0, ((len(payload)+rsaPlainBlockMax-1)/rsaPlainBlockMax)*rsaBlockSize)
	for len(payload) > 0 {
		blockSize := rsaPlainBlockMax
		if len(payload) < blockSize {
			blockSize = len(payload)
		}
		ciphertext = append(ciphertext, rsaEncryptBlock(payload[:blockSize])...)
		payload = payload[blockSize:]
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// RSADecrypt applies the pinned 115 response transform to base64 ciphertext.
func RSADecrypt(ciphertext string) ([]byte, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(ciphertext)
	if err != nil || len(raw) == 0 || len(raw)%rsaBlockSize != 0 {
		return nil, errInvalidRSAInput
	}
	decoded := make([]byte, 0, len(raw))
	for len(raw) > 0 {
		block, err := rsaDecryptBlock(raw[:rsaBlockSize])
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, block...)
		raw = raw[rsaBlockSize:]
	}
	if len(decoded) <= 16 {
		return nil, errInvalidRSAInput
	}
	longKey, err := rsaDeriveKey(decoded[:16], 12)
	if err != nil {
		return nil, err
	}
	plaintext := xorProtocol(decoded[16:], longKey)
	reverseBytes(plaintext)
	return xorProtocol(plaintext, rsaDefaultKey), nil
}

func rsaEncryptBlock(plaintext []byte) []byte {
	padded := make([]byte, rsaBlockSize)
	padded[1] = 0x02
	separator := rsaBlockSize - len(plaintext) - 1
	for index := 2; index < separator; index++ {
		padded[index] = 0x02
	}
	copy(padded[separator+1:], plaintext)
	value := new(big.Int).SetBytes(padded)
	transformed := new(big.Int).Exp(value, rsaExponent, rsaModulus).Bytes()
	block := make([]byte, rsaBlockSize)
	copy(block[rsaBlockSize-len(transformed):], transformed)
	return block
}

func rsaDecryptBlock(ciphertext []byte) ([]byte, error) {
	value := new(big.Int).SetBytes(ciphertext)
	transformed := new(big.Int).Exp(value, rsaExponent, rsaModulus).Bytes()
	for index, current := range transformed {
		if current == 0 {
			return append([]byte(nil), transformed[index+1:]...), nil
		}
	}
	return nil, errInvalidRSAInput
}

func rsaDeriveKey(randomKey []byte, size int) ([]byte, error) {
	if size <= 0 || len(randomKey) < size || size*size > len(rsaKeyTable)+size {
		return nil, errInvalidRSAInput
	}
	key := make([]byte, size)
	left := size * (size - 1)
	right := 0
	for index := range size {
		key[index] = rsaKeyTable[left] ^ (randomKey[index] + rsaKeyTable[right])
		left -= size
		right += size
	}
	return key, nil
}

func xorProtocol(source, key []byte) []byte {
	output := make([]byte, len(source))
	prefix := len(source) & 0b11
	for index := 0; index < prefix; index++ {
		output[index] = source[index] ^ key[index]
	}
	for offset := prefix; offset < len(source); offset += len(key) {
		blockSize := len(key)
		if remaining := len(source) - offset; remaining < blockSize {
			blockSize = remaining
		}
		for index := 0; index < blockSize; index++ {
			output[offset+index] = source[offset+index] ^ key[index]
		}
	}
	return output
}

func reverseBytes(value []byte) {
	for left, right := 0, len(value)-1; left < right; left, right = left+1, right-1 {
		value[left], value[right] = value[right], value[left]
	}
}

func mustBigInt(value string) *big.Int {
	parsed, ok := new(big.Int).SetString(value, 16)
	if !ok {
		panic("invalid fixed p115 RSA modulus")
	}
	return parsed
}
