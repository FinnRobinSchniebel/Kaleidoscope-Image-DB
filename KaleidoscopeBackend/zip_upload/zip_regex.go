package zipupload

import (
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yeka/zip"
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
	Values   map[string]string
	FileType string
	Path     string
	File     *zip.File
}

func ValidateAndParseZip(zipPath string, folderTemplates []string, fileTemplate string, groupingLayer int) (map[string][]ParsedFolderInfo, error) {

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Build folder patterns
	folderPatterns := []*LayerPattern{}
	for _, t := range folderTemplates {
		newPattern, err := buildLayerPattern(t)
		if err != nil {
			return nil, err
		}
		folderPatterns = append(folderPatterns, newPattern)
	}

	// Build file pattern
	filePattern, err := buildLayerPattern(fileTemplate)
	if err != nil {
		return nil, err
	}

	results := map[string][]ParsedFolderInfo{}
	// seen := map[string]bool{}

	//for each zip (1)
	for _, fileFromZip := range reader.File {

		if fileFromZip.IsEncrypted() {
			return nil, fmt.Errorf("zip is password protected")
		}

		isDir := strings.HasSuffix(fileFromZip.Name, "/")
		if isDir {
			continue
		}

		zipBase := strings.TrimSuffix(filepath.Base(zipPath), ".zip")

		pathParts := strings.Split(strings.TrimSuffix(fileFromZip.Name, "/"), "/")

		// prepend zip name as level 0
		parts := append([]string{zipBase}, pathParts...)

		//log.Print(parts)

		//combine upto path part that everything gets grouped by
		var matchPath string
		for i := range groupingLayer + 1 {
			matchPath += parts[i]
			if i > groupingLayer-1 {
				matchPath += "/"
			}
		}

		if filepath.Ext(fileFromZip.Name) == ".txt" {
			currentPathContent := ParsedFolderInfo{Path: (fileFromZip.Name), FileType: filepath.Ext(fileFromZip.Name), File: fileFromZip}
			results[matchPath] = append(results[matchPath], currentPathContent)
			continue
		}

		RegexMatches := map[string]string{}

		for depth, PathPart := range parts {

			log.Print(PathPart)

			isFile := depth == len(parts)-1

			// Normalize PathPartName (strip extension for files)
			PathPartName := PathPart
			if isFile {
				PathPartName = strings.TrimSuffix(PathPartName, filepath.Ext(PathPartName))
			}

			if depth < len(folderPatterns) {
				log.Print(folderPatterns[depth].RawTemplate)
				err = MatchReg(folderPatterns[depth], PathPartName, RegexMatches)
			}
			if err != nil {
				return results, err
			}

			if isFile {
				err = MatchReg(filePattern, PathPartName, RegexMatches)
			}
		}

		currentPathContent := ParsedFolderInfo{Path: (fileFromZip.Name), Values: RegexMatches, FileType: filepath.Ext(fileFromZip.Name), File: fileFromZip}

		results[matchPath] = append(results[matchPath], currentPathContent)
	}
	for key := range results {
		slices.SortFunc(results[key], sortParsedInfo)

	}

	return results, nil
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
		result[field] = match[i+1]
	}

	return nil
}
