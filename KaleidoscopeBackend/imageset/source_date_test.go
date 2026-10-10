package imageset

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var jst = time.FixedZone("JST", 9*60*60)

func TestDayOnlyDate(t *testing.T) {
	// 23:30 at UTC-5 is already the 28th in UTC.
	got := DayOnlyDate(time.Date(2026, 8, 27, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)))
	if want := time.Date(2026, 8, 28, 0, 17, 0, 17_000_000, time.UTC); !got.Equal(want) {
		t.Errorf("DayOnlyDate = %v, want %v", got, want)
	}
	if !isDayOnly(got) {
		t.Error("a DayOnlyDate result is not day-only")
	}
	for _, exact := range []time.Time{
		time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 27, 18, 12, 12, 0, jst),
	} {
		if isDayOnly(exact) {
			t.Errorf("exact time %v counted as day-only", exact)
		}
	}

	raw, err := bson.Marshal(struct{ D time.Time }{got})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct{ D time.Time }
	if err := bson.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !isDayOnly(decoded.D) {
		t.Errorf("marker lost in a BSON round trip: %v", decoded.D)
	}
}

func TestSourceDateMatches(t *testing.T) {
	dayOnly := DayOnlyDate(time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	exact := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	tests := []struct {
		name            string
		stored, service time.Time
		want            bool
	}{
		{"day-only, same UTC day", dayOnly, time.Date(2026, 8, 28, 5, 0, 0, 0, jst), true},
		{"day-only, JST morning is the previous UTC day", dayOnly, time.Date(2026, 8, 27, 8, 0, 0, 0, jst), false},
		{"exact, same instant in another zone", exact, exact.In(jst), true},
		{"exact, one second later", exact, exact.Add(time.Second), false},
	}
	for _, tt := range tests {
		if got := SourceDateMatches(tt.stored, tt.service); got != tt.want {
			t.Errorf("%s: SourceDateMatches = %t, want %t", tt.name, got, tt.want)
		}
	}
}

func TestMergeSourceDate(t *testing.T) {
	exact := time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
	sameDay := DayOnlyDate(exact)
	otherDay := DayOnlyDate(exact.AddDate(0, 0, 1))
	tests := []struct {
		name                   string
		stored, incoming, want time.Time
	}{
		{"empty incoming keeps stored", exact, time.Time{}, exact},
		{"empty stored takes incoming", time.Time{}, sameDay, sameDay},
		{"same instant keeps stored zone", exact, exact.In(jst), exact},
		{"day-only never replaces exact on the same day", exact, sameDay, exact},
		{"exact replaces day-only", sameDay, exact, exact},
		{"day-only on another day wins", exact, otherDay, otherDay},
		{"later exact wins", exact, exact.Add(time.Hour), exact.Add(time.Hour)},
	}
	for _, tt := range tests {
		got := mergeSourceDate(tt.stored, tt.incoming)
		if !got.Equal(tt.want) || got.Location() != tt.want.Location() {
			t.Errorf("%s: mergeSourceDate = %v, want %v", tt.name, got, tt.want)
		}
	}
}
