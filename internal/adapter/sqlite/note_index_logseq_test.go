package sqlite

import (
	"testing"

	"github.com/zk-org/zk/internal/core"
	"github.com/zk-org/zk/internal/util"
	"github.com/zk-org/zk/internal/util/test/assert"
)

// A link is resolved when its source is indexed, against the notes indexed so
// far; a link whose target is indexed LATER is re-targeted afterwards by
// batchFixExistingLinks. Both paths must resolve a Logseq link the same way,
// otherwise resolution silently depends on the order notes are walked in —
// which for a real notebook is alphabetical, so roughly half the links to any
// given note are lost.
func TestNoteIndexLogseqLinksResolveInBothOrders(t *testing.T) {
	target := core.Note{
		Path:     "pages/bravos-research___intermarket-sector-methodology.md",
		Title:    "Intermarket Sector Methodology",
		Metadata: map[string]any{"alias": []string{"ISM Framework"}},
	}

	// Every link form a Logseq page uses to reach that note.
	sourceFor := func(href string) core.Note {
		return core.Note{
			Path:  "pages/source.md",
			Title: "Source",
			Links: []core.Link{{
				Title: href,
				Href:  href,
				Type:  core.LinkTypeWikiLink,
			}},
		}
	}

	hrefs := []string{
		"bravos-research/intermarket-sector-methodology", // namespace form
		"Intermarket Sector Methodology",                 // title form
		"ISM Framework",                                  // alias form
	}

	for _, href := range hrefs {
		// Order A: target first, then the source. Resolved by FindIdsByHref
		// while indexing the source.
		t.Run("target-first/"+href, func(t *testing.T) {
			db, index := testNoteIndexLogseq(t)
			defer db.Close()
			assertLinkResolves(t, db, index, target, sourceFor(href), true)
		})

		// Order B: source first, then the target. The link is unresolved when
		// written, and must be re-targeted when the target lands.
		t.Run("source-first/"+href, func(t *testing.T) {
			db, index := testNoteIndexLogseq(t)
			defer db.Close()
			assertLinkResolves(t, db, index, target, sourceFor(href), false)
		})
	}
}

// The indexer does not fix links note by note: it adds every note with
// fixLinks=false and calls BatchUpdateLinks once at the end (see
// core.NoteIndex.IndexAll). Exercise that exact path, not just Add(_, true).
func TestNoteIndexLogseqBatchUpdateLinks(t *testing.T) {
	db, index := testNoteIndexLogseq(t)
	defer db.Close()

	source := core.Note{
		Path:  "pages/source.md",
		Title: "Source",
		Links: []core.Link{{
			Title: "bravos-research/intermarket-sector-methodology",
			Href:  "bravos-research/intermarket-sector-methodology",
			Type:  core.LinkTypeWikiLink,
		}},
	}
	target := core.Note{
		Path:  "pages/bravos-research___intermarket-sector-methodology.md",
		Title: "Intermarket Sector Methodology",
	}

	sourceID, err := index.Add(source, false)
	assert.Nil(t, err)
	targetID, err := index.Add(target, false)
	assert.Nil(t, err)

	err = index.BatchUpdateLinks(
		[]core.NoteID{sourceID, targetID},
		[]string{source.Path, target.Path},
	)
	assert.Nil(t, err)

	assertExistOrNot(t, db, true,
		"SELECT id FROM links WHERE href = ? AND target_id = ?",
		source.Links[0].Href, int64(targetID))
}

// Commit hands a wrapped index to its transaction. That index must carry every
// field of the one it wraps: a dropped field is silently replaced by its zero
// value, which for a bool disables the feature without any error. Indexing runs
// inside Commit, so this is the path a real `zk index` takes.
func TestNoteIndexCommitPreservesConfig(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	index := NewNoteIndex("/notebook", db, &util.NullLogger, "md", true)

	err := index.Commit(func(idx core.NoteIndex) error {
		inner, ok := idx.(*NoteIndex)
		assert.True(t, ok)
		assert.True(t, inner.logseqCompat)
		assert.Equal(t, inner.extension, "md")
		assert.Equal(t, inner.notebookPath, "/notebook")
		return nil
	})
	assert.Nil(t, err)
}

// A link must not be re-targeted onto an unrelated note.
func TestNoteIndexLogseqDoesNotOvermatch(t *testing.T) {
	db, index := testNoteIndexLogseq(t)
	defer db.Close()

	source := core.Note{
		Path:  "pages/source.md",
		Title: "Source",
		Links: []core.Link{{
			Title: "Some Absent Page",
			Href:  "Some Absent Page",
			Type:  core.LinkTypeWikiLink,
		}},
	}
	_, err := index.Add(source, true)
	assert.Nil(t, err)

	_, err = index.Add(core.Note{
		Path:  "pages/unrelated.md",
		Title: "Unrelated",
	}, true)
	assert.Nil(t, err)

	assertExistOrNot(t, db, false,
		"SELECT id FROM links WHERE href = ? AND target_id IS NOT NULL", "Some Absent Page")
}

// assertLinkResolves indexes target and source in the requested order and
// asserts the link ends up pointing at the target either way.
func assertLinkResolves(t *testing.T, db *DB, index *NoteIndex, target core.Note, source core.Note, targetFirst bool) {
	var targetID core.NoteID
	var err error

	if targetFirst {
		targetID, err = index.Add(target, true)
		assert.Nil(t, err)
		_, err = index.Add(source, true)
		assert.Nil(t, err)
	} else {
		_, err = index.Add(source, true)
		assert.Nil(t, err)
		targetID, err = index.Add(target, true)
		assert.Nil(t, err)
	}

	assertExistOrNot(t, db, true,
		"SELECT id FROM links WHERE href = ? AND target_id = ?",
		source.Links[0].Href, int64(targetID))
}

func testNoteIndexLogseq(t *testing.T) (*DB, *NoteIndex) {
	db := testDB(t)
	return db, NewNoteIndex("", db, &util.NullLogger, "md", true)
}
