// Package crypto 提供与原版 ArchiSteamFarm 兼容的加密/哈希原语，
// 供 IPC（ASF-ui 密码加密）与 Bot（登录 Key/密码加密存储）共用。
// 密钥可通过 ASF_CRYPTKEY 环境变量覆盖（32 字节 hex），默认与原版一致。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// EnvironmentVariableCryptKey 对应原版 SharedInfo.EnvironmentVariableCryptKey。
const EnvironmentVariableCryptKey = "ASF_CRYPTKEY"

// defaultKey 与原版 ArchiSteamFarm 默认 EncryptionKey 一致。
var defaultKey = []byte("ArchiSteamFarm")

var (
	keyMu         sync.RWMutex
	encryptionKey []byte
)

// LoadEncryptionKey 从 ASF_CRYPTKEY 环境变量加载密钥（hex，至少 32 字节）；未设置时使用默认密钥。
func LoadEncryptionKey() {
	k := defaultKey
	if v := os.Getenv(EnvironmentVariableCryptKey); v != "" {
		if key, err := hex.DecodeString(v); err == nil && len(key) >= 32 {
			k = key
		}
	}
	keyMu.Lock()
	encryptionKey = k
	keyMu.Unlock()
}

// EncryptionEnabled 返回是否使用了非默认密钥（即设置了有效的 ASF_CRYPTKEY）。
func EncryptionEnabled() bool {
	LoadEncryptionKey()
	keyMu.RLock()
	defer keyMu.RUnlock()
	return !stringEqual(encryptionKey, defaultKey)
}

// key 返回当前加密密钥。
func key() []byte {
	LoadEncryptionKey()
	keyMu.RLock()
	defer keyMu.RUnlock()
	return encryptionKey
}

func stringEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// EncryptAES 对应原版 ArchiCryptoHelper.Encrypt(ECryptoMethod.AES, text)：
//   - 密钥 = SHA256(EncryptionKey)
//   - 生成随机 16 字节 IV
//   - 输出 = Base64(AES-256-ECB(IV) || AES-256-CBC(PKCS7, IV=原始IV)(明文))
//
// 该格式可被原版 ASF 解密。
func EncryptAES(text string) (string, error) {
	k := sha256.Sum256(key())

	block, err := aes.NewCipher(k[:])
	if err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	encryptedIV := make([]byte, aes.BlockSize)
	block.Encrypt(encryptedIV, iv)

	plain := pkcs7Pad([]byte(text), aes.BlockSize)
	encryptedText := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encryptedText, plain)

	out := make([]byte, 0, len(encryptedIV)+len(encryptedText))
	out = append(out, encryptedIV...)
	out = append(out, encryptedText...)

	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptAES 解密 EncryptAES 的输出。
func DecryptAES(cipherText string) (string, error) {
	k := sha256.Sum256(key())

	block, err := aes.NewCipher(k[:])
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	if len(data) < 2*aes.BlockSize {
		return "", errors.New("密文长度无效")
	}

	encryptedIV := data[:aes.BlockSize]
	iv := make([]byte, aes.BlockSize)
	block.Decrypt(iv, encryptedIV)

	encryptedText := data[aes.BlockSize:]
	if len(encryptedText)%aes.BlockSize != 0 {
		return "", errors.New("密文长度不是块大小整数倍")
	}

	plain := make([]byte, len(encryptedText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, encryptedText)

	unpadded, err := pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(unpadded), nil
}

// EncryptLoginKey 加密登录 Key；未启用 ASF_CRYPTKEY 时返回明文（与原版行为一致）。
func EncryptLoginKey(loginKey string) string {
	if !EncryptionEnabled() {
		return loginKey
	}
	encrypted, err := EncryptAES(loginKey)
	if err != nil {
		return loginKey
	}
	return "encrypted:" + encrypted
}

// DecryptLoginKey 解密 EncryptLoginKey 的结果；非加密值原样返回。
func DecryptLoginKey(value string) string {
	if !strings.HasPrefix(value, "encrypted:") {
		return value
	}
	decrypted, err := DecryptAES(strings.TrimPrefix(value, "encrypted:"))
	if err != nil {
		return value
	}
	return decrypted
}

// HashSCrypt 对应原版 Hash(EHashingMethod.SCrypt, text)：
// SCrypt(N=8192, r=8, p=1)，salt=EncryptionKey，输出 32 字节，Base64 编码。
func HashSCrypt(text string) (string, error) {
	h, err := scrypt.Key([]byte(text), key(), 8192, 8, 1, 32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h), nil
}

// HashPBKDF2 对应原版 Hash(EHashingMethod.Pbkdf2, text)：
// PBKDF2-HMAC-SHA256，迭代 10000 次，salt=EncryptionKey，输出 32 字节，Base64 编码。
func HashPBKDF2(text string) string {
	h := pbkdf2.Key([]byte(text), key(), 10000, 32, sha256.New)
	return base64.StdEncoding.EncodeToString(h)
}

// pkcs7Pad 按 blockSize 对 data 进行 PKCS7 填充。
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padLen)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}

// pkcs7Unpad 去除 PKCS7 填充。
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("填充数据长度无效")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("无效 PKCS7 填充")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if int(data[i]) != padLen {
			return nil, errors.New("无效 PKCS7 填充字节")
		}
	}
	return data[:len(data)-padLen], nil
}