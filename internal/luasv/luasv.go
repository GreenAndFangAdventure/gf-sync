// Package luasv parses World of Warcraft SavedVariables files.
//
// It understands ONLY Lua literal syntax: top-level `Name = <value>`
// assignments where a value is a string, number, boolean, nil, or a table
// constructor made of those. Nothing is ever executed: function calls,
// operators (other than a unary minus on a number literal), variable
// references and every other Lua construct are rejected with an error.
package luasv

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Limits protect against hostile or corrupted files.
type Limits struct {
	MaxBytes     int // whole input size
	MaxDepth     int // table nesting depth
	MaxStringLen int // a single string literal, after unescaping
	MaxNodes     int // total number of values (tables + scalars)
}

// DefaultLimits are generous for real SavedVariables but bounded.
var DefaultLimits = Limits{
	MaxBytes:     32 << 20,
	MaxDepth:     64,
	MaxStringLen: 1 << 20,
	MaxNodes:     2_000_000,
}

// Table is a parsed Lua table. Keys are split by type so that the
// array part can be recognised for JSON conversion.
type Table struct {
	Str map[string]any // string keys (and stringified bool/float keys)
	Int map[int64]any  // integer keys, including positional items
}

// Value types produced: string, int64, float64, bool, *Table.
// Lua nil values are dropped (a table entry set to nil does not exist).

// Get returns t[key] for a string key, or nil.
func (t *Table) Get(key string) any {
	if t == nil || t.Str == nil {
		return nil
	}
	return t.Str[key]
}

// Len returns the length of the sequence 1..n (border without holes).
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	n := 0
	for {
		if _, ok := t.Int[int64(n+1)]; !ok {
			return n
		}
		n++
	}
}

// IsArray reports whether the table holds only integer keys 1..n.
func (t *Table) IsArray() bool {
	return len(t.Str) == 0 && t.Len() == len(t.Int)
}

// Array returns the sequence part 1..n.
func (t *Table) Array() []any {
	n := t.Len()
	out := make([]any, n)
	for i := 0; i < n; i++ {
		out[i] = t.Int[int64(i+1)]
	}
	return out
}

// SyntaxError describes where parsing failed.
type SyntaxError struct {
	Line, Col int
	Msg       string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("luasv: line %d col %d: %s", e.Line, e.Col, e.Msg)
}

// ErrTooLarge is returned when the input exceeds Limits.MaxBytes.
var ErrTooLarge = errors.New("luasv: input too large")

// Parse parses a SavedVariables document into its top-level assignments.
func Parse(src []byte) (map[string]any, error) {
	return ParseWithLimits(src, DefaultLimits)
}

// ParseWithLimits is Parse with explicit limits.
func ParseWithLimits(src []byte, lim Limits) (map[string]any, error) {
	if lim.MaxBytes > 0 && len(src) > lim.MaxBytes {
		return nil, ErrTooLarge
	}
	p := &parser{src: src, lim: lim, line: 1}
	// UTF-8 BOM is tolerated.
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		p.pos = 3
	}
	out := map[string]any{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.eof() {
			return out, nil
		}
		if p.peek() == ';' {
			p.advance()
			continue
		}
		name, ok := p.ident()
		if !ok {
			return nil, p.errf("expected variable name")
		}
		if isReserved(name) {
			return nil, p.errf("unexpected keyword %q", name)
		}
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.eof() || p.peek() != '=' {
			return nil, p.errf("expected '=' after %q", name)
		}
		p.advance()
		v, err := p.value(0)
		if err != nil {
			return nil, err
		}
		if v == nil {
			delete(out, name)
		} else {
			out[name] = v
		}
	}
}

type parser struct {
	src   []byte
	pos   int
	line  int
	col   int
	lim   Limits
	nodes int
}

func (p *parser) eof() bool  { return p.pos >= len(p.src) }
func (p *parser) peek() byte { return p.src[p.pos] }
func (p *parser) peekAt(off int) byte {
	if p.pos+off < len(p.src) {
		return p.src[p.pos+off]
	}
	return 0
}

func (p *parser) advance() byte {
	c := p.src[p.pos]
	p.pos++
	if c == '\n' {
		p.line++
		p.col = 0
	} else {
		p.col++
	}
	return c
}

func (p *parser) errf(format string, a ...any) error {
	return &SyntaxError{Line: p.line, Col: p.col + 1, Msg: fmt.Sprintf(format, a...)}
}

func (p *parser) countNode() error {
	p.nodes++
	if p.lim.MaxNodes > 0 && p.nodes > p.lim.MaxNodes {
		return p.errf("too many values")
	}
	return nil
}

