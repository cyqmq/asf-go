package store

import (
	"testing"

	"github.com/asf-go/asf/internal/steam"
)

func TestAddFreeLicenseValidation(t *testing.T) {
	if _, err := AddFreeLicense(nil, 123); err == nil {
		t.Error("Web 会话为空时应返回错误")
	}
	if _, err := AddFreeLicense(&steam.WebSession{}, 0); err == nil {
		t.Error("subID 为 0 时应返回错误")
	}
}

func TestAddFreeLicenseResultString(t *testing.T) {
	r := AddFreeLicenseResult{Result: EResultOK, PurchaseResultDetail: EPurchaseResultDetailAlreadyPurchased}
	if s := r.String(); s != "1/9" {
		t.Errorf("String() = %q, 期望 %q", s, "1/9")
	}
}

func TestRedeemKeyValidation(t *testing.T) {
	if _, err := RedeemKey(nil, "AAAAA-BBBBB-CCCCC"); err == nil {
		t.Error("Web 会话为空时应返回错误")
	}
	if _, err := RedeemKey(&steam.WebSession{}, ""); err == nil {
		t.Error("激活码为空时应返回错误")
	}
	if _, err := RedeemKey(&steam.WebSession{}, "   "); err == nil {
		t.Error("全空白激活码时应返回错误")
	}
}