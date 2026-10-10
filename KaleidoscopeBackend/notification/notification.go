package notification

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Type is the kind of a notification; its payload decodes by it.
type Type string

const (
	TypeImportReport       Type = "import_report"
	TypeSuggestedMerge     Type = "suggested_merge"
	TypePendingImageUpdate Type = "pending_image_update"
)

// Status is the decision state of a notification that waits on the user.
// Receipts have none.
type Status string

const (
	StatusPending  Status = "pending"
	StatusCombined Status = "combined"
	StatusAccepted Status = "accepted"
	StatusDeclined Status = "declined"
	StatusInvalid  Status = "invalid"
)

// Notification is the stored envelope. It is only written: reading has to
// keep the payload raw and decode it by Type.
type Notification struct {
	ID        bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID    string        `json:"user_id" bson:"user_id"`
	Type      Type          `json:"type" bson:"type"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time     `json:"updated_at" bson:"updated_at"`
	ReadAt    time.Time     `json:"read_at,omitempty" bson:"read_at,omitempty"` //absent until read
	Status    Status        `json:"status,omitempty" bson:"status,omitempty"`
	Payload   Payload       `json:"payload" bson:"payload"`
}

// Payload is a notification body. Only this package's types implement it.
type Payload interface {
	envelope() envelopeInfo
}

// envelopeInfo is what a payload decides about its own notification.
type envelopeInfo struct {
	typ        Type
	status     Status
	startsRead bool
}

func newNotification(userID string, p Payload, now time.Time) Notification {
	info := p.envelope()
	n := Notification{
		UserID:    userID,
		Type:      info.typ,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    info.status,
		Payload:   p,
	}
	if info.startsRead {
		n.ReadAt = now
	}
	return n
}
