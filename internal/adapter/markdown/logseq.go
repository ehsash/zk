package markdown

import (
	"regexp"
	"strings"

	"github.com/zk-org/zk/internal/util/opt"
)

// Logseq stores page-level metadata as `key:: value` lines in the first block
// of a page file, e.g.
//
//	title:: Renewable Energy
//	type:: [[concept]]
//	alias:: énergies renouvelables, Renewable Energy
//	tags:: [[energy]], [[climate]]
//
//	- # Renewable Energy
//
// This is neither a YAML frontmatter nor a heading, so without explicit support
// the title, tags and aliases of such a page are invisible to zk.

// logseqPropertyRegex matches a single page property line. The key is
// restricted to a word-like token so prose containing `::` is not mistaken for
// a property. The value is greedy, so `::` may appear inside it.
var logseqPropertyRegex = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*):: ?(.*)$`)

// logseqProperties holds the page properties found at the top of a note.
type logseqProperties struct {
	// raw maps a lowercased property key to its unparsed value.
	raw map[string]string
	// end is the offset at which the content after the property block starts.
	end int
}

// parseLogseqProperties extracts the leading `key:: value` block of a note.
//
// Only a block at the start of the file is a *page* property block. Logseq also
// allows `key:: value` inside any bullet, but those are block properties scoped
// to that bullet and must not be promoted to page metadata.
//
// Blank lines before the block are skipped, so a note may carry both a YAML
// frontmatter and Logseq properties.
func parseLogseqProperties(source []byte) logseqProperties {
	props := logseqProperties{raw: map[string]string{}, end: 0}

	offset := 0
	seenProperty := false
	for offset < len(source) {
		line := source[offset:]
		if i := indexOfNewline(line); i >= 0 {
			line = line[:i]
		}

		match := logseqPropertyRegex.FindSubmatch(line)
		if match == nil {
			// A blank line only ends the block once it has started; before
			// that it is separation from a preceding frontmatter.
			if !seenProperty && strings.TrimSpace(string(line)) == "" {
				offset += len(line)
				if offset < len(source) && source[offset] == '\r' {
					offset++
				}
				if offset < len(source) && source[offset] == '\n' {
					offset++
				}
				continue
			}
			break
		}
		seenProperty = true

		key := strings.ToLower(string(match[1]))
		props.raw[key] = strings.TrimSpace(string(match[2]))

		offset += len(line)
		// Consume the line break, if any.
		if offset < len(source) && source[offset] == '\r' {
			offset++
		}
		if offset < len(source) && source[offset] == '\n' {
			offset++
		}
		props.end = offset
	}

	return props
}

// indexOfNewline returns the offset of the next line break, or -1.
func indexOfNewline(source []byte) int {
	for i, c := range source {
		if c == '\n' || c == '\r' {
			return i
		}
	}
	return -1
}

// getString returns a single-valued property, e.g. `title::`. Commas are *not*
// separators here, since a title may legitimately contain one.
func (p logseqProperties) getString(key string) opt.String {
	value, ok := p.raw[strings.ToLower(key)]
	if !ok {
		return opt.NullString
	}
	return opt.NewNotEmptyString(unwrapLogseqValue(value))
}

// getStrings returns a multi-valued property, e.g. `tags::` or `alias::`,
// splitting on the commas Logseq itself treats as separators.
func (p logseqProperties) getStrings(key string) []string {
	values := []string{}

	raw, ok := p.raw[strings.ToLower(key)]
	if !ok {
		return values
	}

	for _, value := range splitLogseqList(raw) {
		if value = unwrapLogseqValue(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// splitLogseqList splits a property value on commas, ignoring those inside
// [[wiki brackets]] so a target such as `[[Carré, Jean-Baptiste]]` stays whole.
func splitLogseqList(value string) []string {
	parts := []string{}
	depth := 0
	current := strings.Builder{}

	for i := 0; i < len(value); i++ {
		switch {
		case strings.HasPrefix(value[i:], "[["):
			depth++
			current.WriteString("[[")
			i++
		case strings.HasPrefix(value[i:], "]]"):
			if depth > 0 {
				depth--
			}
			current.WriteString("]]")
			i++
		case value[i] == ',' && depth == 0:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(value[i])
		}
	}
	parts = append(parts, current.String())

	return parts
}

// unwrapLogseqValue normalizes a single property value by trimming whitespace
// and removing the [[…]] or # decoration, so that `[[energy]]`, `#energy` and
// `energy` all yield the same value.
func unwrapLogseqValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]]") && len(value) >= 4 {
		value = strings.TrimSpace(value[2 : len(value)-2])
	} else {
		value = strings.TrimPrefix(value, "#")
	}
	return strings.TrimSpace(value)
}

// logseqListProperties are the property keys whose value is a list. Every other
// key keeps its raw string value in the note metadata.
var logseqListProperties = []string{"tags", "alias", "aliases"}

// metadata returns the properties in the shape expected by NoteContent.Metadata:
// list-valued keys as string slices, everything else as a plain string.
func (p logseqProperties) metadata() map[string]any {
	metadata := map[string]any{}

	for key := range p.raw {
		if isLogseqListProperty(key) {
			metadata[key] = p.getStrings(key)
		} else if value := p.getString(key); !value.IsNull() {
			metadata[key] = value.String()
		}
	}

	return metadata
}

func isLogseqListProperty(key string) bool {
	for _, k := range logseqListProperties {
		if k == key {
			return true
		}
	}
	return false
}
