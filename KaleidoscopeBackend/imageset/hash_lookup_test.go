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

	got := findHashMatches(incoming, stored)
	want := []HashMatch{
		{SetID: exact.SetID, ImageID: exact.ImageID, Exact: true},
		{SetID: similar.SetID, ImageID: similar.ImageID, Exact: false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("matches = %+v, want the exact pixel match and the PDQ match at %d", got, SimilarDistance)
	}

	//two images without hashes must not count as equal
	if got := findHashMatches(ImageHashes{}, stored); len(got) != 0 {
		t.Errorf("an image without hashes matched %+v", got)
	}
}