func (p *parser) skipSpace() error {
	for !p.eof() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v':
			p.advance()
		case c == '-' && p.peekAt(1) == '-':
			p.advance()
			p.advance()
			if !p.eof() && p.peek() == '[' {
				if level, ok := p.longBracketLevel(); ok {
					if _, err := p.longBody(level, false); err != nil {
						return err
					}
					continue
				}
			}
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
		default:
			return nil
		}
	}
	return nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentChar(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

func (p *parser) ident() (string, bool) {
	if p.eof() || !isIdentStart(p.peek()) {
		return "", false
	}
	start := p.pos
	for !p.eof() && isIdentChar(p.peek()) {
		p.advance()
	}
	return string(p.src[start:p.pos]), true
}

var reserved = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "goto": true,
	"if": true, "in": true, "local": true, "nil": true, "not": true, "or": true,
	"repeat": true, "return": true, "then": true, "true": true, "until": true,
	"while": true,
}

func isReserved(s string) bool { return reserved[s] }

// value parses one literal. A nil result means Lua nil.
func (p *parser) value(depth int) (any, error) {
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	if p.eof() {
		return nil, p.errf("unexpected end of input, expected a value")
	}
	if err := p.countNode(); err != nil {
		return nil, err
	}
	c := p.peek()
	switch {
	case c == '{':
		return p.table(depth + 1)
	case c == '"' || c == '\'':
		return p.quoted()
	case c == '[':
		level, ok := p.longBracketLevel()
		if !ok {
			return nil, p.errf("unexpected '['")
		}
		return p.longBody(level, true)
	case c == '-':
		p.advance()
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.eof() || !(isDigit(p.peek()) || p.peek() == '.') {
			return nil, p.errf("unary minus is only allowed before a number")
		}
		n, err := p.number()
		if err != nil {
			return nil, err
		}
		switch v := n.(type) {
		case int64:
			if v == math.MinInt64 {
				return float64(-float64(v)), nil
			}
			return -v, nil
		case float64:
			return -v, nil
		}
		return nil, p.errf("internal: bad number")
	case isDigit(c) || (c == '.' && isDigit(p.peekAt(1))):
		return p.number()
	case isIdentStart(c):
		name, _ := p.ident()
		switch name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
		return nil, p.errf("identifier %q is not a literal (code is never executed)", name)
	}
	return nil, p.errf("unexpected character %q", c)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (p *parser) number() (any, error) {
	start := p.pos
	if p.peek() == '0' && (p.peekAt(1) == 'x' || p.peekAt(1) == 'X') {
		p.advance()
		p.advance()
		hs := p.pos
		for !p.eof() && isHex(p.peek()) {
			p.advance()
		}
		if p.pos == hs {
			return nil, p.errf("malformed hex number")
		}
		if !p.eof() && (isIdentChar(p.peek()) || p.peek() == '.') {
			return nil, p.errf("malformed hex number")
		}
		u, err := strconv.ParseUint(string(p.src[hs:p.pos]), 16, 64)
		if err != nil {
			return nil, p.errf("hex number out of range")
		}
		if u > math.MaxInt64 {
			return float64(u), nil
		}
		return int64(u), nil
	}
	isFloat := false
	for !p.eof() && isDigit(p.peek()) {
		p.advance()
	}
	if !p.eof() && p.peek() == '.' {
		isFloat = true
		p.advance()
		for !p.eof() && isDigit(p.peek()) {
			p.advance()
		}
	}
	if !p.eof() && (p.peek() == 'e' || p.peek() == 'E') {
		isFloat = true
		p.advance()
		if !p.eof() && (p.peek() == '+' || p.peek() == '-') {
			p.advance()
		}
		ds := p.pos
		for !p.eof() && isDigit(p.peek()) {
			p.advance()
		}
		if p.pos == ds {
			return nil, p.errf("malformed number exponent")
		}
	}
	if !p.eof() && (isIdentChar(p.peek()) || p.peek() == '.') {
		return nil, p.errf("malformed number")
	}
	text := string(p.src[start:p.pos])
	if !isFloat {
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i, nil
		}
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return nil, p.errf("number out of range: %s", text)
	}
	return f, nil
}

func (p *parser) quoted() (string, error) {
	quote := p.advance()
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errf("unterminated string")
		}
		c := p.advance()
		switch {
		case c == quote:
			return b.String(), nil
		case c == '\n':
			return "", p.errf("unterminated string (newline)")
		case c == '\\':
			if p.eof() {
				return "", p.errf("unterminated string escape")
			}
			e := p.advance()
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'a':
				b.WriteByte('\a')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'v':
				b.WriteByte('\v')
			case '\\', '"', '\'':
				b.WriteByte(e)
			case '\n':
				b.WriteByte('\n')
			case '\r':
				if !p.eof() && p.peek() == '\n' {
					p.advance()
				}
				b.WriteByte('\n')
			case 'x':
				if p.pos+2 > len(p.src) || !isHex(p.src[p.pos]) || !isHex(p.src[p.pos+1]) {
					return "", p.errf("bad \\x escape")
				}
				v, _ := strconv.ParseUint(string(p.src[p.pos:p.pos+2]), 16, 8)
				p.advance()
				p.advance()
				b.WriteByte(byte(v))
			case 'z':
				for !p.eof() && (p.peek() == ' ' || p.peek() == '\t' || p.peek() == '\r' || p.peek() == '\n') {
					p.advance()
				}
			default:
				if isDigit(e) {
					v := int(e - '0')
					for i := 0; i < 2 && !p.eof() && isDigit(p.peek()); i++ {
						v = v*10 + int(p.advance()-'0')
					}
					if v > 255 {
						return "", p.errf("decimal escape out of range")
					}
					b.WriteByte(byte(v))
				} else {
					return "", p.errf("invalid escape \\%c", e)
				}
			}
		default:
			b.WriteByte(c)
		}
		if p.lim.MaxStringLen > 0 && b.Len() > p.lim.MaxStringLen {
			return "", p.errf("string too long")
		}
	}
}

