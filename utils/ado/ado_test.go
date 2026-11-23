package ado

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMarshalMatrixIsSortedAndValid(t *testing.T) {
	got, err := MarshalMatrix(map[string]map[string]string{
		"b": {"X": "1", "Y": "2"},
		"a": {"X": "3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"X":"3"},"b":{"X":"1","Y":"2"}}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestMarshalMatrixRejectsBadNames(t *testing.T) {
	for _, m := range []map[string]map[string]string{
		{"bad-leg": {"A": "b"}},
		{"ok": {"bad name": "b"}},
		{"": {"A": "b"}},
		{"1leg": {"A": "b"}},
		{"_leg": {"A": "b"}},
		{strings.Repeat("a", 101): {"A": "b"}},
		{"ok": {"Secret_x": "b"}},
		{"ok": {"PaThToImage": "b"}},
	} {
		if _, err := MarshalMatrix(m); err == nil {
			t.Errorf("accepted %v", m)
		}
	}
}

func TestSetVariableEscapes(t *testing.T) {
	var buf bytes.Buffer
	if err := SetVariable(&buf, "matrix", "a%b\nc\rd", true); err != nil {
		t.Fatal(err)
	}
	want := "##vso[task.setvariable variable=matrix;isOutput=true]a%AZP25b%0Ac%0Dd\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
	if strings.Count(buf.String(), "\n") != 1 {
		t.Error("command spans multiple lines")
	}
}

func TestSetVariableRejectsBadName(t *testing.T) {
	if err := SetVariable(&bytes.Buffer{}, "a;b", "v", false); err == nil {
		t.Error("accepted a name that could inject properties")
	}
}

func TestMarshalMatrixAcceptsLongestLegName(t *testing.T) {
	if _, err := MarshalMatrix(map[string]map[string]string{strings.Repeat("a", 100): {"A": "b"}}); err != nil {
		t.Error(err)
	}
}

func TestSetVariableRejectsReservedPrefix(t *testing.T) {
	for _, n := range []string{"secret.x", "INPUT_a", "endpointUrl", "SecureFileName", "path", "PathToImage", "pAtH_cache"} {
		if err := SetVariable(&bytes.Buffer{}, n, "v", false); err == nil {
			t.Errorf("accepted %q", n)
		}
	}
}

func TestMarshalMatrixNormalizesNilMaps(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries map[string]map[string]string
		want    string
	}{
		{"nil matrix", nil, "{}"},
		{"empty matrix", map[string]map[string]string{}, "{}"},
		{"nil leg", map[string]map[string]string{"job": nil}, `{"job":{}}`},
		{"mixed legs", map[string]map[string]string{"job": nil, "vm_host": {"SCENARIO": "vm-host"}}, `{"job":{},"vm_host":{"SCENARIO":"vm-host"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalMatrix(tt.entries)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if original, ok := tt.entries["job"]; ok && original != nil {
				t.Error("encoding changed the caller's nil leg")
			}
		})
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestSetVariableReportsWriteError(t *testing.T) {
	if err := SetVariable(failWriter{}, "a", "v", false); err == nil {
		t.Error("write error swallowed")
	}
}
