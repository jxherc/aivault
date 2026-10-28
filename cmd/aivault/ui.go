package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"
)

// terminal styling. 256-color codes that read on both dark and light
// backgrounds, so there's no need to ask the terminal which one it is.
// pipes and NO_COLOR get plain text (the renderer checks both)
var (
	out    = lipgloss.NewRenderer(os.Stdout)
	accent = out.NewStyle().Foreground(lipgloss.Color("33"))
	good   = out.NewStyle().Foreground(lipgloss.Color("35"))
	warnC  = out.NewStyle().Foreground(lipgloss.Color("172"))
	bad    = out.NewStyle().Foreground(lipgloss.Color("160"))
	dim    = out.NewStyle().Foreground(lipgloss.Color("244"))
	bold   = out.NewStyle().Bold(true)
)

var marks = map[string]string{"ok": "✓", "warn": "!", "fail": "✗", "info": "·"}

func mark(status string) string {
	m := marks[status]
	switch status {
	case "ok":
		return good.Render(m)
	case "warn":
		return warnC.Render(m)
	case "fail":
		return bad.Render(m)
	}
	return dim.Render(m)
}

// usable line width: the terminal's, capped so long lines stay readable
func width() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		// piped: $COLUMNS if the shell exported it
		if w, err = strconv.Atoi(os.Getenv("COLUMNS")); err != nil || w <= 0 {
			return 80
		}
	}
	return min(w, 100)
}

// wraps s into lines of at most w columns. lines after the first get
// indent spaces in front, to line up under a label column
func wrap(s string, w, indent int) string {
	if w < 20 {
		return s
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > w {
			lines = append(lines, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n"+strings.Repeat(" ", indent))
}

// joins items with sep, breaking lines between items, never inside one
func wrapItems(items []string, sep string, w, indent int) string {
	var b strings.Builder
	line := 0
	for i, it := range items {
		iw := lipgloss.Width(it)
		if i > 0 {
			if line+lipgloss.Width(sep)+iw > w && line > 0 && w >= 20 {
				b.WriteString("\n" + strings.Repeat(" ", indent))
				line = 0
			} else {
				b.WriteString(sep)
				line += lipgloss.Width(sep)
			}
		}
		b.WriteString(it)
		line += iw
	}
	return b.String()
}

// cuts s to w columns with an ellipsis
func cut(s string, w int) string {
	if w < 2 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "…"
}

var sparks = []rune("▁▂▃▄▅▆▇█")

// one block per value. square root scale, so one big import day doesn't
// flatten every normal day to the floor
func sparkline(counts []int) string {
	top := 0
	for _, c := range counts {
		top = max(top, c)
	}
	var b strings.Builder
	for _, c := range counts {
		if c == 0 || top == 0 {
			b.WriteString(dim.Render(string(sparks[0])))
			continue
		}
		i := int(math.Round(math.Sqrt(float64(c)/float64(top)) * float64(len(sparks)-1)))
		b.WriteString(accent.Render(string(sparks[max(i, 1)])))
	}
	return b.String()
}

// a spinner on stderr while something slow runs. does nothing when stderr
// isn't a terminal, so logs and pipes stay clean. stop may be called twice
func spin(msg string) (set func(string), stop func()) {
	if !isatty.IsTerminal(os.Stderr.Fd()) || os.Getenv("TERM") == "dumb" {
		return func(string) {}, func() {}
	}
	var mu sync.Mutex
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			mu.Lock()
			fmt.Fprintf(os.Stderr, "\r\x1b[K%s %s", accent.Render(string(frames[i%len(frames)])), dim.Render(msg))
			mu.Unlock()
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\x1b[K")
				return
			case <-t.C:
			}
		}
	}()
	var once sync.Once
	return func(m string) { mu.Lock(); msg = m; mu.Unlock() },
		func() { once.Do(func() { close(done); <-finished }) }
}

// paints the marks at the start of plain report lines (sync, remove, init)
type markWriter struct {
	w   io.Writer
	buf []byte
}

func painted(w io.Writer) *markWriter { return &markWriter{w: w} }

func (p *markWriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := strings.IndexByte(string(p.buf), '\n')
		if i < 0 {
			return len(b), nil
		}
		fmt.Fprintln(p.w, paintLine(string(p.buf[:i])))
		p.buf = p.buf[i+1:]
	}
}

func (p *markWriter) Flush() {
	if len(p.buf) > 0 {
		fmt.Fprint(p.w, paintLine(string(p.buf)))
		p.buf = nil
	}
}

func paintLine(l string) string {
	t := strings.TrimLeft(l, " ")
	pad := l[:len(l)-len(t)]
	for st, m := range marks {
		if rest, ok := strings.CutPrefix(t, m+" "); ok {
			rest = tildeText(rest)
			if st == "info" {
				return pad + mark(st) + " " + codes(rest, dim)
			}
			return pad + mark(st) + " " + codes(rest, out.NewStyle())
		}
	}
	return l
}

// st.Render one line at a time. lipgloss pads every line of a multi-line
// string to the widest one, which leaves trailing spaces after wrapped text
func paint(st lipgloss.Style, s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = st.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// `command` spans in accent, the rest in base. the backticks go
func codes(s string, base lipgloss.Style) string {
	parts := strings.Split(s, "`")
	if len(parts) < 3 {
		return paint(base, s)
	}
	var b strings.Builder
	for i, p := range parts {
		switch {
		case i%2 == 1 && i < len(parts)-1:
			b.WriteString(paint(accent, p))
		case i%2 == 1:
			b.WriteString(paint(base, "`"+p)) // unpaired
		default:
			b.WriteString(paint(base, p))
		}
	}
	return b.String()
}

// home folder paths inside a message, shortened to ~
func tildeText(s string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return strings.ReplaceAll(s, h+string(os.PathSeparator), "~"+string(os.PathSeparator))
	}
	return s
}

// short path for display: ~ for home
func tildePath(p string) string {
	if h, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, h) {
		return "~" + p[len(h):]
	}
	return p
}

// the closest command to a typo, or ""
func didYouMean(s string, cmds []string) string {
	best, bestD := "", 3
	for _, c := range cmds {
		if len(s) >= 2 && strings.HasPrefix(c, s) {
			return c
		}
		if d := editDist(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDist(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev = cur
	}
	return prev[len(b)]
}

// counts "- " lines in a markdown file
func bullets(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(strings.TrimSpace(sc.Text()), "- ") {
			n++
		}
	}
	return n
}
