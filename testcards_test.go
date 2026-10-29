package tap

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTestCardsIncludeConstants(t *testing.T) {
	numbers := map[string]bool{}
	for _, card := range TestCards {
		if card.Number == "" || card.Method == "" {
			t.Errorf("incomplete card %+v", card)
		}
		numbers[card.Number] = true
	}
	for _, n := range []string{
		TestCardMastercard3DS, TestCardMastercard, TestCardVisa3DS, TestCardVisa,
		TestCardAmex3DS, TestCardAmex, TestCardMada3DS, TestCardMada,
		TestCardOmanNet3DS, TestCardKNETCaptured, TestCardBenefitCaptured,
		TestCardBenefitDeclined, TestCardQPayCaptured, TestCardJaywanCaptured,
		TestCardJaywanDeclined,
	} {
		if !numbers[n] {
			t.Errorf("constant %s missing from TestCards", n)
		}
	}
}

func TestFlexNumber(t *testing.T) {
	var v struct {
		A, B, C FlexNumber
	}
	if err := json.Unmarshal([]byte(`{"A": 1698392202943, "B": "1698392202943", "C": null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != v.B || v.C != "" {
		t.Errorf("got %+v", v)
	}
	ts, err := v.B.Time()
	if err != nil || !ts.Equal(time.UnixMilli(1698392202943)) {
		t.Errorf("Time() = %v, %v", ts, err)
	}
	out, err := json.Marshal(v)
	if err != nil ||
		string(out) != `{"A":1698392202943,"B":1698392202943,"C":null}` {
		t.Errorf("Marshal = %s, %v", out, err)
	}
	if err := json.Unmarshal([]byte(`{"A": true}`), &v); err == nil {
		t.Error("bool accepted as FlexNumber")
	}
}
