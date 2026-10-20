package imageset

import (
	"bytes"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// preparedWithPixels returns a prepared image whose pixel hash is pixels repeated.
func preparedWithPixels(pixels byte) PreparedImage {
	return PreparedImage{Info: ImageInfo{ID: bson.NewObjectID(), ImageHashes: ImageHashes{Pixel: bytes.Repeat([]byte{pixels}, pixelHashBytes)}}}
}

func TestClassifyCandidate(t *testing.T) {

	type hit struct {
		incoming, stored int
		exact, plain     bool
	}
	tests := []struct {
		name             string
		stored, incoming int
		hits             []hit
		want             mergeScenario
	}{
		{"duplicate", 2, 2, []hit{{0, 0, true, false}, {1, 1, true, false}}, scenarioDuplicate},
		{"duplicate with a repeated incoming image", 2, 3, []hit{{0, 0, true, false}, {1, 0, true, false}, {2, 1, true, false}}, scenarioDuplicate},
		{"addition", 2, 3, []hit{{0, 0, true, false}, {1, 1, true, false}}, scenarioAddition},
		{"less", 3, 2, []hit{{0, 0, true, false}, {1, 1, true, false}}, scenarioLess},
		{"partial, both sides have extras", 2, 2, []hit{{0, 0, true, false}}, scenarioPartial},
		{"partial, exact plus similar-only", 2, 2, []hit{{0, 0, true, false}, {1, 1, false, false}}, scenarioPartial},
		{"similar", 2, 2, []hit{{0, 0, false, false}, {1, 1, false, false}}, scenarioSimilar},
		{"a stored image only similar to an incoming one isn't hit", 2, 2, []hit{{0, 0, true, false}, {0, 1, false, false}}, scenarioPartial},
		{"a plain exact page inside a candidate counts as hit", 2, 2, []hit{{0, 0, true, true}, {1, 1, true, false}}, scenarioDuplicate},
	}
	for _, tt := range tests {

		set := &ImageSetMongo{ID: bson.NewObjectID()}
		for range tt.stored {
			set.Image = append(set.Image, ImageInfo{ID: bson.NewObjectID()})
		}

		incoming := make([]PreparedImage, tt.incoming)
		for i := range incoming {
			incoming[i] = preparedWithPixels(byte(i))
		}

		matches := make(CollisionMap)
		for _, h := range tt.hits {
			matches[h.incoming] = append(matches[h.incoming], HashMatch{SetID: set.ID, ImageID: set.Image[h.stored].ID, Exact: h.exact, Plain: h.plain})
		}

		if got := classifyCandidate(set, incoming, matches); got.scenario != tt.want {
			t.Errorf("%s: scenario = %s, want %s", tt.name, got.scenario, tt.want)
		}
	}

	setA, setB, setC := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	matches := CollisionMap{
		0: {{SetID: setA, Exact: true, Plain: true}, {SetID: setC, Exact: true, Plain: true}},
		1: {{SetID: setB, Exact: true}, {SetID: setA}},
	}
	if got := findCandidates(matches, 2); !slices.Equal(got, []bson.ObjectID{setB, setA}) {
		t.Errorf("findCandidates = %v, want B then A in first-hit order, and not C (only a plain page)", got)
	}

}

func TestSplitCandidates(t *testing.T) {

	scenarios := []mergeScenario{scenarioDuplicate, scenarioPartial, scenarioAddition, scenarioSimilar, scenarioLess}
	candidates := make([]candidateMatch, len(scenarios))
	for i, scenario := range scenarios {
		candidates[i] = candidateMatch{scenario: scenario}
	}

	scenariosOf := func(list []candidateMatch) []mergeScenario {
		var got []mergeScenario
		for _, candidate := range list {
			got = append(got, candidate.scenario)
		}
		return got
	}

	automatic, suggested := splitCandidates(candidates)
	if want := []mergeScenario{scenarioDuplicate, scenarioAddition, scenarioLess}; !slices.Equal(scenariosOf(automatic), want) {
		t.Errorf("automatic = %v, want %v in order", scenariosOf(automatic), want)
	}

	if want := []mergeScenario{scenarioPartial, scenarioSimilar}; !slices.Equal(scenariosOf(suggested), want) {
		t.Errorf("suggested = %v, want %v in order", scenariosOf(suggested), want)
	}
}

// insertFixture is a stored set [A,B] and an import [N0, A, N1, N1 again, B, N2].
func insertFixture() (existing []ImageInfo, incoming []PreparedImage, match candidateMatch) {

	idA, idB := bson.NewObjectID(), bson.NewObjectID()
	existing = []ImageInfo{{ID: idA}, {ID: idB}}

	repeat := preparedWithPixels(3)
	repeat.Info.ID = bson.NewObjectID()
	incoming = []PreparedImage{preparedWithPixels(1), preparedWithPixels(2), preparedWithPixels(3), repeat, preparedWithPixels(4), preparedWithPixels(5)}
	match = candidateMatch{scenario: scenarioAddition, exactOf: [][]bson.ObjectID{nil, {idA}, nil, nil, {idB}, nil}}
	return existing, incoming, match
}

func TestPlanAndPlaceInsertedImages(t *testing.T) {

	existing, incoming, match := insertFixture()
	keeperOf, added := planNewImages(incoming, match)

	if want := []int{0, -1, 2, 2, -1, 5}; !slices.Equal(keeperOf, want) {
		t.Fatalf("keeperOf = %v, want %v: the repeat of N1 planned once", keeperOf, want)
	}

	for i, info := range added {
		if info.ID == incoming[i].Info.ID || info.ID.IsZero() || !info.IsImageActive {
			t.Errorf("planned image %d: ID %v (prepared %v), active %t; want a fresh active ID", i, info.ID, incoming[i].Info.ID, info.IsImageActive)
		}
	}

	placed := placeInsertedImages(existing, match, keeperOf, added)
	want := []bson.ObjectID{added[0].ID, existing[0].ID, added[2].ID, existing[1].ID, added[5].ID}
	got := make([]bson.ObjectID, len(placed))
	for i, img := range placed {
		got[i] = img.ID
	}

	if !slices.Equal(got, want) {
		t.Errorf("placed = %v, want N0 first, N1 after A, N2 after B: %v", got, want)
	}
}

func TestTargetImageIDsAndMergeAttribution(t *testing.T) {

	existing, incoming, match := insertFixture()
	copyOfA := bson.NewObjectID()
	match.exactOf[1] = append(match.exactOf[1], copyOfA)

	keeperOf, added := planNewImages(incoming, match)
	got := targetImageIDs(match, keeperOf, added)
	want := []bson.ObjectID{added[0].ID, existing[0].ID, copyOfA, added[2].ID, existing[1].ID, added[5].ID}
	if !slices.Equal(got, want) {
		t.Errorf("targetImageIDs = %v, want page order with both copies of A and the repeat once: %v", got, want)
	}

	oldOnly := bson.NewObjectID()
	merged := mergeAttribution([]bson.ObjectID{existing[1].ID, oldOnly}, got)
	if wantMerged := append(slices.Clone(got), oldOnly); !slices.Equal(merged, wantMerged) {
		t.Errorf("mergeAttribution = %v, want the incoming order then the old image it no longer lists", merged)
	}
}
