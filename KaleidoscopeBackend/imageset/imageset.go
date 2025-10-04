package imageset

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type SourceInfo struct {
	Name     string    `json:"name" form:"name"`
	ID       string    `json:"id" form:"id"`
	Title    string    `json:"title" form:"title"`
	SourceID string    `json:"sourceid" form:"sourceid"`
	Tags     []string  `json:"tags" form:"tags"`
	Date     time.Time `json:"date" form:"date"`
}
type ImageInfo struct {
	Name          string `json:"images" bson:"images" form:"images"`
	LowResName    string `json:"low_images" bson:"low_images" form:"low_images"`
	IsImageActive bool   `json:"active,omitempty" bson:"active,omitempty" form:"active"`
	ImageHash     string `json:"hash" bson:"hash,omitempty" form:"hash"`
}

type ImageSetMongo struct {
	ID               bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Title            string        `json:"title" bson:"title,omitempty" form:"title"`
	Tags             []string      `json:"tags" bson:"tags,omitempty" form:"tags"`
	Sources          []SourceInfo  `json:"sources" bson:"sources,omitempty" form:"sources"`
	Authors          []string      `json:"authors" bson:"authors,omitempty" form:"authors"`
	Path             string        `json:"path" bson:"path,omitempty" form:"path"`
	Image            []ImageInfo   `json:"images,omitempty" bson:"images,omitempty" form:"images"`
	AutoTags         []string      `json:"autotags" bson:"autotags,omitempty" form:"autotags"`
	TagRuleOverrides []string      `json:"tag_rule_overrides" bson:"tag_rule_overrides,omitempty" form:"tag_rule_overrides"`
	Itype            string        `json:"type" bson:"type,omitempty" form:"type"`
	Description      string        `json:"description" bson:"description,omitempty" form:"description"`
	Other            string        `json:"other" bson:"other,omitempty" form:"other"`
	KscopeUserId     string        `json:"kscope_userid" bson:"kscope_userid" form:"kscope_userid"`
	DateAdded        time.Time     `json:"date_added" bson:"date_added" form:"date_added"`
	// API will send file as well but it will not be placed in the struct: `json: media`
}

type InternalResponse struct {
	ErrorCode   int
	ErrorString string
}

func CleanImagSetForFrontEnd(iSet ...ImageSetMongo) []ImageSetMongo {
	for index, _ := range iSet {
		iSet[index].Image = nil
		//iSet[index].LowImage = nil
		//iSet[index].IsImageActive = nil
		iSet[index].Path = ""
	}
	return iSet
}
