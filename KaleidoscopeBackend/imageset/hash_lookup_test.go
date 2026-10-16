package imageset

import (
	"bytes"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFindHashMatches(t *testing.T) {
	//a PDQ hash with its first n bits set, so it's n bits from the all-zero one
	pdqWithBits := func(n int) []byte {
		pdq := make([]byte, pdqHashBytes)
		for i := 0; i < n; i++ {
			pdq[i/8] |= 1 << (i % 8)
		}
		return pdq
	}
	incomingPixel := bytes.Repeat([]byte{1}, pixelHashBytes)
	otherPixel := bytes.Repeat([]byte{2}, pixelHashBytes)
	incoming := ImageHashes{Pixel: incomingPixel, PDQ: pdqWithBits(0)}

	exact := StoredImageHash{SetID: bson.NewObjectID(), ImageID: bson.NewObjectID(), Hashes: ImageHashes{Pixel: incomingPixel, PDQ: pdqWithBits(200)}}
	similar := StoredImageHash{SetID: bson.NewObjectID(), ImageID: bson.NewObjectID(), Hashes: ImageHashes{Pixel: otherPixel, PDQ: pdqWithBits(SimilarDistance)}}
	tooFar := StoredImageHash{SetID: bson.NewObjectID(), ImageID: bson.NewObjectID(), Hashes: ImageHashes{Pixel: otherPixel, PDQ: pdqWithBits(SimilarDistance + 1)}}
	noHashes := StoredImageHash{SetID: bson.NewObjectID(), ImageID: bson.NewObjectID()}
	stored := []StoredImageHash{exact, similar, tooFar, noHashes}

	got := findHashMatches(incoming, false, stored)
	want := []HashMatch{
		{SetID: exact.SetID, ImageID: exact.ImageID, Exact: true},
		{SetID: similar.SetID, ImageID: similar.ImageID, Exact: false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("matches = %+v, want the exact pixel match and the PDQ match at %d", got, SimilarDistance)
	}

	//two images without hashes must not count as equal
	if got := findHashMatches(ImageHashes{}, false, stored); len(got) != 0 {
		t.Errorf("an image without hashes matched %+v", got)
	}

	//a plain page on either side only matches exactly, and the match says so
	plainExact, plainSimilar := exact, similar
	plainExact.Plain, plainSimilar.Plain = true, true
	got = findHashMatches(incoming, false, []StoredImageHash{plainExact, plainSimilar})
	want = []HashMatch{{SetID: exact.SetID, ImageID: exact.ImageID, Exact: true, Plain: true}}
	if !slices.Equal(got, want) {
		t.Errorf("against plain stored pages: matches = %+v, want only the exact one, flagged plain", got)
	}

	got = findHashMatches(incoming, true, []StoredImageHash{exact, similar})
	want = []HashMatch{{SetID: exact.SetID, ImageID: exact.ImageID, Exact: true, Plain: true}}
	if !slices.Equal(got, want) {
		t.Errorf("a plain incoming page: matches = %+v, want only the exact one, flagged plain", got)
	}
}
