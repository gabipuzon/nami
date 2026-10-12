package tsjsanalyzer

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
)

type config struct {
	dir, base string
	aliases   map[string][]string
	invalid   bool
}

func parseConfig(meta *metadata) (config, string) {
	c := config{}
	if meta == nil {
		return c, ""
	}
	c.dir = path.Dir(meta.Path)
	if meta.Error != "" {
		c.invalid = true
		return c, "configuration: " + meta.Error
	}
	var value struct {
		Extends         json.RawMessage
		References      json.RawMessage
		CompilerOptions struct {
			RootDirs       json.RawMessage
			ModuleSuffixes json.RawMessage
			BaseURL        string              `json:"baseUrl"`
			Paths          map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(meta.Value, &value); err != nil {
		c.invalid = true
		return c, "invalid configuration: " + err.Error()
	}
	if len(value.References) > 0 || len(value.CompilerOptions.RootDirs) > 0 || len(value.CompilerOptions.ModuleSuffixes) > 0 {
		c.invalid = true
		return c, "project references, rootDirs and moduleSuffixes resolution are unsupported"
	}
	if len(value.Extends) > 0 {
		c.invalid = true
		return c, "configuration extends is unsupported"
	}
	c.base = value.CompilerOptions.BaseURL
	if path.IsAbs(c.base) || strings.Contains(c.base, "\\") {
		c.invalid = true
	}
	c.aliases = value.CompilerOptions.Paths
	for key, targets := range c.aliases {
		if strings.Count(key, "*") > 1 {
			c.invalid = true
		}
		for _, target := range targets {
			if path.IsAbs(target) || strings.Contains(target, "\\") || strings.Count(target, "*") > 1 || strings.Contains(target, "*") && !strings.Contains(key, "*") {
				c.invalid = true
			}
		}
	}
	if c.invalid {
		return c, "complex configuration alias is unsupported"
	}
	return c, ""
}
func packageName(specifier string) string {
	parts := strings.Split(specifier, "/")
	if strings.HasPrefix(specifier, "@") && len(parts) > 1 {
		return strings.Join(parts[:2], "/")
	}
	return parts[0]
}
func parsePackage(meta *metadata) (map[string]bool, string) {
	result := map[string]bool{}
	if meta == nil {
		return result, ""
	}
	if meta.Error != "" {
		return result, "package metadata: " + meta.Error
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(meta.Value, &value); err != nil {
		return result, "invalid package metadata"
	}
	for _, key := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
		var dependencies map[string]json.RawMessage
		if raw, ok := value[key]; ok {
			if err := json.Unmarshal(raw, &dependencies); err != nil {
				return result, "invalid package dependency metadata"
			}
		}
		for name := range dependencies {
			result[name] = true
		}
	}
	if raw, ok := value["workspaces"]; ok && string(raw) != "null" {
		return result, "package workspaces are not resolved"
	}
	return result, ""
}

var extensions = []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

func candidates(base string, files map[string]bool) []string {
	base = path.Clean(base)
	if path.IsAbs(base) || base == ".." || strings.HasPrefix(base, "../") || strings.Contains(base, "\\") {
		return nil
	}
	ext := path.Ext(base)
	switch ext {
	case ".js", ".jsx", ".mjs", ".cjs":
		replacement := map[string][]string{".js": {".ts", ".tsx"}, ".jsx": {".tsx"}, ".mjs": {".mts"}, ".cjs": {".cts"}}[ext]
		var matches []string
		for _, suffix := range replacement {
			if candidate := strings.TrimSuffix(base, ext) + suffix; files[candidate] {
				matches = append(matches, candidate)
			}
		}
		if len(matches) > 0 {
			return matches
		}
	}
	if files[base] {
		return []string{base}
	}
	if ext != "" {
		return nil
	}
	for _, prefix := range []string{base, base + "/index"} {
		var matches []string
		for _, suffix := range extensions {
			if files[prefix+suffix] {
				matches = append(matches, prefix+suffix)
			}
		}
		if len(matches) > 0 {
			return matches
		}
	}
	return nil
}
func resolve(file, specifier string, c config, files map[string]bool) (string, bool, string) {
	if c.invalid {
		return "", true, "resolution unavailable for unsupported configuration"
	}
	bases := []string{}
	aliasMatched := false
	relative := strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
	if relative {
		bases = append(bases, path.Join(path.Dir(file), specifier))
	} else {
		if strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "#") || strings.Contains(specifier, ":") {
			return "", true, "unsupported absolute, package-import or URL specifier"
		}
		// Exact aliases take precedence, followed by the longest wildcard prefix.
		// Equal-prefix matching patterns stay ambiguous rather than relying on metadata order.
		keys := make([]string, 0, len(c.aliases))
		for key := range c.aliases {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := strings.Split(keys[i], "*"), strings.Split(keys[j], "*")
			if len(a) != len(b) {
				return len(a) < len(b)
			}
			if len(a[0]) != len(b[0]) {
				return len(a[0]) > len(b[0])
			}
			return keys[i] < keys[j]
		})

		selectedKey := ""
		for _, key := range keys {
			parts := strings.Split(key, "*")
			wildcard := ""
			if len(parts) == 1 {
				if key != specifier {
					continue
				}
			} else {
				if !strings.HasPrefix(specifier, parts[0]) || !strings.HasSuffix(specifier, parts[1]) || len(specifier) < len(parts[0])+len(parts[1]) {
					continue
				}
				wildcard = specifier[len(parts[0]) : len(specifier)-len(parts[1])]
			}
			if aliasMatched {
				if len(strings.Split(selectedKey, "*")[0]) == len(parts[0]) && strings.Contains(selectedKey, "*") && len(parts) == 2 {
					return "", true, "ambiguous matching alias patterns"
				}
				break
			}
			selectedKey = key
			aliasMatched = true
			for _, target := range c.aliases[key] {
				bases = append(bases, path.Join(c.dir, c.base, strings.ReplaceAll(target, "*", wildcard)))
			}
		}
		if !aliasMatched && c.base != "" {
			bases = append(bases, path.Join(c.dir, c.base, specifier))
		}
		if len(bases) == 0 {
			if aliasMatched {
				return "", true, "alias has no source targets"
			}
			return "", false, ""
		}
	}
	found := map[string]bool{}
	for _, base := range bases {
		for _, target := range candidates(base, files) {
			found[target] = true
		}
	}
	if len(found) == 1 {
		for target := range found {
			return target, true, ""
		}
	}
	if len(found) > 1 {
		return "", true, "ambiguous scanned source targets"
	}
	if !relative && !aliasMatched {
		return "", false, ""
	}
	return "", true, "no uniquely resolved scanned source target (missing, excluded or unsupported asset)"
}
