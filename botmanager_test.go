package discord_bridge

import "testing"

func TestNewBotManager(t *testing.T) {

	_, err := NewBotManager("")

	if err == nil {
		t.Errorf("Empty token should generate error")
	}

	bm, err := NewBotManager("asdf")

	if err != nil {
		t.Errorf("Valid data should not generate errors. Got: " + err.Error())
	}

	if bm == nil {
		t.Errorf("Valid data should not generate errors")
	}
}
