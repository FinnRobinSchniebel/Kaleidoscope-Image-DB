package zipupload

import (
	"Kaleidoscopedb/Backend/KaleidoscopeBackend/notification"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type LayerPattern struct {
	RawTemplate string
	Regex       *regexp.Regexp
	Fields      []string
}

func buildLayerPattern(template string) (*LayerPattern, error) {
	fields := []string{}

	reField := regexp.MustCompile(`\[[^\[\]]+\]`)

	regexStr := reField.ReplaceAllStringFunc(template, func(m string) string {
		field := m[1 : len(m)-1]
		fields = append(fields, field)
		return "___FIELD___"
	})

	// Now escape everything else
	regexStr = regexp.QuoteMeta(regexStr)

	// Replace placeholder with real capture group
	regexStr = strings.ReplaceAll(regexStr, "___FIELD___", "([^/]+)")

	re, err := regexp.Compile("^" + regexStr + "$")
	if err != nil {
		return nil, err
	}

	return &LayerPattern{
		RawTemplate: template,
		Regex:       re,
		Fields:      fields,
	}, nil
}

type ParsedFolderInfo struct {
	Values   map[string]string //parsed values [Key: Filed, value: extracted]
	FileType string            //file ending with dot (.png)
	Path     string            //relative to base extracted folder
	Conflict string            //set when a field was captured twice with different values; the file's group is skipped
}

// errFieldConflict marks a field captured more than once for one file with different values.
var errFieldConflict = errors.New("conflicting values")

// This function makes sure the parsing template and folder structure are the same, and parses it.
// Note: ParsedFolderInfo Path is relative to base file (root + Path for full)
// groupingLayer must be in [0, len(folderTemplates)] (checked by ProcessZip). Files that sit above
// it are returned in skipped rather than grouped.
func ValidateAndParseFolder(rootPath string, folderTemplates []string, fileTemplate string, groupingLayer int) (results map[string][]ParsedFolderInfo, skipped []notification.ItemResult, err error) {

	// Build folder patterns
	folderPatterns := []*LayerPattern{}
	for _, t := range folderTemplates {
		newPattern, err := buildLayerPattern(t)
		if err != nil {
			return nil, nil, err
		}
		folderPatterns = append(folderPatterns, newPattern)
	}

	// Build file pattern
	filePattern, err := buildLayerPattern(fileTemplate)
	if err != nil {
		return nil, nil, err
	}

	results = map[string][]ParsedFolderInfo{}
	// seen := map[string]bool{}

	//get folder itself as a seperate part to add later
	rootBase := filepath.Base(rootPath)

	//for each zip (1)
	err = filepath.WalkDir(rootPath, func(fullPath string, d os.DirEntry, err error) error {

		// if fileFromZip.IsEncrypted() {
		// 	return fmt.Errorf("zip is password protected")
		// }
		if err != nil {
			return err
		}
		if fullPath == rootPath {
			return nil
		}

		log.Printf("Full Path: %s\nRoot Path: %s\nRoot base: %s\n", fullPath, rootPath, rootBase)

		if d.IsDir() {
			return nil
		}

		//path starting from the base directory (not including base, added back using rootBase)
		relativePath, err := filepath.Rel(rootPath, fullPath)
		if err != nil {
			return err
		}
		log.Print(relativePath)

		pathParts := strings.Split(relativePath, string(os.PathSeparator))
		pathParts = append([]string{rootBase}, pathParts...)

		log.Print("Parts: ")
		log.Print(pathParts)

		//a file above the grouping layer belongs to no group; one at the grouping layer itself is a collapsed folder
		if len(pathParts) <= groupingLayer {
			skipped = append(skipped, notification.ItemResult{Kind: notification.ItemSkipped, Ref: relativePath, Reason: "above grouping level"})
			return nil
		}

		//combine up-to path part to create grouping key (sicne grouping Layer is an index, it is inclusive)
		var matchPath string
		for i := 0; i <= groupingLayer; i++ {
			matchPath += pathParts[i]
			if i < groupingLayer {
				matchPath += "/"
			}
		}

		RegexMatches := map[string]string{}
		var conflict string
		isTxt := filepath.Ext(relativePath) == ".txt"

		for depth, part := range pathParts {

			log.Print(part)

			isFile := depth == len(pathParts)-1

			//a description's own name isn't parsed; it only takes its folders' fields (e.g. Source, ID)
			if isFile && isTxt {
				break
			}

			// Normalize PathPartName (strip extension for files)
			PathPartName := part
			if isFile {
				PathPartName = strings.TrimSuffix(PathPartName, filepath.Ext(PathPartName))
			}

			//a file sitting where a folder belongs (collapsed) is named like that folder, so only its template applies
			pattern := filePattern
			if depth < len(folderPatterns) {
				pattern = folderPatterns[depth]
				log.Print("Folder part Template: " + pattern.RawTemplate)
			} else if !isFile {
				continue
			}

			matchErr := MatchReg(pattern, PathPartName, RegexMatches)
			if errors.Is(matchErr, errFieldConflict) {
				conflict = matchErr.Error()
				break
			}
			if matchErr != nil {
				return matchErr
			}
		}

		currentPathContent := ParsedFolderInfo{
			Path:     relativePath,
			Values:   RegexMatches,
			FileType: filepath.Ext(fullPath),
			Conflict: conflict,
		}

		results[matchPath] = append(results[matchPath], currentPathContent)
		return nil
	})
	if err != nil {
		return results, skipped, err
	}

	for key := range results {
		slices.SortFunc(results[key], sortParsedInfo)

	}

	return results, skipped, nil
}

func sortParsedInfo(a, b ParsedFolderInfo) int {

	orderA := a.Values["Order"]
	orderB := b.Values["Order"]
	reverse := false

	if orderA == "" && orderB == "" {
		orderA = a.Values["-Order"]
		orderB = b.Values["-Order"]
		reverse = true
	}
	if orderA == "" && orderB == "" {
		reverse = false
		orderA = a.Path
		orderB = b.Path
	}

	if orderA == orderB {
		return 0
	}

	if reverse {
		if orderA < orderB {
			return 1
		}
		return -1
	}

	if orderA < orderB {
		return -1
	}
	return 1
}

func MatchReg(pattern *LayerPattern, PathSeg string, result map[string]string) error {
	match := pattern.Regex.FindStringSubmatch(PathSeg)

	//if it fails to parse and the regex is not empty create an error
	if match == nil && pattern.RawTemplate != "" {
		return fmt.Errorf("no match: %s Template: %s", PathSeg, pattern.RawTemplate)
	}

	for i, field := range pattern.Fields {
		value := match[i+1]
		if old, ok := result[field]; ok && old != value {
			return fmt.Errorf("%w for field [%s]: %q vs %q", errFieldConflict, field, old, value)
		}
		result[field] = value
	}

	return nil
}
