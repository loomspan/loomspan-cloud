package publication

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

type VariableOccurrence struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type VariableIssue struct {
	Reason string `json:"reason"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type variableDocument struct {
	source    string
	authored  []byte
	execution bool
}

type variableDiscovery struct {
	Names       []string
	Occurrences []VariableOccurrence
}

func discoverVariables(docs []variableDocument) (variableDiscovery, []VariableIssue) {
	var result variableDiscovery
	var failures []VariableIssue
	seen := make(map[string]bool)
	for _, doc := range docs {
		occurrences, issue := scanDocument(doc)
		if issue != nil {
			failures = append(failures, *issue)
		}
		for _, occurrence := range occurrences {
			result.Occurrences = append(result.Occurrences, occurrence)
			if !seen[occurrence.Name] {
				seen[occurrence.Name] = true
				result.Names = append(result.Names, occurrence.Name)
			}
		}
	}
	return result, failures
}

func scanDocument(doc variableDocument) ([]VariableOccurrence, *VariableIssue) {
	decoder := yaml.NewDecoder(bytes.NewReader(doc.authored))
	var parsed, extra yaml.Node
	if err := decoder.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return nil, &VariableIssue{Reason: "invalid-yaml", Source: doc.source}
	}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, &VariableIssue{Reason: "invalid-yaml", Source: doc.source}
	}
	if len(parsed.Content) == 0 {
		return nil, nil
	}
	root := parsed.Content[0]
	preserveNonSpecificTags(root, doc.authored)
	if _, err := expandedNodes(root, make(map[*yaml.Node]int)); err != nil {
		return nil, &VariableIssue{Reason: err.Error(), Source: doc.source}
	}
	var walker variableWalker
	walker.source = doc.source
	if doc.execution {
		walker.execution(field(root, "loomspan"))
	} else if field(root, "executor") != nil {
		if config := dereference(field(root, "config")); config != nil && config.Kind == yaml.MappingNode {
			walker.walk(config, "config")
		}
	}
	if walker.issue != nil {
		return walker.occurrences, walker.issue
	}
	return walker.occurrences, nil
}

const maxExpandedYAMLNodes = 1_000_000

func expandedNodes(node *yaml.Node, visited map[*yaml.Node]int) (int, error) {
	node = dereference(node)
	if node == nil || node.Kind == yaml.ScalarNode {
		return 1, nil
	}
	if size, ok := visited[node]; ok {
		if size < 0 {
			return 0, errors.New("recursive-alias")
		}
		return size, nil
	}
	visited[node] = -1
	count := 1
	for _, child := range node.Content {
		size, err := expandedNodes(child, visited)
		if err != nil {
			return 0, err
		}
		count += size
		if count > maxExpandedYAMLNodes {
			return 0, errors.New("expanded-yaml-limit")
		}
	}
	visited[node] = count
	return count, nil
}

type variableWalker struct {
	source      string
	occurrences []VariableOccurrence
	issue       *VariableIssue
}

func (w *variableWalker) execution(loomspan *yaml.Node) {
	loomspan = dereference(loomspan)
	if loomspan == nil || loomspan.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(loomspan.Content) && w.issue == nil; i += 2 {
		section, _ := yamlString(loomspan.Content[i])
		if section != "connections" && section != "executor-config" {
			continue
		}
		values := dereference(loomspan.Content[i+1])
		if values == nil || values.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(values.Content) && w.issue == nil; j += 2 {
			path := valuePath("loomspan."+section, values.Content[j], j/2)
			w.walk(values.Content[j+1], path)
		}
	}
}

func (w *variableWalker) walk(node *yaml.Node, path string) {
	node = dereference(node)
	if node == nil || w.issue != nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content) == 2 {
			if key, ok := yamlString(node.Content[0]); ok && key == "$var" {
				w.record(node.Content[1], path)
				return
			}
		}
		for i := 0; i < len(node.Content) && w.issue == nil; i += 2 {
			w.walk(node.Content[i], path+"{key#"+strconv.Itoa(i/2)+"}")
			w.walk(node.Content[i+1], valuePath(path, node.Content[i], i/2))
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if w.issue != nil {
				break
			}
			w.walk(child, path+"["+strconv.Itoa(i)+"]")
		}
	}
}

func (w *variableWalker) record(node *yaml.Node, path string) {
	name, ok := yamlString(node)
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)
	var reason string
	switch {
	case !ok:
		reason = "name-must-be-string"
	case blank(name):
		reason = "blank-name"
	case frameworkWhitespace(first) || frameworkWhitespace(last):
		reason = "padded-name"
	default:
		w.occurrences = append(w.occurrences, VariableOccurrence{Name: name, Source: w.source, Path: path})
		return
	}
	w.issue = &VariableIssue{Reason: reason, Source: w.source, Path: path}
}

func field(node *yaml.Node, name string) *yaml.Node {
	node = dereference(node)
	if node != nil && node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if key, ok := yamlString(node.Content[i]); ok && key == name {
				return node.Content[i+1]
			}
		}
	}
	return nil
}

func dereference(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode {
		return node.Alias
	}
	return node
}

var (
	coreNumber = regexp.MustCompile(`^(?:[-+]?(?:\.[0-9]+|[0-9]+(?:\.[0-9]*)?)(?:[eE][-+]?[0-9]+)?|0o[0-7]+|0x[0-9a-fA-F]+|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
	coreEnv    = regexp.MustCompile(`^\$\{[ \t\n\v\f\r]*(?:[A-Za-z0-9_]+(?::?[-?][A-Za-z0-9_]*)?)?[ \t\n\v\f\r]*\}$`)
	dotKey     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

func yamlString(node *yaml.Node) (string, bool) {
	node = dereference(node)
	if node == nil || node.Kind != yaml.ScalarNode {
		return "", false
	}
	if node.Style != 0 {
		return node.Value, node.ShortTag() == "!!str"
	}
	// Framework uses SnakeYAML Engine's Core schema. Go's implicit resolver
	// differs for timestamps, separated/signed numerics, and ENV scalar tags.
	switch node.Value {
	case "", "~", " ", "null", "Null", "NULL", "<<", "true", "True", "TRUE", "false", "False", "FALSE":
		return "", false
	}
	return node.Value, !coreNumber.MatchString(node.Value) && !coreEnv.MatchString(node.Value)
}

// The node decoder discards the explicit non-specific "!" tag on plain
// scalars, then applies its implicit resolver. Recover that tag from the
// parser's source marks so Framework still sees a string for "! 123".
func preserveNonSpecificTags(root *yaml.Node, authored []byte) {
	if !bytes.ContainsRune(authored, '!') {
		return
	}
	cursor := scalarSource{
		rest: bytes.TrimPrefix(authored, []byte{0xef, 0xbb, 0xbf}), line: 1, column: 1,
	}
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode && node.Style == 0 {
			prefix := cursor.at(node.Line, node.Column)
			if hasNonSpecificTag(prefix, node.Anchor) {
				node.Style, node.Tag = yaml.TaggedStyle, "!!str"
			}
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(root)
}

type scalarSource struct {
	rest         []byte
	line, column int
}

func (c *scalarSource) at(line, column int) []byte {
	for len(c.rest) > 0 && (c.line < line || c.line == line && c.column < column) {
		r, width := utf8.DecodeRune(c.rest)
		c.rest = c.rest[width:]
		if yamlBreak(r) {
			if r == '\r' && len(c.rest) > 0 && c.rest[0] == '\n' {
				c.rest = c.rest[1:]
			}
			c.line, c.column = c.line+1, 1
		} else {
			c.column++
		}
	}
	if c.line != line || c.column != column {
		return nil
	}
	return c.rest
}

func hasNonSpecificTag(prefix []byte, anchor string) bool {
	if anchor != "" && bytes.HasPrefix(prefix, []byte("&"+anchor)) {
		prefix = prefix[1+len(anchor):]
		for {
			prefix = bytes.TrimLeftFunc(prefix, yamlSeparation)
			if len(prefix) == 0 || prefix[0] != '#' {
				break
			}
			next := bytes.IndexFunc(prefix, yamlBreak)
			if next < 0 {
				return false
			}
			prefix = prefix[next:]
		}
	}
	if len(prefix) == 0 || prefix[0] != '!' {
		return false
	}
	next, width := utf8.DecodeRune(prefix[1:])
	return width == 0 || yamlSeparation(next)
}

func yamlSeparation(r rune) bool {
	return r == ' ' || r == '\t' || yamlBreak(r)
}

func yamlBreak(r rune) bool {
	return r == '\n' || r == '\r' || r == '\u0085' || r == '\u2028' || r == '\u2029'
}

func valuePath(path string, key *yaml.Node, index int) string {
	name, ok := yamlString(key)
	if !ok {
		return path + "{value#" + strconv.Itoa(index) + "}"
	}
	if dotKey.MatchString(name) {
		if path == "" {
			return name
		}
		return path + "." + name
	}
	return path + "[" + quotePath(name) + "]"
}

func quotePath(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, c := range value {
		switch c {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(c)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if c < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, c)
			} else {
				out.WriteRune(c)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

func frameworkWhitespace(r rune) bool {
	return r >= '\t' && r <= '\r' || r >= 0x1c && r <= 0x1f || unicode.Is(unicode.Z, r)
}

func blank(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !frameworkWhitespace(r) }) == -1
}

func variableValues(found variableDiscovery, saved map[string]string) (map[string]string, []VariableIssue) {
	values := make(map[string]string, len(found.Names))
	var problems []VariableIssue
	for _, occurrence := range found.Occurrences {
		value, exists := saved[occurrence.Name]
		switch {
		case !exists:
			problems = append(problems, VariableIssue{"missing-value", occurrence.Source, occurrence.Path})
		case blank(value):
			problems = append(problems, VariableIssue{"blank-value", occurrence.Source, occurrence.Path})
		default:
			values[occurrence.Name] = value
		}
	}
	return values, problems
}
