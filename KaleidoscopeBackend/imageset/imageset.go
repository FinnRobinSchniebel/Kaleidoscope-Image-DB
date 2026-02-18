package imageset

import (
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type SourceInfo struct {
	Name         string    `json:"name" form:"name"`
	ID           string    `json:"id" form:"id"`                       // ID of source itself (created by DB)
	Title        string    `json:"title" form:"title"`                 // Title of work at source
	SourceAuthor string    `json:"source_author" form:"source_author"` //the authors name at this source
	AttributedTo []int     `json:"attributed_to" form:"attributed_to"` //index of images in set that this source belongs to
	SourceID     string    `json:"source_id" form:"source_id"`         // id of art WORK at the source
	AuthorID     string    `json:"author_id" form:"author_id"`         //id the author user was assigned
	Tags         []string  `json:"tags" form:"tags"`                   //tags provided at the source
	Date         time.Time `json:"date" form:"date"`                   //Publishing date
}

// info regarding the images location on the DB and current state
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

// only checks if the base info is the same. It does not check attribution and tags
func SourceInfoEqual(a, b SourceInfo) bool {
	if a.Name != b.Name ||

		a.Title != b.Title ||
		a.SourceAuthor != b.SourceAuthor ||
		a.SourceID != b.SourceID ||
		a.AuthorID != b.AuthorID ||
		!a.Date.Equal(b.Date) {
		return false
	}
	return true
}

func PrintISet(a ImageSetMongo) {
	log.Printf("%s", ImageSetToString(a))
}

func ImageSetToString(a ImageSetMongo) string {
	var sb strings.Builder

	sb.WriteString("====================================\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", a.ID.Hex()))
	sb.WriteString(fmt.Sprintf("Title: %s\n", a.Title))
	sb.WriteString(fmt.Sprintf("Path: %s\n", a.Path))
	sb.WriteString(fmt.Sprintf("Type: %s\n", a.Itype))
	sb.WriteString(fmt.Sprintf("Description: %s\n", a.Description))
	sb.WriteString(fmt.Sprintf("Other: %s\n", a.Other))
	sb.WriteString(fmt.Sprintf("KscopeUserId: %s\n", a.KscopeUserId))
	sb.WriteString(fmt.Sprintf("DateAdded: %s\n", a.DateAdded.Format(time.RFC3339)))

	// Tags
	sb.WriteString("\nTags:\n")
	for _, tag := range a.Tags {
		sb.WriteString(fmt.Sprintf(" - %s\n", tag))
	}

	// AutoTags
	sb.WriteString("\nAutoTags:\n")
	for _, tag := range a.AutoTags {
		sb.WriteString(fmt.Sprintf(" - %s\n", tag))
	}

	// TagRuleOverrides
	sb.WriteString("\nTagRuleOverrides:\n")
	for _, tag := range a.TagRuleOverrides {
		sb.WriteString(fmt.Sprintf(" - %s\n", tag))
	}

	// Authors
	sb.WriteString("\nAuthors:\n")
	for _, author := range a.Authors {
		sb.WriteString(fmt.Sprintf(" - %s\n", author))
	}

	// Sources
	sb.WriteString("\nSources:\n")
	for _, source := range a.Sources {
		sb.WriteString(SourcesToString(source))
	}

	// Images
	sb.WriteString("\nImages:\n")
	for _, img := range a.Image {
		sb.WriteString(ImageToString(img))
	}

	sb.WriteString("====================================\n")

	return sb.String()
}

func SourcesToString(a SourceInfo) string {
	var sb strings.Builder

	sb.WriteString("------------------------------------\n")
	sb.WriteString(fmt.Sprintf("Source Name: %s\n", a.Name))
	sb.WriteString(fmt.Sprintf("Source DB ID: %s\n", a.ID))
	sb.WriteString(fmt.Sprintf("Title At Source: %s\n", a.Title))
	sb.WriteString(fmt.Sprintf("Source Author: %s\n", a.SourceAuthor))
	sb.WriteString(fmt.Sprintf("SourceID: %s\n", a.SourceID))
	sb.WriteString(fmt.Sprintf("AuthorID: %s\n", a.AuthorID))
	sb.WriteString(fmt.Sprintf("Date: %s\n", a.Date.Format(time.RFC3339)))

	sb.WriteString("Tags:\n")
	for _, tag := range a.Tags {
		sb.WriteString(fmt.Sprintf("   - %s\n", tag))
	}

	sb.WriteString("AttributedTo (image indexes):\n")
	for _, idx := range a.AttributedTo {
		sb.WriteString(fmt.Sprintf("   - %d\n", idx))
	}

	return sb.String()
}

func ImageToString(a ImageInfo) string {
	return fmt.Sprintf(
		"------------------------------------\nImage Name: %s\nLowRes Name: %s\nActive: %t\nHash: %s\n",
		a.Name,
		a.LowResName,
		a.IsImageActive,
		a.ImageHash,
	)
}
