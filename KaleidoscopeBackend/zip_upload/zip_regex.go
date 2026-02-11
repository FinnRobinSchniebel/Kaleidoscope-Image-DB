package zipupload

import (
	"fmt"
	"log"
	"path/filepath"
	"regexp"
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

	regexStr := regexp.QuoteMeta(template)

	regexStr = regexp.MustCompile(`\\\[([^\]]+)\\\]`).ReplaceAllStringFunc(
		regexStr,
		func(m string) string {
			field := m[2 : len(m)-2] // strip \[ \]
			fields = append(fields, field)
			return `([^_/]+)`
		},
	)

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
	Values map[string]string
	Path   string
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
	for _, filePath := range reader.File {

		if filePath.IsEncrypted() {
			return nil, fmt.Errorf("zip is password protected")
		}

		isDir := strings.HasSuffix(filePath.Name, "/")
		if isDir {
			continue
		}

		zipBase := strings.TrimSuffix(filepath.Base(zipPath), ".zip")

		pathParts := strings.Split(strings.TrimSuffix(filePath.Name, "/"), "/")

		// prepend zip name as level 0
		parts := append([]string{zipBase}, pathParts...)

		log.Print(parts)

		//combine upto path part that everything gets grouped by
		var matchPath string
		for i := range groupingLayer + 1 {
			matchPath += parts[i]
			if i > groupingLayer-1 {
				matchPath += "/"
			}
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

		currentPathContent := ParsedFolderInfo{Path: (filePath.Name), Values: RegexMatches}

		results[matchPath] = append(results[matchPath], currentPathContent)
	}

	return results, nil
}

func MatchReg(pattern *LayerPattern, PathSeg string, result map[string]string) error {
	match := pattern.Regex.FindStringSubmatch(PathSeg)

	//if it fails to parse and the regex is not empty create an error
	if match == nil && pattern.RawTemplate != "" {
		return fmt.Errorf("no match")
	}

	for i, field := range pattern.Fields {
		result[field] = match[i+1]
	}

	return nil
}
