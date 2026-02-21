package zipupload

import (
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
}

// This function makes sure the parsing template and folder structure are the same, and parses it.
// Note: ParsedFolderInfo Path is relative to base file (root + Path for full)
// IMPORTANT: No validation of groupinglayer being smaller then groupingLayer. That should have been done at API Validation
func ValidateAndParseFolder(rootPath string, folderTemplates []string, fileTemplate string, groupingLayer int) (map[string][]ParsedFolderInfo, error) {

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

		//combine up-to path part to create grouping key (sicne grouping Layer is an index, it is inclusive)
		//IMPORTANT: No validation of groupinglayer being smaller then groupingLayer. That should have been done at API input Validation
		var matchPath string
		for i := 0; i <= groupingLayer; i++ {
			matchPath += pathParts[i]
			if i < groupingLayer-1 {
				matchPath += "/"
			}
		}

		if filepath.Ext(relativePath) == ".txt" {
			currentPathContent := ParsedFolderInfo{
				Path:     relativePath,
				FileType: filepath.Ext(fullPath),
			}
			results[matchPath] = append(results[matchPath], currentPathContent)
			return nil
		}

		RegexMatches := map[string]string{}

		for depth, part := range pathParts {

			log.Print(part)

			isFile := depth == len(pathParts)-1

			// Normalize PathPartName (strip extension for files)
			PathPartName := part
			if isFile {
				PathPartName = strings.TrimSuffix(PathPartName, filepath.Ext(PathPartName))
			}

			if depth < len(folderPatterns) {
				log.Print("Folder part Template: " + folderPatterns[depth].RawTemplate)
				if err = MatchReg(folderPatterns[depth], PathPartName, RegexMatches); err != nil {
					return err
				}
			}

			if isFile {
				if err = MatchReg(filePattern, PathPartName, RegexMatches); err != nil {
					return err
				}
			}
		}

		currentPathContent := ParsedFolderInfo{
			Path:     relativePath,
			Values:   RegexMatches,
			FileType: filepath.Ext(fullPath),
		}

		results[matchPath] = append(results[matchPath], currentPathContent)
		return nil
	})
	if err != nil {
		return results, err
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
