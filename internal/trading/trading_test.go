package trading

import (
	"encoding/json"
	"testing"

	"github.com/asf-go/asf/internal/steam"
)

func TestGetTradeOffersResponseParse(t *testing.T) {
	data := `{
		"response": {
			"trade_offers_received": [
				{
					"tradeofferid": "123456789",
					"accountid_other": 100200300,
					"message": "hello",
					"trade_offer_state": 2,
					"is_our_offer": false,
					"time_created": 1700000000,
					"time_updated": 1700000100,
					"items_to_receive": [
						{"appid": 753, "contextid": "6", "assetid": "111", "classid": "222", "instanceid": "0", "amount": 1, "missing": false}
					],
					"items_to_give": []
				}
			],
			"trade_offers_sent": []
		}
	}`
	var out GetTradeOffersResponse
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	offers := out.Response.TradeOffersReceived
	if len(offers) != 1 {
		t.Fatalf("期望 1 条报价，得到 %d", len(offers))
	}
	o := offers[0]
	if o.TradeOfferID != 123456789 {
		t.Errorf("TradeOfferID 不一致: %d", o.TradeOfferID)
	}
	if o.State != TradeOfferStateActive {
		t.Errorf("State 应为 Active(2)，得到 %d", o.State)
	}
	if o.AccountIDOther != 100200300 {
		t.Errorf("AccountIDOther 不一致: %d", o.AccountIDOther)
	}
	if len(o.ItemsToReceive) != 1 {
		t.Fatalf("ItemsToReceive 应为 1，得到 %d", len(o.ItemsToReceive))
	}
	if o.ItemsToReceive[0].AssetID != 111 {
		t.Errorf("AssetID 不一致: %d", o.ItemsToReceive[0].AssetID)
	}
}

func TestTradeOfferStateConstants(t *testing.T) {
	cases := map[TradeOfferState]int{
		TradeOfferStateInvalid:                  1,
		TradeOfferStateActive:                   2,
		TradeOfferStateAccepted:                 3,
		TradeOfferStateCountered:                4,
		TradeOfferStateExpired:                  5,
		TradeOfferStateCanceled:                 6,
		TradeOfferStateDeclined:                 7,
		TradeOfferStateInvalidItems:             8,
		TradeOfferStateCreatedNeedsConfirmation: 9,
		TradeOfferStateCanceledBySecondFactor:   10,
		TradeOfferStateInEscrow:                 11,
	}
	for state, want := range cases {
		if int(state) != want {
			t.Errorf("状态 %d != %d", state, want)
		}
	}
}

func TestGetTradeOffersNoAPIKey(t *testing.T) {
	if _, err := GetTradeOffers("", true); err == nil {
		t.Error("未配置 API Key 时应返回错误")
	}
}

func TestAcceptTradeOfferNilSession(t *testing.T) {
	if err := AcceptTradeOffer(nil, 123); err == nil {
		t.Error("Web 会话为空时应返回错误")
	}
}

func TestSendTradeOfferValidation(t *testing.T) {
	if _, err := SendTradeOffer(nil, "key", 76561198000000000, []TradeItem{{AppID: 753, AssetID: 1}}, ""); err == nil {
		t.Error("Web 会话为空时应返回错误")
	}
	if _, err := SendTradeOffer(&steam.WebSession{}, "", 76561198000000000, []TradeItem{{AppID: 753, AssetID: 1}}, ""); err == nil {
		t.Error("API Key 为空时应返回错误")
	}
	if _, err := SendTradeOffer(&steam.WebSession{}, "key", 76561198000000000, nil, ""); err == nil {
		t.Error("物品列表为空时应返回错误")
	}
	if _, err := SendTradeOffer(&steam.WebSession{}, "key", 76561198000000000, []TradeItem{{AppID: 753, AssetID: 0}}, ""); err == nil {
		t.Error("AssetID 为 0 时应返回错误")
	}
}
