package scanner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const ignoreFile = ".namiignore"

type ignoreRule struct {
	pattern       string
	line          int
	include       bool
	directoryOnly bool
	expression    *regexp.Regexp
}

func loadIgnore(root string) ([]ignoreRule, error) {
	filename := filepath.Join(root, ignoreFile)
	info, err := os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ignoreFile, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("read %s: must be a regular file", ignoreFile)
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ignoreFile, err)
	}
	if !utf8.Valid(content) || strings.ContainsRune(string(content), 0) {
		return nil, fmt.Errorf("read %s: expected UTF-8 text without NUL bytes", ignoreFile)
	}
	return parseIgnore(strings.TrimPrefix(string(content), "\ufeff"))
}

func parseIgnore(content string) ([]ignoreRule, error) {
	var rules []ignoreRule
	for index, raw := range strings.Split(content, "\n") {
		line := trimIgnoreSpaces(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := ignoreRule{pattern: line, line: index + 1}
		if line[0] == '!' {
			rule.include = true
			line = line[1:]
		}
		line = unescapeSeparators(line)
		anchored := strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		rule.directoryOnly = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if line == "" {
			return nil, fmt.Errorf("%s:%d: empty pattern", ignoreFile, index+1)
		}
		expression, err := compileIgnore(line, anchored || strings.Contains(line, "/"))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: invalid pattern %q: %w", ignoreFile, index+1, rule.pattern, err)
		}
		rule.expression = expression
		rules = append(rules, rule)
	}
	return rules, nil
}

// An escaped separator still separates path components. Preserve pairs of
// backslashes, which represent literal backslashes in a Unix file name.
func unescapeSeparators(pattern string) string {
	var normalized strings.Builder
	slashes := 0
	for _, character := range pattern {
		if character == '\\' {
			slashes++
			continue
		}
		count := slashes
		if character == '/' && count%2 == 1 {
			count--
		}
		normalized.WriteString(strings.Repeat("\\", count))
		slashes = 0
		normalized.WriteRune(character)
	}
	normalized.WriteString(strings.Repeat("\\", slashes))
	return normalized.String()
}

func trimIgnoreSpaces(line string) string {
	for strings.HasSuffix(line, " ") {
		slashes := 0
		for i := len(line) - 2; i >= 0 && line[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 1 {
			break
		}
		line = line[:len(line)-1]
	}
	return line
}

// Translate path segments rather than using filepath.Match: ** crosses path
// separators, while ordinary stars and question marks stay inside one segment.
func compileIgnore(pattern string, anchored bool) (*regexp.Regexp, error) {
	var expression strings.Builder
	if anchored {
		expression.WriteString("^")
	} else {
		expression.WriteString("(?:^|/)")
	}
	segments := strings.Split(pattern, "/")
	for index, segment := range segments {
		if segment == "**" && len(segments) > 1 {
			if index == len(segments)-1 {
				expression.WriteString("(?s:.*)")
			} else {
				expression.WriteString("(?:[^/]+/)*")
			}
			continue
		}
		glob, err := segmentGlob(segment)
		if err != nil {
			return nil, err
		}
		expression.WriteString(glob)
		if index < len(segments)-1 {
			expression.WriteByte('/')
		}
	}
	expression.WriteByte('$')
	return regexp.Compile(expression.String())
}

func segmentGlob(segment string) (string, error) {
	runes := []rune(segment)
	var expression strings.Builder
	for index := 0; index < len(runes); index++ {
		switch runes[index] {
		case '*':
			expression.WriteString("[^/]*")
		case '?':
			expression.WriteString("[^/]")
		case '\\':
			index++
			if index == len(runes) {
				return "", fmt.Errorf("trailing backslash")
			}
			expression.WriteString(regexp.QuoteMeta(string(runes[index])))
		case '[':
			class, end, err := ignoreClass(runes, index)
			if err != nil {
				return "", err
			}
			expression.WriteString(class)
			index = end
		default:
			expression.WriteString(regexp.QuoteMeta(string(runes[index])))
		}
	}
	return expression.String(), nil
}

func ignoreClass(runes []rune, start int) (string, int, error) {
	var class strings.Builder
	class.WriteByte('[')
	index := start + 1
	if index < len(runes) && (runes[index] == '!' || runes[index] == '^') {
		class.WriteByte('^')
		class.WriteByte('/')
		index++
	}
	// A closing bracket immediately after the opening/negation is a literal.
	if index < len(runes) && runes[index] == ']' {
		class.WriteString("\\]")
		index++
	}
	for ; index < len(runes); index++ {
		switch runes[index] {
		case ']':
			class.WriteByte(']')
			return class.String(), index, nil
		case '\\':
			index++
			if index == len(runes) {
				return "", 0, fmt.Errorf("unclosed character class")
			}
			if strings.ContainsRune(`[]^-\`, runes[index]) {
				class.WriteByte('\\')
			}
			class.WriteRune(runes[index])
		case '[':
			if index+1 < len(runes) && runes[index+1] == ':' {
				end := index + 2
				for end+1 < len(runes) && !(runes[end] == ':' && runes[end+1] == ']') {
					end++
				}
				if end+1 == len(runes) {
					return "", 0, fmt.Errorf("unclosed POSIX character class")
				}
				class.WriteString(string(runes[index : end+2]))
				index = end + 1
			} else {
				class.WriteString("\\[")
			}
		default:
			class.WriteRune(runes[index])
		}
	}
	return "", 0, fmt.Errorf("unclosed character class")
}

func exclusionFor(rules []ignoreRule, relative string, directory bool) *ignoreRule {
	var match *ignoreRule
	for index := range rules {
		rule := &rules[index]
		if rule.directoryOnly && !directory {
			continue
		}
		if !rule.expression.MatchString(relative) {
			continue
		}
		if rule.include {
			match = nil
		} else {
			match = rule
		}
	}
	return match
}
