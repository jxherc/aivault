package wire

import (
	"bytes"
	"strings"
)

// markers around the only text we own inside someone else's file
type marks struct{ start, end string }

var (
	mdMarks   = marks{"<!-- ai-vault:start -->", "<!-- ai-vault:end -->"}
	tomlMarks = marks{"# ai-vault:start", "# ai-vault:end"}
)

func nl(b []byte) string {
	if bytes.Contains(b, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// region returns [from, to) of the block incl. its trailing newline, or -1
func (m marks) region(b []byte) (int, int) {
	s := bytes.Index(b, []byte(m.start))
	if s < 0 {
		return -1, -1
	}
	e := bytes.Index(b[s:], []byte(m.end))
	if e < 0 {
		return -1, -1
	}
	e += s + len(m.end)
	if bytes.HasPrefix(b[e:], []byte("\r\n")) {
		e += 2
	} else if bytes.HasPrefix(b[e:], []byte("\n")) {
		e++
	}
	return s, e
}

func (m marks) has(b []byte) bool {
	s, _ := m.region(b)
	return s >= 0
}

func (m marks) render(body, eol string) string {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", eol)
	return m.start + eol + strings.TrimRight(body, "\r\n") + eol + m.end + eol
}

// set puts body in the block. replaces in place if the block is there,
// otherwise appends. sep is what we put before the block, so strip can take it back out
func (m marks) set(b []byte, body string) (out []byte, sep string) {
	eol := nl(b)
	blk := m.render(body, eol)
	if s, e := m.region(b); s >= 0 {
		out = append(append(append([]byte{}, b[:s]...), blk...), b[e:]...)
		return out, ""
	}
	switch {
	case len(b) == 0:
		sep = ""
	case bytes.HasSuffix(b, []byte("\n")):
		sep = eol
	default:
		sep = eol + eol
	}
	out = append(append(append([]byte{}, b...), sep...), blk...)
	return out, sep
}

// strip removes the block and the separator we added before it
func (m marks) strip(b []byte, sep string) []byte {
	s, e := m.region(b)
	if s < 0 {
		return b
	}
	if sep != "" && bytes.HasSuffix(b[:s], []byte(sep)) {
		s -= len(sep)
	}
	return append(append([]byte{}, b[:s]...), b[e:]...)
}

func (m marks) body(b []byte) string {
	s, e := m.region(b)
	if s < 0 {
		return ""
	}
	inner := string(b[s+len(m.start) : e])
	inner = strings.TrimSuffix(strings.TrimRight(inner, "\r\n"), m.end)
	return strings.Trim(strings.ReplaceAll(inner, "\r\n", "\n"), "\n")
}
