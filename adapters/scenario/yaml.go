// Package scenario decodes a bounded, strict YAML subset into pure scenario
// contracts. No database, environment credentials or providers are consulted.
package scenario

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"

	domain "github.com/tushardhara/dream/simulator/scenario"
	"go.yaml.in/yaml/v3"
)

const MaxDepth = 24
const MaxNodes = 20000

func problem(p, code, msg string) error { return &domain.Error{Path: p, Code: code, Message: msg} }
func Parse(r io.Reader) (domain.Scenario, error) {
	var out domain.Scenario
	b, err := io.ReadAll(io.LimitReader(r, domain.MaxBytes+1))
	if err != nil {
		return out, problem("$", "io", "cannot read input")
	}
	if len(b) > domain.MaxBytes {
		return out, problem("$", "limit", "YAML exceeds 1 MiB")
	}
	// Decoder builds at most a 1 MiB syntax tree. Alias expansion is never invoked;
	// the explicit traversal below rejects anchors/aliases and caps depth/nodes.
	d := yaml.NewDecoder(bytes.NewReader(b))
	var root yaml.Node
	if err = d.Decode(&root); err != nil {
		return out, problem("$", "syntax", "invalid YAML syntax")
	}
	var extra yaml.Node
	if d.Decode(&extra) != io.EOF {
		return out, problem("$", "document", "exactly one YAML document required")
	}
	if len(root.Content) != 1 {
		return out, problem("$", "document", "scenario document required")
	}
	count := 0
	if err = guard(root.Content[0], "$", 0, &count); err != nil {
		return out, err
	}
	if err = decode(root.Content[0], reflect.ValueOf(&out).Elem(), "$"); err != nil {
		return domain.Scenario{}, err
	}
	if _, err = out.Canonical(); err != nil {
		return domain.Scenario{}, err
	}
	return out, nil
}
func guard(n *yaml.Node, p string, depth int, count *int) error {
	*count++
	if depth > MaxDepth || *count > MaxNodes {
		return problem(p, "limit", "YAML depth or node limit exceeded")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return problem(p, "alias", "anchors and aliases are forbidden")
	}
	if n.Style&yaml.TaggedStyle != 0 {
		return problem(p, "tag", "explicit YAML tags are forbidden")
	}
	for i, c := range n.Content {
		if err := guard(c, fmt.Sprintf("%s[%d]", p, i), depth+1, count); err != nil {
			return err
		}
	}
	return nil
}
func decode(n *yaml.Node, v reflect.Value, p string) error {
	bad := func() error { return problem(p, "type", "expected "+v.Type().String()+" without YAML coercion") }
	if v.Kind() == reflect.Pointer {
		v.Set(reflect.New(v.Type().Elem()))
		return decode(n, v.Elem(), p)
	}
	switch v.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return bad()
		}
		fields := map[string]int{}
		required := map[string]bool{}
		for i := 0; i < v.NumField(); i++ {
			parts := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")
			fields[parts[0]] = i
			required[parts[0]] = len(parts) == 1
		}
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k, val := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return problem(p, "key", "mapping keys must be strings")
			}
			idx, ok := fields[k.Value]
			q := p + "." + k.Value
			if !ok {
				return problem(q, "unknown", "unknown field")
			}
			if seen[k.Value] {
				return problem(q, "duplicate", "duplicate field")
			}
			seen[k.Value] = true
			if err := decode(val, v.Field(idx), q); err != nil {
				return err
			}
		}
		for i := 0; i < v.NumField(); i++ {
			name := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
			if required[name] && !seen[name] {
				return problem(p+"."+name, "required", "required field missing")
			}
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return bad()
		}
		if len(n.Content) > domain.MaxItems {
			return problem(p, "limit", "too many items")
		}
		v.Set(reflect.MakeSlice(v.Type(), len(n.Content), len(n.Content)))
		for i, c := range n.Content {
			if err := decode(c, v.Index(i), fmt.Sprintf("%s[%d]", p, i)); err != nil {
				return err
			}
		}
	case reflect.String:
		if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
			return bad()
		}
		v.SetString(n.Value)
	case reflect.Int, reflect.Int64:
		if n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
			return bad()
		}
		x, err := strconv.ParseInt(n.Value, 10, v.Type().Bits())
		if err != nil {
			return bad()
		}
		v.SetInt(x)
	case reflect.Uint64:
		if n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
			return bad()
		}
		x, err := strconv.ParseUint(n.Value, 10, 64)
		if err != nil {
			return bad()
		}
		v.SetUint(x)
	case reflect.Float64:
		if n.Kind != yaml.ScalarNode || (n.Tag != "!!float" && n.Tag != "!!int") {
			return bad()
		}
		x, err := strconv.ParseFloat(n.Value, 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
			return bad()
		}
		v.SetFloat(x)
	default:
		return bad()
	}
	return nil
}
