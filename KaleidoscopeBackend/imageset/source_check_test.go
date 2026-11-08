package imageset

import (
	"crypto/sha256"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// pagePDQ is a PDQ hash standing for the page named name, about 128 bits from
// any other name's.
func pagePDQ(name string) []byte {
	sum := sha256.Sum256([]byte("pdq " + name))
	return sum[:]
}

// pagePixels is the pixel hash of the page named name.
func pagePixels(name string) []byte {
	sum := sha256.Sum256([]byte("pixels " + name))
	return sum[:]
}

// flipBits returns a copy of hash with the bits at positions flipped.
func flipBits(hash []byte, positions ...int) []byte {

	flipped := slices.Clone(hash)
	for _, position := range positions {
		flipped[position/8] ^= 1 << (position % 8)
	}
	return flipped
}

// preview is a fetched preview with the given PDQ hash and no pixel hash, as no
// preview is pixel-identical to a stored original.
func preview(pdq []byte) PreparedImage {
	return PreparedImage{Info: ImageInfo{ImageHashes: ImageHashes{PDQ: pdq}}}
}

// original is the fetched original of the page named name.
func original(name string) PreparedImage {
	return PreparedImage{Info: ImageInfo{ImageHashes: ImageHashes{Pixel: pagePixels(name), PDQ: pagePDQ(name)}}}
}

// previewsOf previews the pages named names, unchanged.
func previewsOf(names ...string) []PreparedImage {

	previews := make([]PreparedImage, len(names))
	for i, name := range names {
		previews[i] = preview(pagePDQ(name))
	}
	return previews
}

// newCheckSet returns user1's set whose pixiv/1 copy, dated date and titled Old,
// attributes the images named own in order; the images named others are a zip
// source's. Every image is hashed from its name and sized 100x200, and ids maps
// each name to its image's ID.
func newCheckSet(date time.Time, own, others []string) (*ImageSetMongo, map[string]bson.ObjectID) {

	ids := make(map[string]bson.ObjectID)
	set := &ImageSetMongo{ID: bson.NewObjectID(), KscopeUserId: "user1", Title: "Old"}
	attribute := func(names []string) []bson.ObjectID {

		attributed := make([]bson.ObjectID, len(names))
		for i, name := range names {

			image := ImageInfo{ID: bson.NewObjectID(), ImageHashes: ImageHashes{Pixel: pagePixels(name), PDQ: pagePDQ(name)}, Width: 100, Height: 200}
			set.Image = append(set.Image, image)
			ids[name] = image.ID
			attributed[i] = image.ID
		}
		return attributed
	}

	lastChecked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	set.Sources = []SourceInfo{{Name: "pixiv", SourceID: "1", Title: "Old", Date: date, LastChecked: lastChecked, AttributedTo: attribute(own)}}
	if len(others) > 0 {
		set.Sources = append(set.Sources, SourceInfo{Name: "zipsrc", SourceID: "9", AttributedTo: attribute(others)})
	}
	return set, ids
}

// findImage returns the set's image with id, for a test to adjust.
func findImage(set *ImageSetMongo, id bson.ObjectID) *ImageInfo {
	return &set.Image[slices.IndexFunc(set.Image, func(image ImageInfo) bool { return image.ID == id })]
}

// idsOf maps names to their image IDs, keeping nil as nil.
func idsOf(ids map[string]bson.ObjectID, names []string) []bson.ObjectID {

	if names == nil {
		return nil
	}

	mapped := make([]bson.ObjectID, len(names))
	for i, name := range names {
		mapped[i] = ids[name]
	}
	return mapped
}

var (
	checkDate   = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	zipDate     = DayOnlyDate(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	serviceDate = time.Date(2026, 8, 27, 9, 12, 12, 0, time.UTC)
)

func TestCheckSourceImages(t *testing.T) {

	nearX := func(bits int) []byte {

		positions := make([]int, bits)
		for i := range positions {
			positions[i] = i
		}
		return flipBits(pagePDQ("x"), positions...)
	}

	tests := []struct {
		name        string
		date        time.Time
		own, others []string
		adjust      func(set *ImageSetMongo, ids map[string]bson.ObjectID)
		previews    []PreparedImage
		originals   map[int]PreparedImage
		firstPage   [2]int //the listing's width and height; zero means 100x200
		kind        SourceWriteKind
		attribution []string
		matched     []string //an insert's match per page; "" is a page to insert
		needs       []int
	}{
		{
			name: "every page in place", date: checkDate, own: []string{"p1", "p2", "p3"},
			previews: previewsOf("p1", "p2", "p3"), kind: SourcePendingFlip,
		},
		{
			name: "in place with a leftover at the end", date: checkDate, own: []string{"p1", "p2", "p3", "old"},
			previews: previewsOf("p1", "p2", "p3"), kind: SourcePendingFlip,
		},
		{
			name: "reordered with a leftover behind", date: checkDate, own: []string{"p1", "p2", "p3", "old"},
			previews: previewsOf("p2", "p1", "p3"), kind: SourceReorder, attribution: []string{"p2", "p1", "p3", "old"},
		},
		{
			name: "near-identical pages stay in place", date: checkDate, own: []string{"p1", "p1n"},
			adjust: func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
				findImage(set, ids["p1n"]).PDQ = flipBits(pagePDQ("p1"), 0, 1)
			},
			//page 0 is 3 bits from its own page and 1 bit from the next one
			previews: []PreparedImage{preview(flipBits(pagePDQ("p1"), 0, 1, 2)), preview(flipBits(pagePDQ("p1"), 0, 1))},
			kind:     SourcePendingFlip,
		},
		{
			name: "a middle page gone", date: checkDate, own: []string{"p1", "p2", "p3", "p4"},
			previews: previewsOf("p1", "p3", "p4"), kind: SourceRemoval, attribution: []string{"p1", "p3", "p4", "p2"},
		},
		{
			name: "the last page gone", date: checkDate, own: []string{"p1", "p2", "p3"},
			previews: previewsOf("p1", "p2"), kind: SourcePendingFlip,
		},
		{
			name: "a page edited", date: checkDate, own: []string{"p1", "p2", "p3"},
			previews: []PreparedImage{preview(pagePDQ("p1")), preview(flipBits(pagePDQ("p2"), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)), preview(pagePDQ("p3"))},
			kind:     SourcePendingFlip,
		},
		{
			name: "a page added", date: checkDate, own: []string{"p1", "p2"},
			previews: previewsOf("p1", "p2", "p3"), needs: []int{2},
		},
		{
			name: "a page added, original fetched", date: checkDate, own: []string{"p1", "p2"},
			previews: previewsOf("p1", "p2", "p3"), originals: map[int]PreparedImage{2: original("p3")},
			kind: SourceInsert, matched: []string{"p1", "p2", ""},
		},
		{
			name: "a page added first", date: checkDate, own: []string{"p1", "p2"},
			previews: previewsOf("p3", "p1", "p2"), originals: map[int]PreparedImage{0: original("p3")},
			kind: SourceInsert, matched: []string{"", "p1", "p2"},
		},
		{
			name: "reordered with a page added", date: checkDate, own: []string{"p1", "p2"},
			previews: previewsOf("p2", "p1", "p3"), originals: map[int]PreparedImage{2: original("p3")},
			kind: SourceInsert, matched: []string{"p2", "p1", ""},
		},
		{
			name: "a page added beside a page another source brought", date: checkDate, own: []string{"p1"}, others: []string{"x"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(pagePDQ("p3"))},
			originals: map[int]PreparedImage{1: original("x"), 2: original("p3")},
			kind:      SourceInsert, matched: []string{"p1", "x", ""},
		},
		{
			name: "a page added that looks like an own page", date: checkDate, own: []string{"p1", "p2"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(pagePDQ("p2")), preview(flipBits(pagePDQ("p2"), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9))},
			originals: map[int]PreparedImage{2: {Info: ImageInfo{ImageHashes: ImageHashes{Pixel: pagePixels("variant"), PDQ: flipBits(pagePDQ("p2"), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)}}}},
			kind:      SourceInsert, matched: []string{"p1", "p2", ""},
		},
		{
			name: "a page added that is pixel-identical to a page already taken", date: checkDate, own: []string{"p1", "p2"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(pagePDQ("p2")), preview(flipBits(pagePDQ("p2"), 0, 1, 2, 3, 4, 5))},
			originals: map[int]PreparedImage{2: original("p2")},
			kind:      SourcePendingFlip,
		},
		{
			name: "a page added while a leftover sits at the end", date: checkDate, own: []string{"p1", "p2", "old"},
			previews: previewsOf("p1", "p2", "p3"), kind: SourcePendingFlip,
		},
		{
			name: "zip-dated, a page added first", date: zipDate, own: []string{"p1", "p2"},
			previews: previewsOf("p3", "p1", "p2"), originals: map[int]PreparedImage{0: original("p3")},
			kind: SourcePendingFlip,
		},
		{
			name: "zip-dated, a page added at the end", date: zipDate, own: []string{"p1", "p2"},
			previews: previewsOf("p1", "p2", "p3"), originals: map[int]PreparedImage{2: original("p3")},
			kind: SourceInsert, matched: []string{"p1", "p2", ""},
		},
		{
			name: "a page near another image, no original", date: checkDate, own: []string{"p1", "p2", "p3"}, others: []string{"x"},
			previews: []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(pagePDQ("p3"))},
			needs:    []int{1},
		},
		{
			name: "a page near another image, exact original", date: checkDate, own: []string{"p1", "p2", "p3"}, others: []string{"x"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(pagePDQ("p3"))},
			originals: map[int]PreparedImage{1: original("x")},
			kind:      SourceAttribution, attribution: []string{"p1", "x", "p3", "p2"},
		},
		{
			name: "a page near another image, original not exact", date: checkDate, own: []string{"p1", "p2", "p3"}, others: []string{"x"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(pagePDQ("p3"))},
			originals: map[int]PreparedImage{1: original("y")},
			kind:      SourcePendingFlip,
		},
		{
			name: "a plain page near another image", date: checkDate, own: []string{"p1", "p2"}, others: []string{"x"},
			previews: []PreparedImage{preview(pagePDQ("p1")), {Info: ImageInfo{ImageHashes: ImageHashes{PDQ: nearX(10)}}, Plain: true}},
			kind:     SourcePendingFlip,
		},
		{
			name: "zip-dated, in place", date: zipDate, own: []string{"p1", "p2", "p3"},
			previews: previewsOf("p1", "p2", "p3"), kind: SourceMetadataUpdate, attribution: []string{"p1", "p2", "p3"},
		},
		{
			name: "zip-dated, in place, size differs", date: zipDate, own: []string{"p1", "p2", "p3"},
			previews: previewsOf("p1", "p2", "p3"), firstPage: [2]int{100, 201}, kind: SourcePendingFlip,
		},
		{
			name: "no date, in place", own: []string{"p1", "p2", "p3"},
			previews: previewsOf("p1", "p2", "p3"), kind: SourceMetadataUpdate, attribution: []string{"p1", "p2", "p3"},
		},
		{
			name: "zip-dated reorder, size of the page now first", date: zipDate, own: []string{"p1", "p2", "p3"},
			adjust: func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
				findImage(set, ids["p2"]).Width, findImage(set, ids["p2"]).Height = 300, 400
			},
			previews: previewsOf("p2", "p1", "p3"), firstPage: [2]int{300, 400},
			kind: SourceReorder, attribution: []string{"p2", "p1", "p3"},
		},
		{
			name: "zip-dated reorder, size of the page that was first", date: zipDate, own: []string{"p1", "p2", "p3"},
			adjust: func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
				findImage(set, ids["p2"]).Width, findImage(set, ids["p2"]).Height = 300, 400
			},
			previews: previewsOf("p2", "p1", "p3"), kind: SourcePendingFlip,
		},
		{
			name: "a moved page takes the closer own image", date: checkDate, own: []string{"z", "far", "close"},
			adjust: func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
				findImage(set, ids["far"]).PDQ = flipBits(pagePDQ("base"), 0, 1, 2)
				findImage(set, ids["close"]).PDQ = flipBits(pagePDQ("base"), 0)
			},
			previews: previewsOf("base"), kind: SourceRemoval, attribution: []string{"close", "z", "far"},
		},
		{
			name: "a moved page takes the earlier own image on a tie", date: checkDate, own: []string{"z", "first", "second"},
			adjust: func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
				findImage(set, ids["first"]).PDQ = flipBits(pagePDQ("base"), 0, 1)
				findImage(set, ids["second"]).PDQ = flipBits(pagePDQ("base"), 2, 3)
			},
			previews: previewsOf("base"), kind: SourceRemoval, attribution: []string{"first", "z", "second"},
		},
		{
			name: "two originals exact to one image", date: checkDate, own: []string{"p1"}, others: []string{"x"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(nearX(12))},
			originals: map[int]PreparedImage{1: original("x"), 2: original("x")},
			kind:      SourcePendingFlip,
		},
		{
			name: "a middle page gone and the rest reordered", date: checkDate, own: []string{"p1", "p2", "p3", "p4"},
			previews: previewsOf("p4", "p1", "p3"), kind: SourceRemoval, attribution: []string{"p4", "p1", "p3", "p2"},
		},
		{
			name: "a page added that is in the set", date: checkDate, own: []string{"p1", "p2"}, others: []string{"x"},
			previews:  []PreparedImage{preview(pagePDQ("p1")), preview(pagePDQ("p2")), preview(nearX(10))},
			originals: map[int]PreparedImage{2: original("x")},
			kind:      SourceAttribution, attribution: []string{"p1", "p2", "x"},
		},
		{
			name: "nothing attributed, no hits", date: checkDate,
			previews: previewsOf("p1"), kind: SourcePendingFlip,
		},
		{
			name: "no previews", date: checkDate, own: []string{"p1", "p2"},
			kind: SourcePendingFlip,
		},
		{
			name: "a hit before a page with no hit", date: checkDate, own: []string{"p1", "p2", "p3"}, others: []string{"x"},
			previews: []PreparedImage{preview(pagePDQ("p1")), preview(nearX(10)), preview(flipBits(pagePDQ("p3"), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15))},
			kind:     SourcePendingFlip,
		},
	}

	for _, tt := range tests {

		set, ids := newCheckSet(tt.date, tt.own, tt.others)
		if tt.adjust != nil {
			tt.adjust(set, ids)
		}

		pages := FetchedPages{Previews: tt.previews, Originals: tt.originals, FirstPageWidth: 100, FirstPageHeight: 200}
		if tt.firstPage != [2]int{} {
			pages.FirstPageWidth, pages.FirstPageHeight = tt.firstPage[0], tt.firstPage[1]
		}

		check, err := checkSourceImages(set, 0, pages)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}

		if check.kind != tt.kind || !slices.Equal(check.attribution, idsOf(ids, tt.attribution)) || !slices.Equal(check.matched, idsOf(ids, tt.matched)) || !slices.Equal(check.needsOriginals, tt.needs) {
			t.Errorf("%s: kind %q, attribution %v, matched %v, needs %v; want %q, %v (%v), %v (%v), %v", tt.name, check.kind, check.attribution, check.matched, check.needsOriginals,
				tt.kind, idsOf(ids, tt.attribution), tt.attribution, idsOf(ids, tt.matched), tt.matched, tt.needs)
		}
	}
}

