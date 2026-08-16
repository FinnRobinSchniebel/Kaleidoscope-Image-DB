package imageset

import (
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SourceTag holds a single tag as imported from a source. EN is a translation
// (only populated when the source actually provides one); Default is the
// source's own untranslated tag and is always populated.
type SourceTag struct {
	EN      string `json:"en,omitempty" bson:"en,omitempty" form:"en"`
	Default string `json:"default" bson:"default" form:"default"`
}

// canonical is the tag's translation-independent identity, used to detect
// whether two SourceTags refer to the same underlying tag.
func (t SourceTag) canonical() string {
	return NormalizeTagText(t.Default)
}

// NormalizeTagText is the shared tag-identity transform (lowercase, trimmed)
// so casing/whitespace differences don't produce distinct identities.
func NormalizeTagText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

type SourceInfo struct {
	Name               string      `json:"name" bson:"name" form:"name"`
	ID                 string      `json:"id" bson:"id" form:"id"`                                                       // ID of source itself (created by DB)
	Title              string      `json:"title" bson:"title" form:"title"`                                              // Title of work at source
	Description        string      `json:"description" bson:"description" form:"description"`                            //imported description, preserved even if the set's Description is edited
	SourceAuthor       string      `json:"source_author" bson:"source_author" form:"source_author"`                      //the authors name at this source
	AttributedTo       []int       `json:"attributed_to" bson:"attributed_to" form:"attributed_to"`                      //index of images in set that this source belongs to
	SourceID           string      `json:"source_id" bson:"source_id" form:"source_id"`                                  // id of art WORK at the source
	AuthorID           string      `json:"author_id" bson:"author_id" form:"author_id"`                                  //id the author user was assigned
	Tags               []SourceTag `json:"tags" bson:"tags" form:"tags"`                                                 //tags provided at the source
	Date               time.Time   `json:"date" bson:"date" form:"date"`                                                 //date reported by source; on Pixiv tracks last edit, not original post
	LastChecked        time.Time   `json:"last_checked" bson:"last_checked" form:"last_checked"`                         //last time this source was polled for changes
	LastImageUpdate    time.Time   `json:"last_image_update" bson:"last_image_update" form:"last_image_update"`          //Date value as of the last image hash check
	PendingImageChange bool        `json:"pending_image_change" bson:"pending_image_change" form:"pending_image_change"` //true once a hash check finds an unresolved image difference
	SourceMissing      bool        `json:"source_missing" bson:"source_missing" form:"source_missing"`                   //true when the source could not be fetched; existing data is left untouched
}

// info regarding the images location on the DB and current state
type ImageInfo struct {
	Name          string `json:"images" bson:"images" form:"images"`
	LowResName    string `json:"low_images" bson:"low_images" form:"low_images"`
	IsImageActive bool   `json:"active,omitempty" bson:"active,omitempty" form:"active"`
	ImageHash     string `json:"hash" bson:"hash,omitempty" form:"hash"`
}

type ImageSetMongo struct {
	ID               bson.ObjectID   `json:"id,omitempty" bson:"_id,omitempty" form:"id,omitempty"`
	Title            string          `json:"title" bson:"title,omitempty" form:"title"`
	Tags             []string        `json:"tags" bson:"tags,omitempty" form:"tags"`
	Sources          []SourceInfo    `json:"sources" bson:"sources,omitempty" form:"sources"`
	Authors          []string        `json:"authors" bson:"authors,omitempty" form:"authors"`
	Path             string          `json:"path" bson:"path,omitempty" form:"path"`
	Image            []ImageInfo     `json:"images,omitempty" bson:"images,omitempty" form:"images"`
	AutoTags         []bson.ObjectID `json:"autotags" bson:"autotags,omitempty" form:"autotags"`
	TagRuleOverrides []string        `json:"tag_rule_overrides" bson:"tag_rule_overrides,omitempty" form:"tag_rule_overrides"`
	Itype            string          `json:"type" bson:"type,omitempty" form:"type"`
	Description      string          `json:"description" bson:"description,omitempty" form:"description"`
	Other            string          `json:"other" bson:"other,omitempty" form:"other"`
	KscopeUserId     string          `json:"kscope_userid" bson:"kscope_userid" form:"kscope_userid"`
	DateAdded        time.Time       `json:"date_added" bson:"date_added" form:"date_added"`
	ThumbNail        string          `json:"thumbnail" bson:"thumbnail" form:"thumbnail"` //rendered 256x256 image path
	// API will send file as well but it will not be placed in the struct: `json: media`

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

// SetImportDescription mirrors description onto the set and onto every
// current entry in Sources. Use for a set's first import, before it has an
// existing Description to preserve.
func SetImportDescription(a *ImageSetMongo, description string) {
	if description == "" {
		return
	}
	a.Description = description
	for i := range a.Sources {
		a.Sources[i].Description = description
	}
}

// AppendSource adds source to a set that already has at least one source,
// appending description to the set's Description after a blank-line gap
// rather than overwriting it.
func AppendSource(a *ImageSetMongo, source SourceInfo, description string) {
	source.Description = description
	a.Sources = append(a.Sources, source)
	if description == "" {
		return
	}
	if a.Description == "" {
		a.Description = description
	} else {
		a.Description = a.Description + "\n\n" + description
	}
}

// UpdateSourceDescription updates only Sources[i].Description, leaving the
// set's own Description untouched.
func UpdateSourceDescription(a *ImageSetMongo, i int, description string) {
	a.Sources[i].Description = description
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

func PrintISet(a *ImageSetMongo) {
	log.Printf("%s", ImageSetToString(*a))
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
		sb.WriteString(fmt.Sprintf(" - %s\n", tag.Hex()))
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
	sb.WriteString(fmt.Sprintf("Description: %s\n", a.Description))
	sb.WriteString(fmt.Sprintf("Source Author: %s\n", a.SourceAuthor))
	sb.WriteString(fmt.Sprintf("SourceID: %s\n", a.SourceID))
	sb.WriteString(fmt.Sprintf("AuthorID: %s\n", a.AuthorID))
	sb.WriteString(fmt.Sprintf("Date: %s\n", a.Date.Format(time.RFC3339)))

	sb.WriteString("Tags:\n")
	for _, tag := range a.Tags {
		if tag.EN != "" {
			sb.WriteString(fmt.Sprintf("   - %s (%s)\n", tag.EN, tag.Default))
		} else {
			sb.WriteString(fmt.Sprintf("   - %s\n", tag.Default))
		}
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
