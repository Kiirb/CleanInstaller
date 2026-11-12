// Package vdf reads Valve's KeyValues formats: the text format used by
// libraryfolders.vdf, loginusers.vdf and appmanifest files, and the binary
// format used by shortcuts.vdf.
package vdf

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// KV is a text-VDF node: either a leaf (Value) or a block (Children).
type KV struct {
	Key      string
	Value    string
	Children []*KV
}

// Child returns the first child whose key matches case-insensitively, or nil.
// It is safe to call on a nil receiver, so lookups can be chained.
func (k *KV) Child(key string) *KV {
	if k == nil {
		return nil
	}
	for _, c := range k.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// Get walks children by key and returns the value of the leaf at the end.
func (k *KV) Get(path ...string) (string, bool) {
	n := k
	for _, p := range path {
		n = n.Child(p)
	}
	if n == nil || n == k || n.Children != nil {
		return "", false
	}
	return n.Value, true
}

type tokKind int

const (
	tokEOF tokKind = iota
	tokString
	tokOpen
	tokClose
	tokCond
)

type token struct {
	kind tokKind
	text string
}

type lexer struct {
	r    *bufio.Reader
	line int
}

func (l *lexer) next() (token, error) {
	for {
		c, _, err := l.r.ReadRune()
		if err == io.EOF {
			return token{kind: tokEOF}, nil
		}
		if err != nil {
			return token{}, err
		}
		switch {
		case c == '\n':
			l.line++
		case c == ' ' || c == '\t' || c == '\r' || c == 0xFEFF:
		case c == '/':
			n, _, err := l.r.ReadRune()
			if err != nil || n != '/' {
				return token{}, l.errf("unexpected '/'")
			}
			if _, err := l.r.ReadString('\n'); err != nil && err != io.EOF {
				return token{}, err
			}
			l.line++
		case c == '{':
			return token{kind: tokOpen}, nil
		case c == '}':
			return token{kind: tokClose}, nil
		case c == '[':
			s, err := l.r.ReadString(']')
			if err != nil {
				return token{}, l.errf("unterminated conditional")
			}
			return token{kind: tokCond, text: s}, nil
		case c == '"':
			s, err := l.quoted()
			if err != nil {
				return token{}, err
			}
			return token{kind: tokString, text: s}, nil
		default:
			// Unquoted word, ends at whitespace or a structural character.
			var b strings.Builder
			b.WriteRune(c)
			for {
				n, _, err := l.r.ReadRune()
				if err != nil {
					break
				}
				if strings.ContainsRune(" \t\r\n{}\"", n) {
					_ = l.r.UnreadRune()
					break
				}
				b.WriteRune(n)
			}
			return token{kind: tokString, text: b.String()}, nil
		}
	}
}

func (l *lexer) quoted() (string, error) {
	var b strings.Builder
	for {
		c, _, err := l.r.ReadRune()
		if err != nil {
			return "", l.errf("unterminated string")
		}
		switch c {
		case '"':
			return b.String(), nil
		case '\\':
			e, _, err := l.r.ReadRune()
			if err != nil {
				return "", l.errf("unterminated string")
			}
			switch e {
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			default: // \\ and \" and anything else: take literally
				b.WriteRune(e)
			}
		default:
			if c == '\n' {
				l.line++
			}
			b.WriteRune(c)
		}
	}
}

func (l *lexer) errf(format string, a ...any) error {
	return fmt.Errorf("vdf: line %d: %s", l.line+1, fmt.Sprintf(format, a...))
}

// ParseText parses a text VDF document. The returned root has an empty Key
// and holds the top-level nodes as Children.
func ParseText(r io.Reader) (*KV, error) {
	l := &lexer{r: bufio.NewReader(r)}
	root := &KV{Children: []*KV{}}
	if err := parseBlock(l, root, true); err != nil {
		return nil, err
	}
	return root, nil
}

func parseBlock(l *lexer, parent *KV, top bool) error {
	for {
		t, err := l.next()
		if err != nil {
			return err
		}
		switch t.kind {
		case tokEOF:
			if top {
				return nil
			}
			return l.errf("unexpected end of file, missing '}'")
		case tokClose:
			if top {
				return l.errf("unexpected '}'")
			}
			return nil
		case tokCond:
			continue
		case tokOpen:
			return l.errf("unexpected '{' without key")
		}
		node := &KV{Key: t.text}
		v, err := l.next()
		if err != nil {
			return err
		}
		switch v.kind {
		case tokString:
			node.Value = v.text
		case tokOpen:
			node.Children = []*KV{}
			if err := parseBlock(l, node, false); err != nil {
				return err
			}
		default:
			return l.errf("key %q has no value", t.text)
		}
		parent.Children = append(parent.Children, node)
	}
}