// longBracketLevel checks for `[[` or `[==[` at the cursor without consuming.
func (p *parser) longBracketLevel() (int, bool) {
	if p.peek() != '[' {
		return 0, false
	}
	i := p.pos + 1
	level := 0
	for i < len(p.src) && p.src[i] == '=' {
		level++
		i++
	}
	if i < len(p.src) && p.src[i] == '[' {
		return level, true
	}
	return 0, false
}

func (p *parser) longBody(level int, keep bool) (string, error) {
	for i := 0; i < level+2; i++ {
		p.advance()
	}
	// A newline right after the opening bracket is skipped.
	if !p.eof() && p.peek() == '\r' {
		p.advance()
	}
	if !p.eof() && p.peek() == '\n' {
		p.advance()
	}
	start := p.pos
	for !p.eof() {
		if p.peek() == ']' {
			j := p.pos + 1
			l := 0
			for j < len(p.src) && p.src[j] == '=' {
				l++
				j++
			}
			if l == level && j < len(p.src) && p.src[j] == ']' {
				body := string(p.src[start:p.pos])
				for p.pos <= j {
					p.advance()
				}
				if keep && p.lim.MaxStringLen > 0 && len(body) > p.lim.MaxStringLen {
					return "", p.errf("string too long")
				}
				return body, nil
			}
		}
		p.advance()
	}
	if keep {
		return "", p.errf("unterminated long string")
	}
	return "", p.errf("unterminated long comment")
}

func (p *parser) table(depth int) (*Table, error) {
	if p.lim.MaxDepth > 0 && depth > p.lim.MaxDepth {
		return nil, p.errf("tables nested too deeply")
	}
	p.advance() // '{'
	t := &Table{}
	var next int64 = 1
	set := func(k any, v any) {
		switch key := k.(type) {
		case string:
			if t.Str == nil {
				t.Str = map[string]any{}
			}
			if v == nil {
				delete(t.Str, key)
			} else {
				t.Str[key] = v
			}
		case int64:
			if t.Int == nil {
				t.Int = map[int64]any{}
			}
			if v == nil {
				delete(t.Int, key)
			} else {
				t.Int[key] = v
			}
		}
	}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.eof() {
			return nil, p.errf("unterminated table")
		}
		c := p.peek()
		if c == '}' {
			p.advance()
			return t, nil
		}
		switch {
		case c == '[' && p.peekAt(1) != '[' && p.peekAt(1) != '=':
			// [key] = value
			p.advance()
			k, err := p.value(depth)
			if err != nil {
				return nil, err
			}
			if err := p.skipSpace(); err != nil {
				return nil, err
			}
			if p.eof() || p.peek() != ']' {
				return nil, p.errf("expected ']'")
			}
			p.advance()
			if err := p.expect('='); err != nil {
				return nil, err
			}
			v, err := p.value(depth)
			if err != nil {
				return nil, err
			}
			key, err := p.normKey(k)
			if err != nil {
				return nil, err
			}
			set(key, v)
		case isIdentStart(c):
			// name = value, or a bare literal keyword in positional form
			save, sl, sc := p.pos, p.line, p.col
			name, _ := p.ident()
			if err := p.skipSpace(); err != nil {
				return nil, err
			}
			if !p.eof() && p.peek() == '=' && !isReserved(name) {
				p.advance()
				v, err := p.value(depth)
				if err != nil {
					return nil, err
				}
				set(name, v)
			} else {
				p.pos, p.line, p.col = save, sl, sc
				p.nodes-- // value() counts it again
				v, err := p.value(depth)
				if err != nil {
					return nil, err
				}
				set(next, v)
				next++
			}
		default:
			v, err := p.value(depth)
			if err != nil {
				return nil, err
			}
			set(next, v)
			next++
		}
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.eof() {
			return nil, p.errf("unterminated table")
		}
		switch p.peek() {
		case ',', ';':
			p.advance()
		case '}':
		default:
			return nil, p.errf("expected ',' or '}' in table")
		}
	}
}

func (p *parser) expect(c byte) error {
	if err := p.skipSpace(); err != nil {
		return err
	}
	if p.eof() || p.peek() != c {
		return p.errf("expected '%c'", c)
	}
	p.advance()
	return nil
}

func (p *parser) normKey(k any) (any, error) {
	switch v := k.(type) {
	case nil:
		return nil, p.errf("table key is nil")
	case string, int64:
		return v, nil
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return int64(v), nil
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case *Table:
		return nil, p.errf("table used as a key")
	}
	return nil, p.errf("unsupported key")
}
