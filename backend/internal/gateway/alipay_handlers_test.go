package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"live-auction/backend/internal/config"
)

func TestBuildAlipayPagePayFormOmitsCallbackURLsByDefault(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	form, gatewayURL, method, err := buildAlipayPagePayForm(config.Config{
		AlipayAppID:            "9021000164675433",
		AlipayGatewayURL:       "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
		AlipayNotifyURL:        "https://106.52.68.95:5276/api/payments/alipay/notify",
		AlipayReturnURL:        "https://106.52.68.95:5276/",
		AlipayPayMethod:        "alipay.trade.page.pay",
		AlipayProductCode:      "FAST_INSTANT_TRADE_PAY",
		AlipayIncludeNotifyURL: false,
		AlipayIncludeReturnURL: false,
	}, privateKey, alipayOrderForPayment{
		OrderID:     "ord_test",
		AmountCents: 12345,
		Title:       "测试订单",
	}, "alipay_ord_test")
	if err != nil {
		t.Fatalf("build form: %v", err)
	}
	if method != "GET" {
		t.Fatalf("method = %q, want GET", method)
	}
	if gatewayURL != "https://openapi-sandbox.dl.alipaydev.com/gateway.do" {
		t.Fatalf("gatewayURL = %q", gatewayURL)
	}
	if strings.Contains(form, `name="notify_url"`) {
		t.Fatalf("form unexpectedly contains notify_url: %s", form)
	}
	if strings.Contains(form, `name="return_url"`) {
		t.Fatalf("form unexpectedly contains return_url: %s", form)
	}
	if !strings.Contains(form, `action="https://openapi-sandbox.dl.alipaydev.com/gateway.do"`) {
		t.Fatalf("form action is not the HTTPS sandbox gateway: %s", form)
	}
	if !strings.Contains(form, `value="alipay.trade.page.pay"`) {
		t.Fatalf("form must contain configured page.pay method: %s", form)
	}
}

func TestBuildAlipayPagePayFormSupportsWapPay(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	form, _, method, err := buildAlipayPagePayForm(config.Config{
		AlipayAppID:       "9021000164675433",
		AlipayGatewayURL:  "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
		AlipayReturnURL:   "https://106.52.68.95:5276/",
		AlipayPayMethod:   "alipay.trade.wap.pay",
		AlipayProductCode: "QUICK_WAP_WAY",
	}, privateKey, alipayOrderForPayment{
		OrderID:     "ord_test",
		AmountCents: 12345,
		Title:       "测试订单",
	}, "alipay_ord_test")
	if err != nil {
		t.Fatalf("build form: %v", err)
	}
	if method != "POST" {
		t.Fatalf("method = %q, want POST", method)
	}
	if !strings.Contains(form, `value="alipay.trade.wap.pay"`) {
		t.Fatalf("form must contain configured wap.pay method: %s", form)
	}
	if !strings.Contains(form, `QUICK_WAP_WAY`) {
		t.Fatalf("form must contain configured wap product code: %s", form)
	}
	if !strings.Contains(form, `name="return_url"`) {
		t.Fatalf("wap form must contain return_url: %s", form)
	}
	if !strings.Contains(form, `quit_url`) {
		t.Fatalf("wap biz_content must contain quit_url: %s", form)
	}
	if !strings.Contains(form, `action="https://openapi-sandbox.dl.alipaydev.com/gateway.do?charset=utf-8"`) {
		t.Fatalf("wap post form action must include charset query string: %s", form)
	}
}

func TestBuildAlipayPagePayFormIncludesNotifyURLWhenExplicitlyEnabled(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	form, _, _, err := buildAlipayPagePayForm(config.Config{
		AlipayAppID:            "9021000164675433",
		AlipayGatewayURL:       "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
		AlipayNotifyURL:        "https://106.52.68.95:5276/api/payments/alipay/notify",
		AlipayReturnURL:        "https://106.52.68.95:5276/",
		AlipayProductCode:      "FAST_INSTANT_TRADE_PAY",
		AlipayIncludeNotifyURL: true,
		AlipayIncludeReturnURL: true,
	}, privateKey, alipayOrderForPayment{
		OrderID:     "ord_test",
		AmountCents: 12345,
		Title:       "测试订单",
	}, "alipay_ord_test")
	if err != nil {
		t.Fatalf("build form: %v", err)
	}
	if !strings.Contains(form, `name="notify_url"`) {
		t.Fatalf("form must contain notify_url when explicitly enabled: %s", form)
	}
	if !strings.Contains(form, `name="return_url"`) {
		t.Fatalf("form must contain return_url when explicitly enabled: %s", form)
	}
}
