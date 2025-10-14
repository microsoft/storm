package utils

import (
	"testing"
)

// StringFilter tests

func TestNewStringFilterFromSlice(t *testing.T) {
	t.Run("creates filter from empty slice", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		if filter == nil {
			t.Error("expected non-nil filter")
		}
		if !filter.emptyIsAny {
			t.Error("expected emptyIsAny to be true")
		}
		if len(filter.contents) != 0 {
			t.Errorf("expected empty contents, got %d items", len(filter.contents))
		}
	})

	t.Run("creates filter from slice with items", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item2", "item3"})
		if len(filter.contents) != 3 {
			t.Errorf("expected 3 items, got %d", len(filter.contents))
		}
		if !filter.contents["item1"] || !filter.contents["item2"] || !filter.contents["item3"] {
			t.Error("expected all items to be in filter")
		}
	})

	t.Run("handles duplicate items", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item1", "item2"})
		if len(filter.contents) != 2 {
			t.Errorf("expected 2 unique items, got %d", len(filter.contents))
		}
	})
}

func TestStringFilter_SetStrict(t *testing.T) {
	t.Run("sets emptyIsAny to false", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		filter.SetStrict()
		if filter.emptyIsAny {
			t.Error("expected emptyIsAny to be false after SetStrict")
		}
	})
}

func TestStringFilter_Match(t *testing.T) {
	t.Run("empty filter matches any when emptyIsAny is true", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		if !filter.Match("anything") {
			t.Error("expected empty filter to match anything")
		}
	})

	t.Run("empty filter matches nothing when strict", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		filter.SetStrict()
		if filter.Match("anything") {
			t.Error("expected strict empty filter to match nothing")
		}
	})

	t.Run("matches items in filter", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item2"})
		if !filter.Match("item1") {
			t.Error("expected to match item1")
		}
		if !filter.Match("item2") {
			t.Error("expected to match item2")
		}
	})

	t.Run("does not match items not in filter", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item2"})
		if filter.Match("item3") {
			t.Error("expected not to match item3")
		}
	})

	t.Run("case sensitive matching", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"Item1"})
		if filter.Match("item1") {
			t.Error("expected case-sensitive match to fail")
		}
		if !filter.Match("Item1") {
			t.Error("expected exact case match to succeed")
		}
	})
}

func TestStringFilter_MatchAny(t *testing.T) {
	t.Run("empty filter matches any list when emptyIsAny is true", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		if !filter.MatchAny([]string{"anything", "something"}) {
			t.Error("expected empty filter to match any list")
		}
	})

	t.Run("empty filter matches no list when strict", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{})
		filter.SetStrict()
		if filter.MatchAny([]string{"anything", "something"}) {
			t.Error("expected strict empty filter to match no list")
		}
	})

	t.Run("matches when at least one item in list", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item2"})
		if !filter.MatchAny([]string{"item3", "item1", "item4"}) {
			t.Error("expected to match when item1 is in list")
		}
	})

	t.Run("does not match when no items in list", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1", "item2"})
		if filter.MatchAny([]string{"item3", "item4"}) {
			t.Error("expected not to match when no items in list")
		}
	})

	t.Run("matches empty input list", func(t *testing.T) {
		filter := NewStringFilterFromSlice([]string{"item1"})
		if filter.MatchAny([]string{}) {
			t.Error("expected not to match empty input list")
		}
	})
}

// PathFilter tests

func TestNewPathFilterFromSlice(t *testing.T) {
	t.Run("creates non-recursive filter from empty slice", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		if filter == nil {
			t.Error("expected non-nil filter")
		}
		if !filter.emptyIsAny {
			t.Error("expected emptyIsAny to be true")
		}
		if filter.recursive {
			t.Error("expected recursive to be false")
		}
		if len(filter.contents) != 0 {
			t.Errorf("expected empty contents, got %d items", len(filter.contents))
		}
	})

	t.Run("creates recursive filter", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, true)
		if !filter.recursive {
			t.Error("expected recursive to be true")
		}
	})

	t.Run("creates filter from slice with paths", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"path/to/dir1", "path/to/dir2"}, false)
		if len(filter.contents) != 2 {
			t.Errorf("expected 2 items, got %d", len(filter.contents))
		}
	})
}

func TestPathFilter_SetStrict(t *testing.T) {
	t.Run("sets emptyIsAny to false", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		filter.SetStrict()
		if filter.emptyIsAny {
			t.Error("expected emptyIsAny to be false after SetStrict")
		}
	})
}

