package items

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func TestMoneyUsesMinorUnitsAndPreservesJSONPrice(t *testing.T) {
	money, err := ParseMoney("12.50")
	if err != nil {
		t.Fatal(err)
	}
	if money.MinorUnits() != 1250 {
		t.Fatalf("expected 1250 minor units, got %d", money.MinorUnits())
	}
	encoded, err := json.Marshal(money)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "12.50" {
		t.Fatalf("expected decimal JSON price, got %s", encoded)
	}
}

func TestItemOwnsImages(t *testing.T) {
	item := Item{ID: 7}
	if err := item.AddImage(Image{ID: 1, ItemID: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := item.RemoveImage(1); err != nil {
		t.Fatal(err)
	}
	if _, err := item.RemoveImage(1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing image error, got %v", err)
	}
}
