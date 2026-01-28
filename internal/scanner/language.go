package scanner

import (
	"sort"
)

// languageMapping maps file extensions to language names.
var languageMapping = map[string]string{
	".go":     "Go",
	".py":     "Python",
	".js":     "JavaScript",
	".jsx":    "JavaScript",
	".ts":     "TypeScript",
	".tsx":    "TypeScript",
	".java":   "Java",
	".kt":     "Kotlin",
	".rs":     "Rust",
	".rb":     "Ruby",
	".php":    "PHP",
	".c":      "C",
	".cpp":    "C++",
	".cc":     "C++",
	".h":      "C/C++ Header",
	".hpp":    "C++ Header",
	".cs":     "C#",
	".swift":  "Swift",
	".m":      "Objective-C",
	".scala":  "Scala",
	".clj":    "Clojure",
	".ex":     "Elixir",
	".exs":    "Elixir",
	".erl":    "Erlang",
	".hs":     "Haskell",
	".lua":    "Lua",
	".r":      "R",
	".jl":     "Julia",
	".pl":     "Perl",
	".sh":     "Shell",
	".bash":   "Shell",
	".zsh":    "Shell",
	".vue":    "Vue",
	".svelte": "Svelte",
}

// detectLanguages analyzes extension counts to determine primary languages.
func detectLanguages(extCounts map[string]int) []LanguageInfo {
	// Aggregate by language
	langCounts := make(map[string]int)
	langExtensions := make(map[string][]string)

	for ext, count := range extCounts {
		lang, ok := languageMapping[ext]
		if !ok {
			continue
		}
		langCounts[lang] += count
		langExtensions[lang] = append(langExtensions[lang], ext)
	}

	// Calculate total
	total := 0
	for _, count := range langCounts {
		total += count
	}

	if total == 0 {
		return nil
	}

	// Build language info list
	var languages []LanguageInfo
	for lang, count := range langCounts {
		languages = append(languages, LanguageInfo{
			Name:       lang,
			FileCount:  count,
			Percentage: float64(count) / float64(total) * 100,
			Extensions: langExtensions[lang],
		})
	}

	// Sort by file count descending
	sort.Slice(languages, func(i, j int) bool {
		return languages[i].FileCount > languages[j].FileCount
	})

	// Return top languages (those with >5% or top 5)
	var result []LanguageInfo
	for i, lang := range languages {
		if i >= 5 && lang.Percentage < 5 {
			break
		}
		result = append(result, lang)
	}

	return result
}

// PrimaryLanguage returns the most prevalent language name, or empty string if none.
func (p *ProjectInfo) PrimaryLanguage() string {
	if len(p.Languages) == 0 {
		return ""
	}
	return p.Languages[0].Name
}
