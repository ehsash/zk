package sqlite

import (
	"testing"
	"time"

	"github.com/zk-org/zk/internal/core"
	"github.com/zk-org/zk/internal/util"
	"github.com/zk-org/zk/internal/util/opt"
	"github.com/zk-org/zk/internal/util/test/assert"
)

// logseqNotes is a small Logseq-shaped notebook: kebab-case filenames, a
// human-readable title, aliases in the metadata, and a namespaced page whose
// `/` is encoded as `___` in the filename.
type logseqNotes struct {
	renewable core.NoteID
	dashboard core.NoteID
	person    core.NoteID
}

func addLogseqNotes(t *testing.T, dao *NoteDAO) logseqNotes {
	modified := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	add := func(path string, title string, metadata map[string]any) core.NoteID {
		id, err := dao.Add(core.Note{
			Path:     path,
			Title:    title,
			Body:     "Body of " + title,
			Metadata: metadata,
			Modified: modified,
		})
		assert.Nil(t, err)
		return id
	}

	return logseqNotes{
		renewable: add("pages/renewable-energy.md", "Renewable Energy", map[string]any{
			"alias": []string{"énergies renouvelables", "Renewable Energy"},
		}),
		dashboard: add("pages/workspace___econ-dashboard.md", "Econ Dashboard", nil),
		person:    add("pages/jean-baptiste-carre.md", "Jean-Baptiste Carré", nil),
	}
}

func TestNoteDAOFindIdsByHrefLogseq(t *testing.T) {
	testNoteDAOLogseq(t, func(tx Transaction, dao *NoteDAO) {
		notes := addLogseqNotes(t, dao)

		// Links in a Logseq page carry no directory, e.g. [[renewable-energy]],
		// so they resolve as partial hrefs.
		test := func(href string, expected []core.NoteID) {
			actual, err := dao.FindIdsByHref(href, true)
			assert.Nil(t, err)
			assert.Equal(t, actual, expected)
		}

		// The filename still resolves.
		test("renewable-energy", []core.NoteID{notes.renewable})
		test("pages/renewable-energy", []core.NoteID{notes.renewable})

		// A link written in the readable title form, as Logseq resolves it.
		test("Renewable Energy", []core.NoteID{notes.renewable})

		// Title matching is case-insensitive.
		test("renewable ENERGY", []core.NoteID{notes.renewable})

		// A link written using one of the page's aliases.
		test("énergies renouvelables", []core.NoteID{notes.renewable})

		// A namespaced link: Logseq encodes `/` as `___` in the filename.
		test("workspace/econ-dashboard", []core.NoteID{notes.dashboard})

		// Accented titles resolve verbatim.
		test("Jean-Baptiste Carré", []core.NoteID{notes.person})

		// A target which exists nowhere stays unresolved: a Logseq forward
		// reference must not be coerced onto an unrelated page.
		test("web-clipping", []core.NoteID{})
		test("Some Absent Page", []core.NoteID{})
	})
}

// Without the notebook opting in, href resolution keeps stock behaviour.
func TestNoteDAOFindIdsByHrefLogseqDisabled(t *testing.T) {
	testNoteDAOWithFixtures(t, "", func(tx Transaction, dao *NoteDAO) {
		notes := addLogseqNotes(t, dao)

		test := func(href string, expected []core.NoteID) {
			actual, err := dao.FindIdsByHref(href, true)
			assert.Nil(t, err)
			assert.Equal(t, actual, expected)
		}

		// The filename still resolves.
		test("renewable-energy", []core.NoteID{notes.renewable})

		// But none of the Logseq-specific forms do.
		test("Renewable Energy", []core.NoteID{})
		test("énergies renouvelables", []core.NoteID{})
		test("workspace/econ-dashboard", []core.NoteID{})
	})
}

// An exact filename must always outrank a title.
func TestNoteDAOFindIdsByHrefLogseqPrecedence(t *testing.T) {
	testNoteDAOLogseq(t, func(tx Transaction, dao *NoteDAO) {
		modified := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

		// `decoy.md` is titled "target", while `target.md` is another page.
		_, err := dao.Add(core.Note{
			Path: "pages/decoy.md", Title: "pages/target", Body: "decoy", Modified: modified,
		})
		assert.Nil(t, err)
		target, err := dao.Add(core.Note{
			Path: "pages/target.md", Title: "Not The Decoy", Body: "target", Modified: modified,
		})
		assert.Nil(t, err)

		ids, err := dao.FindIdsByHref("pages/target", false)
		assert.Nil(t, err)
		assert.Equal(t, ids, []core.NoteID{target})
	})
}

func testNoteDAOLogseq(t *testing.T, callback func(tx Transaction, dao *NoteDAO)) {
	testTransactionWithFixtures(t, opt.NewNotEmptyString(""), func(tx Transaction) {
		callback(tx, NewNoteDAO(tx, &util.NullLogger, "md", true))
	})
}
