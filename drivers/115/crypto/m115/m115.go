// Package m115 实现 115 网盘 Web 端下载直链接口使用的自定义加解密。
// 参考 webapi/proapi 的 m115 算法：16 字节随机密钥 + XOR 变换 + RSA 分段加密。
package m115

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"math/big"
)

// Key 是单次请求使用的 16 字节随机密钥。
type Key [16]byte

// GenerateKey 生成随机密钥。
func GenerateKey() Key {
	key := Key{}
	_, _ = io.ReadFull(rand.Reader, key[:])
	return key
}

// Encode 把明文加密为 base64 字符串。
func Encode(input []byte, key Key) string {
	buf := make([]byte, 16+len(input))
	copy(buf, key[:])
	copy(buf[16:], input)
	xorTransform(buf[16:], xorDeriveKey(key[:], 4))
	reverseBytes(buf[16:])
	xorTransform(buf[16:], xorClientKey)
	return base64.StdEncoding.EncodeToString(rsaEncrypt(buf))
}

// Decode 解密 base64 密文，返回明文。
func Decode(input string, key Key) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, err
	}
	data = rsaDecrypt(data)
	if len(data) < 16 {
		return nil, err
	}
	output := make([]byte, len(data)-16)
	copy(output, data[16:])
	xorTransform(output, xorDeriveKey(data[:16], 12))
	reverseBytes(output)
	xorTransform(output, xorDeriveKey(key[:], 4))
	return output, nil
}

var (
	modulus, _ = big.NewInt(0).SetString(
		"8686980c0f5a24c4b9d43020cd2c22703ff3f450756529058b1cf88f09b86021"+
			"36477198a6e2683149659bd122c33592fdb5ad47944ad1ea4d36c6b172aad633"+
			"8c3bb6ac6227502d010993ac967d1aef00f0c8e038de2e4d3bc2ec368af2e9f1"+
			"0a6f1eda4f7262f136420c07c331b871bf139f74f3010e3c4fe57df3afb71683", 16)
	exponent, _ = big.NewInt(0).SetString("10001", 16)

	keyLength = modulus.BitLen() / 8
)

func rsaEncrypt(input []byte) []byte {
	buf := &bytes.Buffer{}
	for remainSize := len(input); remainSize > 0; {
		sliceSize := keyLength - 11
		if sliceSize > remainSize {
			sliceSize = remainSize
		}
		rsaEncryptSlice(input[:sliceSize], buf)

		input = input[sliceSize:]
		remainSize -= sliceSize
	}
	return buf.Bytes()
}

func rsaEncryptSlice(input []byte, w io.Writer) {
	padSize := keyLength - len(input) - 3
	padData := make([]byte, padSize)
	_, _ = rand.Read(padData)

	buf := make([]byte, keyLength)
	buf[0], buf[1] = 0, 2
	for i, b := range padData {
		buf[2+i] = b%0xff + 0x01
	}
	buf[padSize+2] = 0
	copy(buf[padSize+3:], input)

	msg := big.NewInt(0).SetBytes(buf)
	ret := big.NewInt(0).Exp(msg, exponent, modulus).Bytes()
	if fillSize := keyLength - len(ret); fillSize > 0 {
		_, _ = w.Write(make([]byte, fillSize))
	}
	_, _ = w.Write(ret)
}

func rsaDecrypt(input []byte) []byte {
	buf := &bytes.Buffer{}
	for remainSize := len(input); remainSize > 0; {
		sliceSize := keyLength
		if sliceSize > remainSize {
			sliceSize = remainSize
		}
		rsaDecryptSlice(input[:sliceSize], buf)

		input = input[sliceSize:]
		remainSize -= sliceSize
	}
	return buf.Bytes()
}

func rsaDecryptSlice(input []byte, w io.Writer) {
	msg := big.NewInt(0).SetBytes(input)
	ret := big.NewInt(0).Exp(msg, exponent, modulus).Bytes()
	for i, b := range ret {
		if b == 0 && i != 0 {
			_, _ = w.Write(ret[i+1:])
			break
		}
	}
}

var (
	xorKeySeed = []byte{
		0xf0, 0xe5, 0x69, 0xae, 0xbf, 0xdc, 0xbf, 0x8a,
		0x1a, 0x45, 0xe8, 0xbe, 0x7d, 0xa6, 0x73, 0xb8,
		0xde, 0x8f, 0xe7, 0xc4, 0x45, 0xda, 0x86, 0xc4,
		0x9b, 0x64, 0x8b, 0x14, 0x6a, 0xb4, 0xf1, 0xaa,
		0x38, 0x01, 0x35, 0x9e, 0x26, 0x69, 0x2c, 0x86,
		0x00, 0x6b, 0x4f, 0xa5, 0x36, 0x34, 0x62, 0xa6,
		0x2a, 0x96, 0x68, 0x18, 0xf2, 0x4a, 0xfd, 0xbd,
		0x6b, 0x97, 0x8f, 0x4d, 0x8f, 0x89, 0x13, 0xb7,
		0x6c, 0x8e, 0x93, 0xed, 0x0e, 0x0d, 0x48, 0x3e,
		0xd7, 0x2f, 0x88, 0xd8, 0xfe, 0xfe, 0x7e, 0x86,
		0x50, 0x95, 0x4f, 0xd1, 0xeb, 0x83, 0x26, 0x34,
		0xdb, 0x66, 0x7b, 0x9c, 0x7e, 0x9d, 0x7a, 0x81,
		0x32, 0xea, 0xb6, 0x33, 0xde, 0x3a, 0xa9, 0x59,
		0x34, 0x66, 0x3b, 0xaa, 0xba, 0x81, 0x60, 0x48,
		0xb9, 0xd5, 0x81, 0x9c, 0xf8, 0x6c, 0x84, 0x77,
		0xff, 0x54, 0x78, 0x26, 0x5f, 0xbe, 0xe8, 0x1e,
		0x36, 0x9f, 0x34, 0x80, 0x5c, 0x45, 0x2c, 0x9b,
		0x76, 0xd5, 0x1b, 0x8f, 0xcc, 0xc3, 0xb8, 0xf5,
	}

	xorClientKey = []byte{
		0x78, 0x06, 0xad, 0x4c, 0x33, 0x86, 0x5d, 0x18,
		0x4c, 0x01, 0x3f, 0x46,
	}
)

func xorDeriveKey(seed []byte, size int) []byte {
	key := make([]byte, size)
	for i := 0; i < size; i++ {
		key[i] = (seed[i] + xorKeySeed[size*i]) & 0xff
		key[i] ^= xorKeySeed[size*(size-i-1)]
	}
	return key
}

func xorTransform(data []byte, key []byte) {
	dataSize, keySize := len(data), len(key)
	mod := dataSize % 4
	if mod > 0 {
		for i := 0; i < mod; i++ {
			data[i] ^= key[i%keySize]
		}
	}
	for i := mod; i < dataSize; i++ {
		data[i] ^= key[(i-mod)%keySize]
	}
}

func reverseBytes(data []byte) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}
