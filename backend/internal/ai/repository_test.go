package ai

import "testing"

func TestNormalizeCommentaryMoneyCorrectsCentsAsYuan(t *testing.T) {
	body, changed := normalizeCommentaryMoney("当前出价40000元，用户暂时领先。", 40000)
	if !changed {
		t.Fatalf("expected normalization")
	}
	if body != "当前出价¥400.00，用户暂时领先。" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestNormalizeCommentaryMoneyCorrectsWrongWanUnit(t *testing.T) {
	body, changed := normalizeCommentaryMoney("现在这件拍品的出价是4万元。", 40000)
	if !changed {
		t.Fatalf("expected normalization")
	}
	if body != "现在这件拍品的出价是¥400.00。" {
		t.Fatalf("unexpected body: %s", body)
	}
}