func TestPathFilter_Match_NonRecursive(t *testing.T) {
	t.Run("empty filter matches any when emptyIsAny is true", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		if !filter.Match("any/path") {
			t.Error("expected empty filter to match any path")
		}
	})

	t.Run("empty filter matches nothing when strict", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		filter.SetStrict()
		if filter.Match("any/path") {
			t.Error("expected strict empty filter to match nothing")
		}
	})

	t.Run("matches exact paths only", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, false)
		if !filter.Match("a/b/c") {
			t.Error("expected exact match")
		}
		if filter.Match("a/b/c/d") {
			t.Error("expected not to match child path in non-recursive mode")
		}
		if filter.Match("a/b") {
			t.Error("expected not to match parent path")
		}
	})
}

func TestPathFilter_Match_Recursive(t *testing.T) {
	t.Run("matches child paths", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, true)
		if !filter.Match("a/b/c") {
			t.Error("expected to match exact path")
		}
		if !filter.Match("a/b/c/d/e") {
			t.Error("expected to match child path")
		}
	})

	t.Run("does not match parent paths", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, true)
		if filter.Match("a/b") {
			t.Error("expected not to match parent path")
		}
		if filter.Match("a") {
			t.Error("expected not to match parent path")
		}
	})

	t.Run("does not match sibling paths", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, true)
		if filter.Match("a/b/d") {
			t.Error("expected not to match sibling path")
		}
	})

	t.Run("matches multiple base paths", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b", "x/y"}, true)
		if !filter.Match("a/b/c/d") {
			t.Error("expected to match child of first base")
		}
		if !filter.Match("x/y/z") {
			t.Error("expected to match child of second base")
		}
		if filter.Match("c/d/e") {
			t.Error("expected not to match unrelated path")
		}
	})
}

func TestPathFilter_MatchAny(t *testing.T) {
	t.Run("empty filter matches any list when emptyIsAny is true", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		if !filter.MatchAny([]string{"any/path", "another/path"}) {
			t.Error("expected empty filter to match any list")
		}
	})

	t.Run("empty filter matches no list when strict", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{}, false)
		filter.SetStrict()
		if filter.MatchAny([]string{"any/path", "another/path"}) {
			t.Error("expected strict empty filter to match no list")
		}
	})

	t.Run("matches when at least one path in list (non-recursive)", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, false)
		if !filter.MatchAny([]string{"x/y/z", "a/b/c", "d/e/f"}) {
			t.Error("expected to match when a/b/c is in list")
		}
	})

	t.Run("matches when at least one path in list (recursive)", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b"}, true)
		if !filter.MatchAny([]string{"x/y/z", "a/b/c/d", "d/e/f"}) {
			t.Error("expected to match when child of a/b is in list")
		}
	})

	t.Run("does not match when no paths in list", func(t *testing.T) {
		filter := NewPathFilterFromSlice([]string{"a/b/c"}, false)
		if filter.MatchAny([]string{"x/y/z", "d/e/f"}) {
			t.Error("expected not to match when no paths in list")
		}
	})
}

// pathIsBase tests

func TestPathIsBase(t *testing.T) {
	t.Run("base equals path", func(t *testing.T) {
		if !pathIsBase("a/b/c", "a/b/c") {
			t.Error("expected path to be base of itself")
		}
	})

	t.Run("base is ancestor of path", func(t *testing.T) {
		if !pathIsBase("a/b/c", "a/b/c/d/e") {
			t.Error("expected a/b/c to be base of a/b/c/d/e")
		}
	})

	t.Run("base is not parent of path", func(t *testing.T) {
		if pathIsBase("a/b/c", "a/b") {
			t.Error("expected a/b/c not to be base of a/b")
		}
	})

	t.Run("base is not related to path", func(t *testing.T) {
		if pathIsBase("a/b/z", "a/b/c/d") {
			t.Error("expected a/b/z not to be base of a/b/c/d")
		}
	})

	t.Run("base has common prefix but is not base", func(t *testing.T) {
		if pathIsBase("a/b/c", "a/b/cd/e") {
			t.Error("expected a/b/c not to be base of a/b/cd/e")
		}
	})

	t.Run("single component paths", func(t *testing.T) {
		if !pathIsBase("a", "a/b/c") {
			t.Error("expected a to be base of a/b/c")
		}
		if !pathIsBase("a", "a") {
			t.Error("expected a to be base of a")
		}
	})

	t.Run("empty base", func(t *testing.T) {
		if !pathIsBase("", "") {
			t.Error("expected empty string to be base of itself")
		}
	})

	t.Run("path is shorter than base", func(t *testing.T) {
		if pathIsBase("a/b/c/d", "a/b") {
			t.Error("expected longer base not to match shorter path")
		}
	})
}
