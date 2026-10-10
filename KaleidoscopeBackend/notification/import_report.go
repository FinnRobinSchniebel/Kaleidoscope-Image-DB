package notification

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Outcome is how an import run ended.
type Outcome string

const (
	OutcomeCompleted Outcome = "completed"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

// ItemKind names the ImportReport list an ItemResult belongs in.
type ItemKind string

const (
	ItemAdded     ItemKind = "added"
	ItemUpdated   ItemKind = "updated"
	ItemPending   ItemKind = "pending"
	ItemSkipped   ItemKind = "skipped"
	ItemFailed    ItemKind = "failed"
	ItemMissing   ItemKind = "missing"
	ItemUnchanged ItemKind = "unchanged"
)

// ItemResult is one thing an import did.
type ItemResult struct {
	Kind   ItemKind      `json:"-" bson:"-"`                          //picks the list; implied by it once recorded
	Ref    string        `json:"ref" bson:"ref"`                      //zip group key or file path, or source ID
	SetID  bson.ObjectID `json:"set_id,omitempty" bson:"set_id,omitempty"`
	Reason string        `json:"reason,omitempty" bson:"reason,omitempty"`
}

// ImportReport is the receipt of one sync run or zip upload.
type ImportReport struct {
	Origin    string       `json:"origin" bson:"origin"` //service name or zip file name
	Started   time.Time    `json:"started" bson:"started"`
	Finished  time.Time    `json:"finished" bson:"finished"`
	Outcome   Outcome      `json:"outcome" bson:"outcome"`
	Error     string       `json:"error,omitempty" bson:"error,omitempty"`
	Added     []ItemResult `json:"added,omitempty" bson:"added,omitempty"`
	Updated   []ItemResult `json:"updated,omitempty" bson:"updated,omitempty"`
	Pending   []ItemResult `json:"pending,omitempty" bson:"pending,omitempty"`
	Skipped   []ItemResult `json:"skipped,omitempty" bson:"skipped,omitempty"`
	Failed    []ItemResult `json:"failed,omitempty" bson:"failed,omitempty"`
	Missing   []ItemResult `json:"missing,omitempty" bson:"missing,omitempty"`
	Unchanged int          `json:"unchanged" bson:"unchanged"`
	Notes     []string     `json:"notes,omitempty" bson:"notes,omitempty"`
}

func NewImportReport(origin string) *ImportReport {
	return &ImportReport{Origin: origin, Started: time.Now()}
}

// Record adds each result to the list its Kind names; unchanged results are
// only counted.
func (r *ImportReport) Record(results ...ItemResult) {
	for _, res := range results {
		switch res.Kind {
		case ItemAdded:
			r.Added = append(r.Added, res)
		case ItemUpdated:
			r.Updated = append(r.Updated, res)
		case ItemPending:
			r.Pending = append(r.Pending, res)
		case ItemSkipped:
			r.Skipped = append(r.Skipped, res)
		case ItemFailed:
			r.Failed = append(r.Failed, res)
		case ItemMissing:
			r.Missing = append(r.Missing, res)
		case ItemUnchanged:
			r.Unchanged++
		}
	}
}

// Finish marks the report ended; err is the run's own failure, if any. Only
// err's user-facing reason is stored, so the caller logs err itself.
func (r *ImportReport) Finish(outcome Outcome, err error) {
	r.Finished = time.Now()
	r.Outcome = outcome
	if err != nil {
		r.Error = reasonFor(err)
	}
}

// envelope starts the report read when nothing in it needs a look: no
// changes, pending decisions, failures or newly missing sources, and the run
// didn't fail.
func (r *ImportReport) envelope() envelopeInfo {
	quiet := len(r.Added)+len(r.Updated)+len(r.Pending)+len(r.Failed)+len(r.Missing) == 0 &&
		r.Outcome != OutcomeFailed
	return envelopeInfo{typ: TypeImportReport, startsRead: quiet}
}
