package main

import (
	"strings"
	"testing"
)

func TestParseResponse_Plain(t *testing.T) {
	r, err := parseResponse(`{"summary":"all_pass","verdicts":[{"draft_id":"x","verdict":"PASS","issues":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != "all_pass" || len(r.Verdicts) != 1 || r.Verdicts[0].DraftID != "x" {
		t.Fatalf("bad parse: %#v", r)
	}
}

func TestParseResponse_FencedWithProse(t *testing.T) {
	in := "Here is my review:\n\n```json\n{\"summary\":\"some_fail\",\"verdicts\":[{\"draft_id\":\"a\",\"verdict\":\"FAIL\",\"issues\":[\"bad quote\"]}]}\n```\n\nLet me know.\n"
	r, err := parseResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != "some_fail" || r.Verdicts[0].Issues[0] != "bad quote" {
		t.Fatalf("bad parse: %#v", r)
	}
}

func TestParseResponse_NoJSON(t *testing.T) {
	if _, err := parseResponse("I'm sorry, I cannot help with that."); err == nil {
		t.Fatal("expected error")
	}
}

func TestMerge_AllPass(t *testing.T) {
	c := &reviewerResp{Summary: "all_pass", Verdicts: []verdict{
		{DraftID: "a", Verdict: "PASS", Issues: []string{}},
		{DraftID: "b", Verdict: "PASS", Issues: []string{}},
	}}
	x := &reviewerResp{Summary: "all_pass", Verdicts: []verdict{
		{DraftID: "a", Verdict: "PASS", Issues: []string{}},
		{DraftID: "b", Verdict: "PASS", Issues: []string{}},
	}}
	got := merge(c, x)
	if got.Summary != "all_pass" {
		t.Fatalf("want all_pass, got %s", got.Summary)
	}
	if len(got.Verdicts) != 2 {
		t.Fatalf("want 2 verdicts, got %d", len(got.Verdicts))
	}
}

func TestMerge_FailOR(t *testing.T) {
	c := &reviewerResp{Verdicts: []verdict{
		{DraftID: "a", Verdict: "PASS", Issues: []string{}},
		{DraftID: "b", Verdict: "FAIL", Issues: []string{"claude-only flag"}},
	}}
	x := &reviewerResp{Verdicts: []verdict{
		{DraftID: "a", Verdict: "FAIL", Issues: []string{"codex-only flag"}},
		{DraftID: "b", Verdict: "PASS", Issues: []string{}},
	}}
	got := merge(c, x)
	if got.Summary != "some_fail" {
		t.Fatalf("want some_fail, got %s", got.Summary)
	}
	if got.Verdicts[0].Verdict != "FAIL" || got.Verdicts[1].Verdict != "FAIL" {
		t.Fatalf("FAIL-OR not enforced: %#v", got.Verdicts)
	}
	if !strings.HasPrefix(got.Verdicts[0].Issues[0], "[codex]") {
		t.Fatalf("want [codex] prefix, got %q", got.Verdicts[0].Issues[0])
	}
	if !strings.HasPrefix(got.Verdicts[1].Issues[0], "[claude]") {
		t.Fatalf("want [claude] prefix, got %q", got.Verdicts[1].Issues[0])
	}
}

func TestMerge_DedupOverlap(t *testing.T) {
	c := &reviewerResp{Verdicts: []verdict{
		{DraftID: "a", Verdict: "FAIL", Issues: []string{"contains 'unverifiable claim about every leader'"}},
	}}
	x := &reviewerResp{Verdicts: []verdict{
		{DraftID: "a", Verdict: "FAIL", Issues: []string{"unverifiable claim about every leader: not in source"}},
	}}
	got := merge(c, x)
	if len(got.Verdicts[0].Issues) != 1 {
		t.Fatalf("want 1 deduped issue, got %d: %v", len(got.Verdicts[0].Issues), got.Verdicts[0].Issues)
	}
	if !strings.HasPrefix(got.Verdicts[0].Issues[0], "[both]") {
		t.Fatalf("want [both] prefix, got %q", got.Verdicts[0].Issues[0])
	}
}

func TestMerge_OnlyClaude(t *testing.T) {
	c := &reviewerResp{Verdicts: []verdict{
		{DraftID: "a", Verdict: "PASS", Issues: []string{}},
	}}
	got := merge(c, nil)
	if got.Summary != "all_pass" {
		t.Fatalf("want all_pass, got %s", got.Summary)
	}
	if len(got.Verdicts) != 1 {
		t.Fatalf("want 1 verdict, got %d", len(got.Verdicts))
	}
}

func TestIssueOverlaps(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"hello world is great", "hello world is great", true},
		{"unverifiable claim about leaders", "the unverifiable claim about leaders flagged", true},
		{"short", "differ", false},
		{"completely different topic A", "totally unrelated topic Z", false},
	}
	for _, tc := range cases {
		got := issueOverlaps(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("issueOverlaps(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
