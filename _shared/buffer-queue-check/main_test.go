package main

import (
	"reflect"
	"testing"
)

func TestNonEmptyKeywords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "foo,bar", []string{"foo", "bar"}},
		{"trailing comma drops empty", "newsletter,", []string{"newsletter"}},
		{"double comma drops empty", "a,,b", []string{"a", "b"}},
		{"whitespace-only drops", "a, ,b", []string{"a", "b"}},
		{"trims each", " foo , bar ", []string{"foo", "bar"}},
		{"empty flag → no keywords (must NOT match everything)", "", []string{}},
		{"only commas → no keywords", ",,,", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nonEmptyKeywords(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("nonEmptyKeywords(%q) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}
