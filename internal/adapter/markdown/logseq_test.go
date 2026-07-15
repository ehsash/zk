package markdown

import (
	"testing"

	"github.com/zk-org/zk/internal/core"
	"github.com/zk-org/zk/internal/util/opt"
	"github.com/zk-org/zk/internal/util/test/assert"
)

func TestParseLogseqPropertiesBlock(t *testing.T) {
	test := func(source string, expected map[string]string) {
		props := parseLogseqProperties([]byte(source))
		assert.Equal(t, len(props.raw), len(expected))
		for key, value := range expected {
			assert.Equal(t, props.raw[key], value)
		}
	}

	// No properties.
	test("", map[string]string{})
	test("# A title", map[string]string{})
	test("Some prose", map[string]string{})

	// A single property.
	test("title:: A Page", map[string]string{"title": "A Page"})

	// Several properties.
	test("title:: A Page\ntype:: [[concept]]", map[string]string{
		"title": "A Page",
		"type":  "[[concept]]",
	})

	// An empty value is a property with an empty string, not an absent key.
	test("tags::", map[string]string{"tags": ""})
	test("tags:: ", map[string]string{"tags": ""})

	// The block ends at the first line which is not a property.
	test("title:: A Page\n- # A Page\ntype:: [[concept]]", map[string]string{
		"title": "A Page",
	})
	test("title:: A Page\n\ntype:: [[concept]]", map[string]string{
		"title": "A Page",
	})

	// Only a *leading* block is a page property block. Logseq writes page
	// properties in the first block of the file; anything later is a block
	// property belonging to a bullet.
	test("# A title\ntitle:: A Page", map[string]string{})

	// An indented `key:: value` is a block property, not a page property.
	test("  title:: A Page", map[string]string{})

	// A key must look like a key.
	test("not a property:: value", map[string]string{})
	test(":: value", map[string]string{})

	// `::` may appear in the value.
	test("source:: https://example.com/a::b", map[string]string{
		"source": "https://example.com/a::b",
	})

	// Keys are lowercased so lookups are stable.
	test("Title:: A Page", map[string]string{"title": "A Page"})
}

func TestParseLogseqPropertiesEnd(t *testing.T) {
	// `end` is the offset at which the body after the property block starts.
	props := parseLogseqProperties([]byte("title:: A Page\ntype:: [[concept]]\n- # A Page"))
	assert.Equal(t, string([]byte("title:: A Page\ntype:: [[concept]]\n- # A Page")[props.end:]), "- # A Page")

	props = parseLogseqProperties([]byte("# Not a property"))
	assert.Equal(t, props.end, 0)
}

func TestParseLogseqPropertyValues(t *testing.T) {
	test := func(source string, key string, expected []string) {
		props := parseLogseqProperties([]byte(source))
		assert.Equal(t, props.getStrings(key), expected)
	}

	// Comma-separated, as Logseq splits multi-value properties.
	test("tags:: energy, trading", "tags", []string{"energy", "trading"})

	// Surrounding [[wiki brackets]] are stripped.
	test("tags:: [[energy]], [[trading]]", "tags", []string{"energy", "trading"})

	// A leading # is stripped, so `#tag` and `[[tag]]` forms agree.
	test("tags:: #energy, #trading", "tags", []string{"energy", "trading"})

	// A comma *inside* brackets does not split the value.
	test("alias:: [[Carré, Jean-Baptiste]], JBC", "alias", []string{"Carré, Jean-Baptiste", "JBC"})

	// Empty and whitespace-only entries are dropped.
	test("tags::", "tags", []string{})
	test("tags:: energy, , trading", "tags", []string{"energy", "trading"})
	test("tags::   energy   ,  trading  ", "tags", []string{"energy", "trading"})

	// An absent key yields no values.
	test("title:: A Page", "tags", []string{})

	// Accents are preserved verbatim.
	test("alias:: énergies renouvelables", "alias", []string{"énergies renouvelables"})
}

func TestParseLogseqPropertyString(t *testing.T) {
	test := func(source string, key string, expected opt.String) {
		props := parseLogseqProperties([]byte(source))
		assert.Equal(t, props.getString(key), expected)
	}

	test("title:: A Page", "title", opt.NewString("A Page"))

	// A title is a plain string: a comma does not split it.
	test("title:: Foo, Bar", "title", opt.NewString("Foo, Bar"))

	// Surrounding brackets are stripped for single values too.
	test("type:: [[concept]]", "type", opt.NewString("concept"))

	// Absent or empty yields a null string.
	test("title:: A Page", "type", opt.NullString)
	test("title::", "title", opt.NullString)
}

