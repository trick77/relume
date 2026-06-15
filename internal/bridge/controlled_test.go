package bridge

import (
	"sort"
	"testing"
	"time"
)

func TestControlledSet_dropsLightOnlyWhenOthersRemain(t *testing.T) {
	// Given: a 1-minute window with a controllable clock
	now := time.Unix(0, 0)
	s := NewControlledSet(time.Minute)
	s.now = func() time.Time { return now }

	// When: two lights are driven 30s apart
	s.Seen("uuid-a")
	now = now.Add(30 * time.Second)
	s.Seen("uuid-b")

	// Then: within the window both are present
	if got := sortedCurrent(s); len(got) != 2 {
		t.Fatalf("within window = %v, want both", got)
	}

	// When: 40s pass — uuid-a is now 70s old (aged out), uuid-b 40s old (fresh)
	now = now.Add(40 * time.Second)

	// Then: uuid-a drops, but ONLY because uuid-b remains (set stays non-empty)
	got := sortedCurrent(s)
	if len(got) != 1 || got[0] != "uuid-b" {
		t.Fatalf("after partial expiry = %v, want [uuid-b]", got)
	}
}

func TestControlledSet_retainsLastSetWhenWindowEmpties(t *testing.T) {
	// Given: a set that has had lights
	now := time.Unix(0, 0)
	s := NewControlledSet(time.Minute)
	s.now = func() time.Time { return now }
	s.Seen("uuid-a")
	s.Seen("uuid-b")
	if got := sortedCurrent(s); len(got) != 2 {
		t.Fatalf("initial = %v, want both", got)
	}

	// When: the TV goes fully silent and the whole window ages out
	now = now.Add(5 * time.Minute)

	// Then: we NEVER report empty — the last non-empty set is retained
	if got := sortedCurrent(s); len(got) != 2 || got[0] != "uuid-a" || got[1] != "uuid-b" {
		t.Fatalf("after full expiry = %v, want retained [uuid-a uuid-b]", got)
	}

	// And: a new non-empty session replaces the retained set
	s.Seen("uuid-c")
	if got := sortedCurrent(s); len(got) != 1 || got[0] != "uuid-c" {
		t.Fatalf("after new session = %v, want [uuid-c]", got)
	}
}

func TestControlledSet_emptyOnlyBeforeAnyLightDriven(t *testing.T) {
	s := NewControlledSet(time.Minute)
	if got := s.Current(); len(got) != 0 {
		t.Fatalf("expected empty set before any drive, got %v", got)
	}
}

func TestControlledSet_ignoresEmptyUUID(t *testing.T) {
	s := NewControlledSet(time.Minute)
	s.Seen("")
	if got := s.Current(); len(got) != 0 {
		t.Fatalf("empty uuid should be ignored, got %v", got)
	}
}

func sortedCurrent(s *ControlledSet) []string {
	got := s.Current()
	sort.Strings(got)
	return got
}
