package ipc

import (
	asfcrypto "github.com/asf-go/asf/internal/crypto"
)

// 以下函数包装 internal/crypto 的加解密/哈希实现，供 IPC 端点使用。
// 具体算法说明见 internal/crypto/crypto.go。

func encryptAES(text string) (string, error) {
	return asfcrypto.EncryptAES(text)
}

func decryptAES(cipherText string) (string, error) {
	return asfcrypto.DecryptAES(cipherText)
}

func hashSCrypt(text string) (string, error) {
	return asfcrypto.HashSCrypt(text)
}

func hashPBKDF2(text string) string {
	return asfcrypto.HashPBKDF2(text)
}