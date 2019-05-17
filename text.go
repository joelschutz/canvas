package canvas

import (
	"image/color"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TextAlign int

const (
	Left TextAlign = iota
	Right
	Center
	Top
	Bottom
	Justify
)

type Text struct {
	lines       [][]lineSpan // each line can have multiple spans
	dy          float64
	lineSpacing float64
}

type lineSpan struct {
	span
	dx float64
	w  float64
}

func splitNewlines(s string) []string {
	ss := []string{}
	i := 0
	for j, r := range s {
		if r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
			if r == '\n' && j > 0 && s[j-1] == '\r' {
				i++
				continue
			}
			ss = append(ss, s[i:j])
			i = j + utf8.RuneLen(r)
		}
	}
	ss = append(ss, s[i:])
	return ss
}

func calcSpanPosition(textWidth, maxTextWidth float64, halign TextAlign, indent, width float64) (float64, float64) {
	dx := indent
	spanWidth := textWidth
	if halign == Right {
		dx = width - textWidth - indent
	} else if halign == Center {
		dx = (width - textWidth) / 2.0
	} else if halign == Justify {
		spanWidth = math.Min(maxTextWidth, width-indent)
	}
	return dx, spanWidth
}

func NewText(ff FontFace, s string) *Text {
	ss := splitNewlines(s)
	lines := [][]lineSpan{}
	for _, s := range ss {
		lines = append(lines, []lineSpan{{newTextSpan(ff, s), 0.0, 0.0}})
	}
	return &Text{
		lines:       lines,
		dy:          0.0,
		lineSpacing: 0.0,
	}
}

func NewTextBox(ff FontFace, s string, width, height float64, halign, valign TextAlign, indent float64) *Text {
	// TODO: do inner-word boundaries
	h, prevLineSpacing := 0.0, 0.0
	lines := [][]lineSpan{}
	var iPrev, iSpace int
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == ' ' {
			iSpace = i
		} else if r == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			size++
		}

		isNewline := r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029'
		if isNewline || width != 0.0 && width < ff.TextWidth(s[iPrev:i+size])+indent {
			iBreak := i
			if !isNewline && iPrev < iSpace {
				iBreak = iSpace // break line at last space
			}

			var ls lineSpan
			if isNewline {
				textSpan := newTextSpan(ff, s[iPrev:iBreak])
				textWidth, _ := textSpan.WidthRange()
				ls = lineSpan{textSpan, indent, textWidth}
			} else {
				textSpan := newTextSpan(ff, s[iPrev:iBreak])
				textWidth, maxTextWidth := textSpan.WidthRange()
				dx, spanWidth := calcSpanPosition(textWidth, maxTextWidth, halign, indent, width)
				ls = lineSpan{textSpan, dx, spanWidth}
			}
			ascent, descent, curLineSpacing := ls.span.Heights()
			topLineSpacing := math.Max(prevLineSpacing, curLineSpacing)
			if h == 0.0 {
				topLineSpacing = 0.0
			}
			if height != 0.0 && height < h+topLineSpacing+ascent+descent {
				break
			}
			lines = append(lines, []lineSpan{ls})
			h += topLineSpacing + ascent + descent
			prevLineSpacing = curLineSpacing
			indent = 0.0

			if i == 0 {
				continue
			}
			iPrev = iBreak
			if isNewline {
				iPrev += size // skip newline
			} else if iPrev == iSpace {
				iPrev += 1 // skip space
			}
		}
		i += size
	}

	// last line does not justify
	var ls lineSpan
	if halign == Right || halign == Center {
		textSpan := newTextSpan(ff, s[iPrev:])
		textWidth, maxTextWidth := textSpan.WidthRange()
		dx, spanWidth := calcSpanPosition(textWidth, maxTextWidth, halign, indent, width)
		ls = lineSpan{textSpan, dx, spanWidth}
	} else {
		textSpan := newTextSpan(ff, s[iPrev:])
		textWidth, _ := textSpan.WidthRange()
		ls = lineSpan{textSpan, indent, textWidth}
	}
	ascent, descent, curLineSpacing := ls.span.Heights()
	topLineSpacing := math.Max(prevLineSpacing, curLineSpacing)
	if h == 0.0 {
		topLineSpacing = 0.0
	}
	if height == 0.0 || h+topLineSpacing+ascent+descent <= height {
		lines = append(lines, []lineSpan{ls})
		h += topLineSpacing + ascent + descent
	}

	dy := 0.0
	lineSpacing := 0.0
	if height != 0.0 && (valign == Bottom || valign == Center || valign == Justify) {
		if valign == Bottom {
			dy = height - h
		} else if valign == Center {
			dy = (height - h) / 2.0
		} else {
			lineSpacing = (height - h) / float64(len(lines)-1)
		}
	}
	return &Text{
		lines:       lines,
		dy:          dy,
		lineSpacing: lineSpacing,
	}
}

