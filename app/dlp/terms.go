package dlp

import (
	"sort"
	"strings"
	"unicode"
)

// Terms are identifiers no pattern can recognise: host names, account names,
// names of people, customers or projects. They are proposed from outside
// (a local model, see the app's term detector) and are never trusted as
// given: AddTerms only accepts a term that passes ValidTerm and really
// occurs in the text. The replacement itself stays literal string
// replacement, exactly like the pattern based kinds.
//
// A term the mapping has learned is masked in every later text as well, so
// a host name found in the first message stays hidden in the tenth even if
// the detector misses it there.

const (
	KindHost Kind = "HOST"
	KindUser Kind = "USER"
	KindName Kind = "NAME"
)

const (
	minTermLen = 3
	maxTermLen = 64
	// MaxTermsPerCall bounds what one detector answer may add.
	MaxTermsPerCall = 40
)

// genericTerms are words a detector tends to return that identify nothing.
// Masking them would only make the text harder to work with.
var genericTerms = map[string]bool{
	"localhost": true, "root": true, "admin": true, "user": true, "users": true,
	"home": true, "host": true, "hostname": true, "server": true, "client": true,
	"sudo": true, "bash": true, "ssh": true, "ubuntu": true, "debian": true,
	"linux": true, "windows": true, "docker": true, "systemd": true, "nginx": true,
	"error": true, "warning": true, "info": true, "debug": true, "none": true,
	"null": true, "true": true, "false": true, "default": true, "test": true,
}

// ValidTerm reports whether term is acceptable as an identifier to mask.
func ValidTerm(term string) bool {
	if len(term) < minTermLen || len(term) > maxTermLen {
		return false
	}
	if strings.HasPrefix(term, placeholderPrefix) || genericTerms[strings.ToLower(term)] {
		return false
	}
	hasLetter := false
	for _, r := range term {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	if !hasLetter {
		return false
	}
	// The term must begin and end with a word character, or it could never
	// match on word boundaries.
	return isWordByte(term[0]) && isWordByte(term[len(term)-1])
}

func isWordByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' || b >= 0x80
}

// AddTerms registers the terms of kind that are valid and occur in text as
// whole words, and returns how many were new. Mask replaces them from then
// on. Terms are matched exactly, including case, so translating back
// restores the text byte for byte.
func (m *Mapping) AddTerms(kind Kind, terms []string, text string) int {
	added := 0
	for _, term := range terms {
		if added >= MaxTermsPerCall {
			break
		}
		term = strings.TrimSpace(term)
		if !ValidTerm(term) || m.terms[term] != "" {
			continue
		}
		if !containsWord(text, term) {
			continue
		}
		if m.terms == nil {
			m.terms = map[string]Kind{}
		}
		m.terms[term] = kind
		added++
	}
	return added
}

// maskTerms replaces every learned term in text, longest first so that
// "db01.corp.example" wins over "corp".
func (m *Mapping) maskTerms(text string) string {
	if len(m.terms) == 0 || text == "" {
		return text
	}
	ordered := make([]string, 0, len(m.terms))
	for term := range m.terms {
		ordered = append(ordered, term)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i]) != len(ordered[j]) {
			return len(ordered[i]) > len(ordered[j])
		}
		return ordered[i] < ordered[j]
	})
	for _, term := range ordered {
		kind := m.terms[term]
		text = replaceWord(text, term, func() string {
			m.counts[kind]++
			return m.token(kind, term, term)
		})
	}
	return text
}

func containsWord(text, term string) bool {
	found := false
	replaceWord(text, term, func() string { found = true; return term })
	return found
}

// replaceWord replaces every occurrence of term that is not glued to
// further word characters. Placeholders are never touched: a term cannot
// start with their prefix, and inside one it would be glued to "_".
func replaceWord(text, term string, replacement func() string) string {
	var out strings.Builder
	rest := text
	for {
		i := strings.Index(rest, term)
		if i < 0 {
			break
		}
		end := i + len(term)
		glued := (i > 0 && isWordByte(rest[i-1])) || (end < len(rest) && isWordByte(rest[end]))
		// The previous byte may already have been written out; look at it
		// there when the match starts the remaining text.
		if i == 0 && out.Len() > 0 {
			written := out.String()
			glued = glued || isWordByte(written[len(written)-1])
		}
		if glued {
			out.WriteString(rest[:i+1])
			rest = rest[i+1:]
			continue
		}
		out.WriteString(rest[:i])
		out.WriteString(replacement())
		rest = rest[end:]
	}
	out.WriteString(rest)
	return out.String()
}
