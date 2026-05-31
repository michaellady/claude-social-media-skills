package main

import (
	"testing"
)

func TestFrontmatterBytes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
		want string
	}{
		{"well-formed", "---\ndate: x\n---\n# body", true, "date: x"},
		{"no-leading-fence", "date: x\n---\n", false, ""},
		{"unterminated", "---\ndate: x\nno closing fence", false, ""},
		{"empty", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := frontmatterBytes([]byte(c.in))
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && string(got) != c.want {
				t.Errorf("front = %q, want %q", string(got), c.want)
			}
		})
	}
}

func TestLoadLedger_CrossSurface(t *testing.T) {
	// Cross-surface file uses surface:, no evidence_video_ids/cohort.
	hs := loadLedger(fixture(t, "ledger-cross"), "cross-surface")
	if len(hs) != 2 {
		t.Fatalf("cross-surface hypotheses = %d, want 2", len(hs))
	}
	byID := map[string]ledgerHypothesis{}
	for _, h := range hs {
		if h.Ledger != "cross-surface" {
			t.Errorf("%s: ledger label = %q, want cross-surface", h.ID, h.Ledger)
		}
		if h.FileDate != "2026-05-24" {
			t.Errorf("%s: file date = %q, want 2026-05-24", h.ID, h.FileDate)
		}
		byID[h.ID] = h
	}
	h1, ok := byID["h-x-2026-05-24-1"]
	if !ok {
		t.Fatal("missing h-x-2026-05-24-1")
	}
	if h1.Surface != "format:carousel" {
		t.Errorf("surface = %q, want format:carousel", h1.Surface)
	}
	if h1.Cohort != "" {
		t.Errorf("cross-surface cohort = %q, want empty", h1.Cohort)
	}
	if h1.Verdict != "" {
		t.Errorf("verdict = %q, want empty (pending)", h1.Verdict)
	}
	h2 := byID["h-x-2026-05-24-2"]
	if h2.Verdict != "refute" {
		t.Errorf("h2 verdict = %q, want refute", h2.Verdict)
	}
}

func TestLoadLedger_YouTube(t *testing.T) {
	// YouTube files use cohort: + evidence_video_ids (which the loader ignores —
	// the dashboard renders cohort, not the evidence ids). Two dated files.
	hs := loadLedger(fixture(t, "ledger-yt"), "youtube")
	if len(hs) != 3 {
		t.Fatalf("youtube hypotheses = %d, want 3", len(hs))
	}
	for _, h := range hs {
		if h.Ledger != "youtube" {
			t.Errorf("%s: ledger label = %q, want youtube", h.ID, h.Ledger)
		}
		if h.Cohort == "" {
			t.Errorf("%s: youtube hypothesis should carry a cohort", h.ID)
		}
		if h.Surface != "" {
			t.Errorf("%s: youtube hypothesis should not carry a surface, got %q", h.ID, h.Surface)
		}
	}
}

func TestLoadLedger_MissingDir(t *testing.T) {
	if hs := loadLedger(fixture(t, "no-such-ledger-dir"), "youtube"); hs != nil {
		t.Errorf("missing ledger dir should yield nil, got %d entries", len(hs))
	}
}

func TestLoadAllLedgers_NewestFirst(t *testing.T) {
	t.Setenv("DASH_XSURFACE_LEDGER", fixture(t, "ledger-cross"))
	t.Setenv("DASH_YT_LEDGER", fixture(t, "ledger-yt"))
	hs := loadAllLedgers()
	if len(hs) != 5 {
		t.Fatalf("combined hypotheses = %d, want 5", len(hs))
	}
	// Stable sort, newest FileDate first.
	for i := 1; i < len(hs); i++ {
		if hs[i-1].FileDate < hs[i].FileDate {
			t.Errorf("not newest-first at %d: %q before %q", i, hs[i-1].FileDate, hs[i].FileDate)
		}
	}
	if hs[0].FileDate != "2026-05-25" {
		t.Errorf("newest = %q, want 2026-05-25", hs[0].FileDate)
	}
	if hs[len(hs)-1].FileDate != "2026-05-18" {
		t.Errorf("oldest = %q, want 2026-05-18", hs[len(hs)-1].FileDate)
	}
}

func TestLedgerSummary(t *testing.T) {
	t.Setenv("DASH_XSURFACE_LEDGER", fixture(t, "ledger-cross"))
	t.Setenv("DASH_YT_LEDGER", fixture(t, "ledger-yt"))
	sum := ledgerSummary()
	// 2 confirm, 1 refute, 0 inconclusive, 2 pending across both fixtures.
	if sum["total"] != 5 {
		t.Errorf("total = %v, want 5", sum["total"])
	}
	if sum["confirmed"] != 2 {
		t.Errorf("confirmed = %v, want 2", sum["confirmed"])
	}
	if sum["refuted"] != 1 {
		t.Errorf("refuted = %v, want 1", sum["refuted"])
	}
	if sum["pending"] != 2 {
		t.Errorf("pending = %v, want 2", sum["pending"])
	}
}
