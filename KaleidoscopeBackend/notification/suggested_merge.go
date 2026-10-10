package notification

import "go.mongodb.org/mongo-driver/v2/bson"

// MatchKind is how an image pair matched.
type MatchKind string

const (
	MatchExact   MatchKind = "exact"
	MatchSimilar MatchKind = "similar"
)

// SuggestedMerge proposes merging the set an import produced (SetID) with
// each candidate set.
type SuggestedMerge struct {
	SetID      bson.ObjectID    `json:"set_id" bson:"set_id"`
	Candidates []MergeCandidate `json:"candidates" bson:"candidates"`
}

// MergeCandidate is one set suggested for merging. No Pairs means it was
// found only by sharing a source.
type MergeCandidate struct {
	SetID bson.ObjectID `json:"set_id" bson:"set_id"`
	Pairs []ImagePair   `json:"pairs,omitempty" bson:"pairs,omitempty"`
}

// ImagePair matches ImageID in the suggestion's set with CandidateImageID in
// the candidate set.
type ImagePair struct {
	ImageID          bson.ObjectID `json:"image_id" bson:"image_id"`
	CandidateImageID bson.ObjectID `json:"candidate_image_id" bson:"candidate_image_id"`
	Match            MatchKind     `json:"match" bson:"match"`
}

func (SuggestedMerge) envelope() envelopeInfo {
	return envelopeInfo{typ: TypeSuggestedMerge, status: StatusPending}
}
