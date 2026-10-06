package tagging

import (
	"errors"
	"slices"
	"testing"

	"Kaleidoscopedb/Backend/KaleidoscopeBackend/imageset"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSplitOverrides(t *testing.T) {
	a, b := bson.NewObjectID().Hex(), bson.NewObjectID().Hex()

	includes, excludes, reversed := splitOverrides([]string{a, "-" + b})

	if !slices.Equal(includes, []string{a}) {
		t.Errorf("includes = %v, want [%s]", includes, a)
	}
	if !slices.Equal(excludes, []string{b}) {
		t.Errorf("excludes = %v, want [%s]", excludes, b)
	}
	if !slices.Equal(reversed, []string{"-" + a, b}) {
		t.Errorf("reversed = %v, want [-%s %s]", reversed, a, b)
	}
}

func TestPatchCountDeltas(t *testing.T) {
	owner := bson.NewObjectID()
	a := bson.NewObjectID()
	other := bson.NewObjectID()

	tests := []struct {
		name     string
		tags     []string
		includes []string
		excludes []string
		want     int
	}{
		{"include not in tags", []string{other.Hex()}, []string{a.Hex()}, nil, 1},
		{"exclude in tags", []string{a.Hex(), other.Hex()}, nil, []string{a.Hex()}, -1},
		{"include already in tags", []string{a.Hex()}, []string{a.Hex()}, nil, 0},
		{"exclude not in tags", []string{other.Hex()}, nil, []string{a.Hex()}, 0},
		{"nil tags, include", nil, []string{a.Hex()}, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := []imageset.SetTagState{{ID: bson.NewObjectID(), KscopeUserId: owner.Hex(), Tags: tt.tags}}
			deltas, err := patchCountDeltas(states, tt.includes, tt.excludes)
			if err != nil {
				t.Fatal(err)
			}
			if got := deltas[owner][a]; got != tt.want {
				t.Errorf("delta = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPatchCountDeltasGroupsByOwner(t *testing.T) {
	owner1, owner2 := bson.NewObjectID(), bson.NewObjectID()
	a := bson.NewObjectID()
	states := []imageset.SetTagState{
		{ID: bson.NewObjectID(), KscopeUserId: owner1.Hex()},
		{ID: bson.NewObjectID(), KscopeUserId: owner1.Hex()},
		{ID: bson.NewObjectID(), KscopeUserId: owner2.Hex(), Tags: []string{a.Hex()}},
	}

	deltas, err := patchCountDeltas(states, []string{a.Hex()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if deltas[owner1][a] != 2 {
		t.Errorf("owner1 delta = %d, want 2", deltas[owner1][a])
	}
	if deltas[owner2][a] != 0 {
		t.Errorf("owner2 delta = %d, want 0", deltas[owner2][a])
	}
}

func TestPatchCountDeltasRejectsOwnerlessSet(t *testing.T) {
	states := []imageset.SetTagState{{ID: bson.NewObjectID(), KscopeUserId: ""}}
	if _, err := patchCountDeltas(states, []string{bson.NewObjectID().Hex()}, nil); err == nil {
		t.Error("expected an error for a set with no owner")
	}
}

func TestValidateOverrideEntries(t *testing.T) {
	a, b := bson.NewObjectID().Hex(), bson.NewObjectID().Hex()

	tests := []struct {
		name    string
		entries []string
		wantErr bool
	}{
		{"valid mix", []string{a, "-" + b}, false},
		{"empty", nil, false},
		{"duplicate same sign", []string{a, a}, true},
		{"duplicate opposite sign", []string{a, "-" + a}, true},
		{"invalid hex", []string{"not-an-id"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOverrideEntries(tt.entries)
			if tt.wantErr && !errors.Is(err, ErrInvalidOverride) {
				t.Errorf("err = %v, want ErrInvalidOverride", err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		})
	}
}

func TestNormalizeOverrides(t *testing.T) {
	id := bson.NewObjectID()
	upper := []byte(id.Hex())
	for i, c := range upper {
		if c >= 'a' && c <= 'f' {
			upper[i] = c - 'a' + 'A'
		}
	}

	got := normalizeOverrides([]string{string(upper), "-" + string(upper)})
	want := []string{id.Hex(), "-" + id.Hex()}
	if !slices.Equal(got, want) {
		t.Errorf("normalizeOverrides = %v, want %v", got, want)
	}
}
