package ipc

import "testing"

func TestVerifyPasswordPlain(t *testing.T) {
	s := &Server{password: "secret", passwordFormat: "PlainText"}
	if !s.verifyPassword("secret") {
		t.Error("正确明文密码应通过")
	}
	if s.verifyPassword("wrong") {
		t.Error("错误密码不应通过")
	}
}

func TestVerifyPasswordPbkdf2(t *testing.T) {
	hash := hashPBKDF2("secret")
	s := &Server{password: hash, passwordFormat: "Pbkdf2"}
	if !s.verifyPassword("secret") {
		t.Error("正确密码经 PBKDF2 哈希后应通过")
	}
	if s.verifyPassword("wrong") {
		t.Error("错误密码不应通过")
	}
}

func TestVerifyPasswordSCrypt(t *testing.T) {
	hash, err := hashSCrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{password: hash, passwordFormat: "SCrypt"}
	if !s.verifyPassword("secret") {
		t.Error("正确密码经 SCrypt 哈希后应通过")
	}
	if s.verifyPassword("wrong") {
		t.Error("错误密码不应通过")
	}
}