func (t *Text) Bounds() Rect {
	var w, h float64
	firstAscent := 0.0
	prevLineSpacing := 0.0
	for j, line := range t.lines {
		maxAscent, maxHeight, maxLineSpacing := 0.0, 0.0, 0.0
		for _, ls := range line {
			w = math.Max(w, ls.dx+ls.w)

			ascent, descent, curLineSpacing := ls.span.Heights()
			curLineSpacing = math.Max(prevLineSpacing, curLineSpacing)
			maxAscent = math.Max(maxAscent, ascent)
			if maxHeight < curLineSpacing+ascent+descent {
				maxHeight = curLineSpacing + ascent + descent
				maxLineSpacing = curLineSpacing
			}
		}
		h += maxHeight + t.lineSpacing
		if j == 0 {
			firstAscent = maxAscent
			h -= maxLineSpacing
		}
		prevLineSpacing = maxLineSpacing
	}
	return Rect{0.0, firstAscent, w, -h}

}

// ToPath makes a path out of the text, with x,y the top-left point of the rectangle that fits the text (ie. y is not the text base)
func (t *Text) ToPath(x, y float64) *Path {
	p := &Path{}
	y -= t.dy
	prevLineSpacing := 0.0
	for _, line := range t.lines {
		maxHeight, maxLineSpacing := 0.0, 0.0
		for _, ls := range line {
			p.Append(ls.span.ToPath(x+ls.dx, y, ls.w))

			ascent, descent, curLineSpacing := ls.span.Heights()
			curLineSpacing = math.Max(prevLineSpacing, curLineSpacing)
			if maxHeight < curLineSpacing+ascent+descent {
				maxHeight = curLineSpacing + ascent + descent
				maxLineSpacing = curLineSpacing
			}
		}
		y -= maxHeight + t.lineSpacing
		prevLineSpacing = maxLineSpacing
	}
	return p
}

func (t *Text) ToSVG(x, y, rot float64, c color.Color) string {
	y += t.dy

	sb := strings.Builder{}
	sb.WriteString("<text x=\"")
	writeFloat64(&sb, x)
	sb.WriteString("\" y=\"")
	writeFloat64(&sb, y)
	if rot != 0.0 {
		sb.WriteString("\" transform=\"rotate(")
		writeFloat64(&sb, -rot)
		sb.WriteString(",")
		writeFloat64(&sb, x)
		sb.WriteString(",")
		writeFloat64(&sb, y)
		sb.WriteString(")")
	}
	if c != color.Black {
		sb.WriteString("\" fill=\"")
		writeCSSColor(&sb, c)
	}
	sb.WriteString("\">")

	for _, line := range t.lines {
		h := 0.0
		for _, ls := range line {
			ascent, descent, lineSpacing := ls.span.Heights()
			h = math.Max(h, ascent+descent+lineSpacing)
			switch span := ls.span.(type) {
			case textSpan:
				name, style, size := span.ff.Info()
				span.splitAtBoundaries(ls.dx, ls.w, func(dx, w float64, s string) {
					sb.WriteString("<tspan x=\"")
					writeFloat64(&sb, x+dx)
					sb.WriteString("\" y=\"")
					writeFloat64(&sb, y)
					if w != 0.0 {
						sb.WriteString("\" textLength=\"")
						writeFloat64(&sb, w)
					}
					sb.WriteString("\" font-family=\"")
					sb.WriteString(name)
					sb.WriteString("\" font-size=\"")
					writeFloat64(&sb, size)
					if style&Italic != 0 {
						sb.WriteString("\" font-style=\"italic")
					}
					if style&Bold != 0 {
						sb.WriteString("\" font-weight=\"bold")
					}
					sb.WriteString("\">")
					sb.WriteString(s) // TODO: replace typographic characters by XML entities
					sb.WriteString("</tspan>")
				})
			default:
				panic("unsupported span type")
			}
		}
		y += h + t.lineSpacing
	}
	sb.WriteString("</text>")
	return sb.String()
}

type span interface {
	WidthRange() (float64, float64)       // min-width and max-width
	Heights() (float64, float64, float64) // ascent, descent, line spacing
	ToPath(float64, float64, float64) *Path
}

const MaxSentenceSpacing = 2.0
const MaxWordSpacing = 1.5
const MaxGlyphSpacing = 1.0

type textSpan struct {
	ff               FontFace
	s                string
	textWidth        float64
	sentenceSpacings int
	wordSpacings     int
	glyphSpacings    int
	textBoundaries   []textBoundary
}

type textBoundary struct {
	isWord bool
	loc    int
}

func calcTextSpanSpacings(s string) (int, int, int, []textBoundary) {
	sentenceSpacings, wordSpacings, glyphSpacings := 0, 0, 0
	locs := []textBoundary{}
	var rPrev, rPrevPrev rune
	for i, r := range s {
		glyphSpacings++
		if r == ' ' {
			if (rPrev == '.' && !unicode.IsUpper(rPrevPrev) && rPrevPrev != ' ') || rPrev == '!' || rPrev == '?' {
				locs = append(locs, textBoundary{false, i})
				sentenceSpacings++
			} else if rPrev != ' ' {
				locs = append(locs, textBoundary{true, i})
				wordSpacings++
			}
		}
		rPrevPrev = rPrev
		rPrev = r
	}
	glyphSpacings -= wordSpacings + sentenceSpacings + 1
	return sentenceSpacings, wordSpacings, glyphSpacings, locs
}