func TestNeedsImageCheck(t *testing.T) {

	sameDay := DayOnlyDate(serviceDate)
	otherDay := DayOnlyDate(serviceDate.AddDate(0, 0, -3))
	tests := []struct {
		name        string
		src         SourceInfo
		serviceDate time.Time
		want        bool
	}{
		{"an exact date moved", SourceInfo{Date: checkDate}, serviceDate, true},
		{"pending, moved", SourceInfo{Date: checkDate, ImageChangeState: ImageChangePending}, serviceDate, false},
		{"declined, moved", SourceInfo{Date: checkDate, ImageChangeState: ImageChangeDeclined}, serviceDate, false},
		{"the same instant", SourceInfo{Date: serviceDate}, serviceDate, false},
		{"day-only on the service's day", SourceInfo{Date: sameDay}, serviceDate, false},
		{"day-only on another day", SourceInfo{Date: otherDay}, serviceDate, true},
		{"no stored date", SourceInfo{}, serviceDate, true},
		{"no service date", SourceInfo{Date: checkDate}, time.Time{}, false},
	}

	for _, tt := range tests {
		if got := NeedsImageCheck(tt.src, tt.serviceDate); got != tt.want {
			t.Errorf("%s: NeedsImageCheck = %t, want %t", tt.name, got, tt.want)
		}
	}
}