// The parser must be inert unless the notebook opts in.
func TestLogseqPropertiesDisabled(t *testing.T) {
	source := "title:: A Page\ntags:: [[energy]]\n- # Heading"
	content := parseWithOptions(t, source, ParserOpts{
		HashtagEnabled:          true,
		LogseqPropertiesEnabled: false,
	})

	// Falls back to stock behaviour: title from the heading, no Logseq tags.
	assert.Equal(t, content.Title, opt.NewString("Heading"))
	assert.Equal(t, content.Tags, []string{})
	assert.Equal(t, len(content.Metadata), 0)
}

func TestLogseqTitle(t *testing.T) {
	test := func(source string, expected string) {
		content := parseLogseq(t, source)
		assert.Equal(t, content.Title, opt.NewNotEmptyString(expected))
	}

	// A page whose title lives only in `title::`.
	test("title:: Renewable Energy\ntags:: [[energy]]\n- [[energy]]", "Renewable Energy")

	// `title::` wins over a heading, matching Logseq: the property is the
	// explicit declaration, the heading is prose.
	test("title:: Renewable Energy\n- # Something Else", "Renewable Energy")

	// A YAML frontmatter still wins over `title::`, so notebooks mixing both
	// keep their existing behaviour.
	test("---\ntitle: From Frontmatter\n---\ntitle:: From Property", "From Frontmatter")

	// No `title::` falls back to the `- # Title` block a Logseq page carries.
	test("type:: [[concept]]\n- # A Heading", "A Heading")

	// Neither: no title.
	test("type:: [[concept]]\n- some prose", "")

	// A sub-heading is a section of the page, never its title. Logseq derives
	// the title of such a page from its filename instead, which the parser
	// cannot see, so it must report no title rather than a wrong one.
	test("type:: [[project]]\n- ## Problem Statement\n- ## Approach", "")
	test("type:: [[project]]\n- ### Deep Section", "")

	// An H1 further down still counts, since it is the page's title block.
	test("type:: [[project]]\n- ## Section\n- # The Title", "The Title")
}

func TestLogseqTitleKeepsHeadingOutOfBody(t *testing.T) {
	// When `title::` supplies the title and the page also carries the matching
	// `- # Title` block, the heading must still be stripped from the body, so
	// the lead stays useful for search snippets.
	content := parseLogseq(t, "title:: A Page\n- # A Page\n  - The real lead.")
	assert.Equal(t, content.Title, opt.NewString("A Page"))
	assert.Equal(t, content.Lead, opt.NewString("- The real lead."))

	// With no heading, the body starts right after the property block.
	content = parseLogseq(t, "title:: A Page\n- The real lead.")
	assert.Equal(t, content.Title, opt.NewString("A Page"))
	assert.Equal(t, content.Lead, opt.NewString("- The real lead."))
}

func TestLogseqTags(t *testing.T) {
	test := func(source string, expected []string) {
		content := parseLogseq(t, source)
		assert.Equal(t, content.Tags, expected)
	}

	test("tags:: [[energy]], [[trading]]", []string{"energy", "trading"})
	test("tags:: energy, trading", []string{"energy", "trading"})

	// An empty `tags::` yields no tags rather than one empty tag.
	test("tags::", []string{})

	// Logseq tags merge with inline #hashtags, deduplicated.
	test("tags:: [[energy]]\n- Some #trading prose", []string{"energy", "trading"})
	test("tags:: [[energy]]\n- Some #energy prose", []string{"energy"})
}

func TestLogseqMetadata(t *testing.T) {
	content := parseLogseq(t, "title:: A Page\ntype:: [[concept]]\nalias:: First, Second\ntags:: [[energy]]\ndate:: [[2022-11-17]]")

	// Single values are plain strings, with surrounding brackets stripped.
	assert.Equal(t, content.Metadata["title"], "A Page")
	assert.Equal(t, content.Metadata["type"], "concept")
	assert.Equal(t, content.Metadata["date"], "2022-11-17")

	// List-valued properties are string slices, so `alias` can be queried from
	// the metadata JSON when resolving links.
	assert.Equal(t, content.Metadata["alias"], []string{"First", "Second"})
	assert.Equal(t, content.Metadata["tags"], []string{"energy"})
}

func TestLogseqMetadataMergesWithFrontmatter(t *testing.T) {
	// A frontmatter key must not be clobbered by a Logseq property.
	content := parseLogseq(t, "---\nkeywords: [from-frontmatter]\n---\ntype:: [[concept]]")
	assert.Equal(t, content.Metadata["type"], "concept")
	assert.NotNil(t, content.Metadata["keywords"])
}

func parseLogseq(t *testing.T, source string) core.NoteContent {
	return parseWithOptions(t, source, ParserOpts{
		HashtagEnabled:          true,
		LogseqPropertiesEnabled: true,
	})
}