func newTextSpan(ff FontFace, s string) textSpan {
	textWidth := ff.TextWidth(s)
	sentenceSpacings, wordSpacings, glyphSpacings, textBoundaries := calcTextSpanSpacings(s)
	return textSpan{
		ff:               ff,
		s:                s,
		textWidth:        textWidth,
		sentenceSpacings: sentenceSpacings,
		wordSpacings:     wordSpacings,
		glyphSpacings:    glyphSpacings,
		textBoundaries:   textBoundaries,
	}
}

func (ts textSpan) WidthRange() (float64, float64) {
	spacings := float64(ts.sentenceSpacings) * MaxSentenceSpacing
	spacings += float64(ts.wordSpacings) * MaxWordSpacing
	spacings += float64(ts.glyphSpacings) * MaxGlyphSpacing
	return ts.textWidth, ts.textWidth + spacings
}

func (ts textSpan) Heights() (float64, float64, float64) {
	return ts.ff.Metrics().Ascent, ts.ff.Metrics().Descent, ts.ff.Metrics().LineHeight - ts.ff.Metrics().Ascent - ts.ff.Metrics().Descent
}

func (ts textSpan) ToPath(x, y, width float64) *Path {
	sentenceSpacing := 0.0
	wordSpacing := 0.0
	glyphSpacing := 0.0
	if width > ts.textWidth {
		widthLeft := width - ts.textWidth
		xHeight := ts.ff.Metrics().XHeight
		if ts.sentenceSpacings > 0 {
			sentenceSpacing = math.Min(widthLeft/float64(ts.sentenceSpacings), xHeight*MaxSentenceSpacing)
			widthLeft -= float64(ts.sentenceSpacings) * sentenceSpacing
		}
		if ts.wordSpacings > 0 {
			wordSpacing = math.Min(widthLeft/float64(ts.wordSpacings), xHeight*MaxWordSpacing)
			widthLeft -= float64(ts.wordSpacings) * wordSpacing
		}
		if ts.glyphSpacings > 0 {
			glyphSpacing = math.Min(widthLeft/float64(ts.glyphSpacings), xHeight*MaxGlyphSpacing)
		}
	}
	s := ts.ff.f.transform(ts.s, glyphSpacing == 0.0)

	p := &Path{}
	var rPrev rune
	iTextBoundary := 0
	for i, r := range s {
		if i > 0 {
			x += ts.ff.Kerning(rPrev, r)
		}

		pr, advance := ts.ff.ToPath(r)
		pr.Translate(x, y)
		p.Append(pr)
		x += advance

		spacing := glyphSpacing
		if iTextBoundary < len(ts.textBoundaries) && ts.textBoundaries[iTextBoundary].loc == i {
			if ts.textBoundaries[iTextBoundary].isWord {
				spacing = wordSpacing
			} else {
				spacing = sentenceSpacing
			}
			iTextBoundary++
		}
		x += spacing
		rPrev = r
	}
	return p
}

func (ts textSpan) splitAtBoundaries(spanDx, width float64, f func(float64, float64, string)) {
	spaceWidth := ts.ff.TextWidth(" ")
	sentenceSpacing := 0.0
	wordSpacing := 0.0
	glyphSpacing := 0.0
	if width > ts.textWidth {
		widthLeft := width - ts.textWidth
		xHeight := ts.ff.Metrics().XHeight
		if ts.sentenceSpacings > 0 {
			sentenceSpacing = math.Min(widthLeft/float64(ts.sentenceSpacings), xHeight*MaxSentenceSpacing)
			widthLeft -= float64(ts.sentenceSpacings) * sentenceSpacing
		}
		if ts.wordSpacings > 0 {
			wordSpacing = math.Min(widthLeft/float64(ts.wordSpacings), xHeight*MaxWordSpacing)
			widthLeft -= float64(ts.wordSpacings) * wordSpacing
		}
		if ts.glyphSpacings > 0 {
			glyphSpacing = math.Min(widthLeft/float64(ts.glyphSpacings), xHeight*MaxGlyphSpacing)
		}
	}
	if sentenceSpacing > 0.0 || wordSpacing > 0.0 {
		textBoundaries := append(ts.textBoundaries, textBoundary{true, len(ts.s)})
		prevLoc := 0
		dx := spanDx
		for _, textBoundary := range textBoundaries {
			s := ts.s[prevLoc:textBoundary.loc]
			w := 0.0
			if glyphSpacing > 0.0 {
				w = ts.ff.TextWidth(s) + float64(utf8.RuneCountInString(s)-1)*glyphSpacing
			}
			f(dx, w, s)
			prevLoc = textBoundary.loc + 1
			dx += ts.ff.TextWidth(s) + spaceWidth + float64(utf8.RuneCountInString(s))*glyphSpacing
			if textBoundary.isWord {
				dx += wordSpacing
			} else {
				dx += sentenceSpacing
			}
		}
	} else {
		w := 0.0
		if glyphSpacing > 0.0 {
			w = width
		}
		f(spanDx, w, ts.s)
	}
}
