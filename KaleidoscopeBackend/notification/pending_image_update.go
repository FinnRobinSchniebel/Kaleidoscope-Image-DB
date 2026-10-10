package notification

import "go.mongodb.org/mongo-driver/v2/bson"

// PendingImageUpdate reports a source whose images changed at the service.
type PendingImageUpdate struct {
	SetID    bson.ObjectID `json:"set_id" bson:"set_id"`
	Service  string        `json:"service" bson:"service"`
	SourceID string        `json:"source_id" bson:"source_id"`
}

func (PendingImageUpdate) envelope() envelopeInfo {
	return envelopeInfo{typ: TypePendingImageUpdate, status: StatusPending}
}