func TestFindPagesNeedingOriginals(t *testing.T) {

	//pages q0, p1, q2; x0 and x2 are near q0 and q2, x1 near p1
	near := func(name string) func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
		return func(set *ImageSetMongo, ids map[string]bson.ObjectID) {
			findImage(set, ids["x"+name[1:]]).PDQ = flipBits(pagePDQ(name), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		}
	}

	build := func(date time.Time, state ImageChangeState, own, others []string, nearNames ...string) *ImageSetMongo {

		set, ids := newCheckSet(date, own, others)
		set.Sources[0].ImageChangeState = state
		for _, name := range nearNames {
			near(name)(set, ids)
		}
		return set
	}

	sets := []*ImageSetMongo{
		build(checkDate, ImageChangeNone, []string{"q0", "p1"}, []string{"x2"}, "q2"),
		build(checkDate, ImageChangeNone, []string{"p1"}, []string{"x0", "x2"}, "q0", "q2"),
		build(checkDate, ImageChangePending, []string{"q0", "q2"}, []string{"x1"}, "p1"),
		build(serviceDate, ImageChangeNone, []string{"q0", "q2"}, []string{"x1"}, "p1"),
	}

	pages := FetchedPages{Previews: previewsOf("q0", "p1", "q2"), FirstPageWidth: 100, FirstPageHeight: 200}
	needed, err := FindPagesNeedingOriginals(sets, SourceInfo{Name: "pixiv", SourceID: "1", Date: serviceDate}, pages)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(needed, []int{0, 2}) {
		t.Errorf("needed %v, want [0 2]: the held and current sets left out", needed)
	}

	//a possible pure addition needs the originals of the pages that matched nothing
	added := build(checkDate, ImageChangeNone, []string{"q0"}, nil)
	needed, err = FindPagesNeedingOriginals([]*ImageSetMongo{added}, SourceInfo{Name: "pixiv", SourceID: "1", Date: serviceDate}, pages)
	if err != nil || !slices.Equal(needed, []int{1, 2}) {
		t.Errorf("pure addition: needed %v (%v), want [1 2]", needed, err)
	}
}
