package ipc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// encryptionKey 与原版 ArchiSteamFarm 默认 EncryptionKey 保持一致。
// 原版可通过命令修改该密钥，本实现固定使用默认值以保证 ASF-ui 兼容。
var encryptionKey = []byte("ArchiSteamFarm")

// encryptAES 对应原版 ArchiCryptoHelper.Encrypt(ECryptoMethod.AES, text)：
//   - 密钥 = SHA256(EncryptionKey)
//   - 生成随机 16 字节 IV
//   - 输出 = Base64(AES-256-ECB(IV) || AES-256-CBC(PKCS7, IV=原始IV)(明文))
//
// 该格式可被原版 ASF 解密，用于 ASF-ui 的密码加密存储。
func encryptAES(text string) (string, error) {
	key := sha256.Sum256(encryptionKey)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	// ECB 加密 IV（无填充，块大小 16 字节）
	encryptedIV := make([]byte, aes.BlockSize)
	block.Encrypt(encryptedIV, iv)

	// CBC 加密明文（PKCS7 填充）
	plain := pkcs7Pad([]byte(text), aes.BlockSize)
	encryptedText := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encryptedText, plain)

	out := make([]byte, 0, len(encryptedIV)+len(encryptedText))
	out = append(out, encryptedIV...)
	out = append(out, encryptedText...)

	return base64.StdEncoding.EncodeToString(out), nil
}

// pkcs7Pad 按 blockSize 对 data 进行 PKCS7 填充。
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

// hashSCrypt 对应原版 Hash(EHashingMethod.SCrypt, text)：
// SCrypt(N=8192, r=8, p=1)，salt=EncryptionKey，输出 32 字节，Base64 编码。
func hashSCrypt(text string) (string, error) {
	h, err := scrypt.Key([]byte(text), encryptionKey, 8192, 8, 1, 32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h), nil
}

// hashPBKDF2 对应原版 Hash(EHashingMethod.Pbkdf2, text)：
// PBKDF2-HMAC-SHA256，迭代 10000 次，salt=EncryptionKey，输出 32 字节，Base64 编码。
func hashPBKDF2(text string) string {
	h := pbkdf2.Key([]byte(text), encryptionKey, 10000, 32, sha256.New)
	return base64.StdEncoding.EncodeToString(h)
}
