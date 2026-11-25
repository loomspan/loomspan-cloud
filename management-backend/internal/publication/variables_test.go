package publication

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestVariableCorpus(t *testing.T) {
	root := "testdata/framework-variables"
	cases, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			dir := filepath.Join(root, c.Name())
			var expected struct {
				Documents   []struct{ Source, Kind string }
				Names       []string
				Occurrences []VariableOccurrence
				Failure     struct {
					Phase, Reason string
					Locations     []struct{ Source, Path string }
				}
			}
			if err := json.Unmarshal(readFixture(t, filepath.Join(dir, "expected.json")), &expected); err != nil {
				t.Fatal(err)
			}
			var docs []variableDocument
			for _, d := range expected.Documents {
				docs = append(docs, variableDocument{d.Source, readFixture(t, filepath.Join(dir, d.Source)), d.Kind == "execution"})
			}
			found, err := discoverVariables(docs)
			if expected.Failure.Phase == "discovery" {
				location := expected.Failure.Locations[0]
				if len(err) != 1 || err[0] != (VariableIssue{expected.Failure.Reason, location.Source, location.Path}) {
					t.Fatalf("unexpected discovery failure: %v", err)
				}
				return
			}
			if err != nil || !slices.Equal(found.Names, expected.Names) || !slices.Equal(found.Occurrences, expected.Occurrences) {
				t.Fatalf("discovery: %+v, %v", found, err)
			}
		})
	}
}

func TestVariableDiscovery(t *testing.T) {
	t.Run("aliases keep the use path and merges are not synthesized", func(t *testing.T) {
		authored := []byte("executor: host:echo\ninput: &literal {$var: threshold}\nmerged: &merge {executor: hidden}\n" +
			"config:\n  first: *literal\n  second: [*literal]\n  <<: *merge\n  '<<': {$var: long}\n")
		found, err := discoverVariables([]variableDocument{{"test.yaml", authored, false}})
		want := []VariableOccurrence{
			{"threshold", "test.yaml", "config.first"},
			{"threshold", "test.yaml", "config.second[0]"},
			{"long", "test.yaml", `config["<<"]`},
		}
		if err != nil || !slices.Equal(found.Occurrences, want) {
			t.Fatalf("got %+v, %v", found, err)
		}
		excluded := []byte("defaults: &defaults {executor: example}\n<<: *defaults\nconfig: {$var: ignored}\n")
		found, err = discoverVariables([]variableDocument{{"test.yaml", excluded, false}})
		if err != nil || len(found.Names) != 0 {
			t.Fatalf("synthetic declaration selected config: %+v, %v", found, err)
		}
	})

	t.Run("parser-specific scalar resolution", func(t *testing.T) {
		for _, tc := range []struct{ scalar, name, reason string }{
			{"2026-11-24", "2026-11-24", ""},
			{"'2026-11-24'", "2026-11-24", ""},
			{"!!timestamp 2026-11-24", "", "name-must-be-string"},
			{"1_000", "1_000", ""},
			{"08", "", "name-must-be-string"},
			{"+0xF", "+0xF", ""},
			{"0o10", "", "name-must-be-string"},
			{"1e-30", "", "name-must-be-string"},
			{"!!str 1e-30", "1e-30", ""},
			{"! 123", "123", ""},
			{"&anchor ! 123", "123", ""},
			{"! &anchor 123", "123", ""},
			{"&anchor # authored\n      ! 123", "123", ""},
			{"~", "", "name-must-be-string"},
			{"<<", "", "name-must-be-string"},
			{"True", "", "name-must-be-string"},
			{"'~'", "~", ""},
			{`"\x85"`, "\u0085", ""},
			{`"\x1fX"`, "", "padded-name"},
			{"${APP:-default}", "", "name-must-be-string"},
			{"'${APP:-default}'", "${APP:-default}", ""},
		} {
			t.Run(tc.scalar, func(t *testing.T) {
				doc := variableDocument{"skill.yaml", []byte("executor: demo\nconfig:\n  marker:\n    $var: " + tc.scalar + "\n"), false}
				found, err := discoverVariables([]variableDocument{doc})
				if tc.reason == "" {
					if err != nil || !slices.Equal(found.Names, []string{tc.name}) {
						t.Fatalf("got %+v, %v", found, err)
					}
					return
				}
				if len(err) != 1 || err[0] != (VariableIssue{tc.reason, "skill.yaml", "config.marker"}) {
					t.Fatalf("got %v", err)
				}
			})
		}
	})

	t.Run("authored path escapes match Framework", func(t *testing.T) {
		authored := "executor: demo\nconfig:\n  \"dot.<\\t\\b\\f\\n\\r\\x01\\\\\\\">\": {$var: x}\n  2026-11-24: {$var: y}\n"
		found, err := discoverVariables([]variableDocument{{"skill.yaml", []byte(authored), false}})
		want := []VariableOccurrence{
			{"x", "skill.yaml", `config["dot.<\t\b\f\n\r\u0001\\\">"]`},
			{"y", "skill.yaml", `config["2026-11-24"]`},
		}
		if err != nil || !slices.Equal(found.Occurrences, want) {
			t.Fatalf("got %+v, %v", found, err)
		}
	})

	t.Run("recursive and expanded aliases are bounded", func(t *testing.T) {
		recursive := []byte("executor: demo\nconfig: &recursive {value: *recursive}\n")
		var expanded strings.Builder
		expanded.WriteString("executor: demo\nconfig:\n  a: &a [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]\n")
		for name := 'b'; name <= 'h'; name++ {
			expanded.WriteString("  " + string(name) + ": &" + string(name) + " [")
			expanded.WriteString(strings.Repeat("*"+string(name-1)+", ", 10))
			expanded.WriteString("0]\n")
		}
		for _, tc := range []struct {
			data   []byte
			reason string
		}{{recursive, "recursive-alias"}, {[]byte(expanded.String()), "expanded-yaml-limit"}} {
			_, err := discoverVariables([]variableDocument{{"skill.yaml", tc.data, false}})
			if len(err) != 1 || err[0].Reason != tc.reason {
				t.Fatalf("got %v, want %s", err, tc.reason)
			}
		}
	})
}

func readFixture(t *testing.T, filename string) []byte {
	t.Helper()
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
