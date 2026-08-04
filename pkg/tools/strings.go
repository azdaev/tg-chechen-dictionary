// Search keys and the respellings behind them: normalization, the fold that
// drops what a keyboard cannot type, and the palochka/ё/ъ candidate cascades.
package tools

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	tagRe  = regexp.MustCompile(`<[^>]*>`)
	boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)

	// The source brackets the part of a word or phrase it considers optional —
	// "[дӏа]долла", "орца [даккхар]", "эхь хета[ш долу]". Written out the
	// bracket is the full form, which is the word a learner needs; left in, the
	// card offers «[дӏа]долла» as the Chechen for «зарыть», which is nothing
	// anyone can say or type. The dictionary's own other corpus spells that
	// entry «дӏадолла». A bracket welded to the headword placeholder — "[как]~а
	// на сене" — is the source missing a space: the placeholder stands for a
	// whole word, so nothing can be prefixed onto it.
	optionalBrackets = strings.NewReplacer("]~", " ~", "[", "", "]", "")
)

func Clean(text string) string {
	// Most dictionary strings carry no markup at all; skip the regex for them.
	if !strings.ContainsAny(text, "<\n[]") {
		return text
	}
	// tagRe already consumed "<>" and "<br />" by the time the old explicit
	// replacements for them ran, so they never fired.
	output := tagRe.ReplaceAllString(text, "")
	output = strings.ReplaceAll(output, "\n", " ")
	return optionalBrackets.Replace(output)
}

// StripTags removes HTML tags but keeps the line structure. It backs the
// plain-text resend after Telegram rejects our markup, where Clean would be
// wrong: Clean also folds newlines into spaces, which is right for a one-line
// gloss and would turn a rejected card into a paragraph.
func StripTags(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	return tagRe.ReplaceAllString(text, "")
}

func NormalizeSearch(text string) string {
	clean := Clean(text)
	clean = strings.TrimSpace(clean)
	clean = strings.ToLower(clean)
	clean = strings.ReplaceAll(clean, "ё", "е")
	return foldPalochka(clean)
}

// foldPalochka replaces the digit-1 and Latin i/l stand-ins for the Chechen
// palochka with the real letter ("г1ала" → "гӏала") so all spellings share
// one cache key and match locally stored words. Only characters with a
// Cyrillic neighbor fold, which keeps Latin words and numbers intact.
func foldPalochka(s string) string {
	if !strings.ContainsAny(s, "1il") {
		return s
	}
	runes := []rune(s)
	isCyr := func(i int) bool {
		if i < 0 || i >= len(runes) {
			return false
		}
		r := runes[i]
		return r >= 'а' && r <= 'я' || r == 'ё' || r == 'ӏ'
	}
	for i, r := range runes {
		if (r == '1' || r == 'i' || r == 'l') && (isCyr(i-1) || isCyr(i+1)) {
			runes[i] = 'ӏ'
		}
	}
	return string(runes)
}

// maxYoVariants caps how many respellings YoVariants generates, since each
// one costs an API retry.
const maxYoVariants = 4

// YoVariants returns candidate respellings of a Russian query for a ё/е retry.
// Russians routinely type е for ё and the dictionary search does not fold the
// two. A Russian word carries at most one ё, so an all-е query yields one
// variant per е position ("береза" → "бёреза", "берёза"), and a query with ё
// yields the single all-е spelling.
func YoVariants(text string) []string {
	if strings.ContainsAny(text, "ёЁ") {
		return []string{strings.NewReplacer("ё", "е", "Ё", "Е").Replace(text)}
	}

	runes := []rune(text)
	var variants []string
	for i, r := range runes {
		var yo rune
		switch r {
		case 'е':
			yo = 'ё'
		case 'Е':
			yo = 'Ё'
		default:
			continue
		}
		v := make([]rune, len(runes))
		copy(v, runes)
		v[i] = yo
		variants = append(variants, string(v))
		if len(variants) == maxYoVariants {
			break
		}
	}
	return variants
}

const (
	// palochkaCarriers are the letters a palochka follows, covering 93% of the
	// 102 palochka-bearing headwords measured on the live dictionary;
	// word-initial adds another 3%. Measured end to end, a query that dropped
	// one palochka is reachable in 94% of cases, median 3 candidates, p90 5.
	// п and ч are worth their place: they cost nothing at the median and lift
	// coverage from 87%, and «чӏегӏардиг» — the word that started this — needs ч.
	palochkaCarriers = "цгбткхдпч"
	chechenVowels    = "аеиоуыэюяьъ"
	// maxPalochkaVariants caps the cascade the same way maxYoVariants does:
	// every candidate costs one API retry.
	maxPalochkaVariants = 8
)

// FoldSearch is NormalizeSearch minus everything a keyboard cannot type:
// combining marks (long vowel U+0303, Russian stress U+0301), the palochka
// itself, and ъ. It is the key of the folded columns, so a query that dropped
// one or all of them still matches a stored word.
//
// ъ folds because it is silent to the ear and users drop it — «колам» for
// «къолам». ь deliberately does NOT: in Chechen it is not a soft sign but half
// of the vowels аь, оь, уь, so folding it would merge «лоьман» into «ломан» and
// «аьрзу» into «арзу» — distinct words collapsing into one bucket.
//
// NFD is deliberately not applied: it would decompose «й» (U+0439) into «и» +
// U+0306 and the Mn filter below would eat the breve, turning «йоьшу» into
// «иоьшу». The marks we do want gone have no precomposed form over Cyrillic and
// already live as separate runes.
func FoldSearch(text string) string {
	s := NormalizeSearch(text)
	s = strings.Map(func(r rune) rune {
		if r == 'ӏ' || r == 'ъ' || unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)
	return s
}

// LooksChechen reports whether a query carries a marker Russian never has: a
// palochka, or a vowel+ь digraph. It gates the palochka cascade so a Russian
// typo costs no API retries.
func LooksChechen(s string) bool {
	s = NormalizeSearch(s)
	if strings.ContainsRune(s, 'ӏ') {
		return true
	}
	return strings.Contains(s, "аь") || strings.Contains(s, "оь") || strings.Contains(s, "уь")
}

// PalochkaVariants returns the spellings one inserted palochka away from word.
// 95% of palochka-bearing words carry exactly one, which is why a single
// insertion is enough and the candidate set stays linear in word length. Of 385
// palochka-bearing headwords measured on the live dictionary, 35 carry two —
// «гӏазгӏумки», «чӏегӏардиг» — and those stay out of reach here: two insertions
// is a squared candidate set against a volunteer API, and see fetch.go for why
// searching the surviving fragment does not work either. The folded columns
// answer them for any word already stored.
// It does not gate on LooksChechen — RespellVariants decides when to spend
// these, because the gate and this function disagree about the common case.
func PalochkaVariants(word string) []string {
	w := NormalizeSearch(word)
	if w == "" {
		return nil
	}

	runes := []rune(w)
	seen := map[string]bool{w: true}
	var out []string
	insert := func(at int) {
		// Never produce «ӏӏ».
		if at < len(runes) && runes[at] == 'ӏ' {
			return
		}
		v := string(runes[:at]) + "ӏ" + string(runes[at:])
		if seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	if !strings.ContainsRune(chechenVowels, runes[0]) {
		insert(0) // word-initial glottal stop
	}
	for i, r := range runes {
		if strings.ContainsRune(palochkaCarriers, r) {
			insert(i + 1)
		}
		if len(out) == maxPalochkaVariants {
			break
		}
	}
	// ponytail: candidates are in word order, not carrier frequency. The cap only
	// bites on words with more than 8 carrier positions, which the measured
	// sample had none of. If one shows up, sort by carrier frequency
	// (ц > г > б > т > к > х > д) before truncating.
	if len(out) > maxPalochkaVariants {
		out = out[:maxPalochkaVariants]
	}
	return out
}

// RespellVariants picks the retry cascade for a failed lookup, capped at
// maxPalochkaVariants candidates however it is composed.
//
// A query still showing a Chechen marker retries the palochka: a missing one is
// far more likely than a ё/е slip. Anything else retries ё/е — unless the
// dictionary returned nothing at all for the spelling, in which case the
// palochka candidates ride along too. That case is not an edge: 95% of
// palochka-bearing words carry exactly one, so dropping it erases the very
// marker LooksChechen looks for. Measured over 102 live headwords, the gate
// alone reaches 19% of them; a total miss is the signal that recovers the rest.
func RespellVariants(word string, unknownSpelling bool) []string {
	if LooksChechen(word) {
		return PalochkaVariants(word)
	}
	variants := YoVariants(word)
	if !unknownSpelling {
		return variants
	}
	for _, v := range PalochkaVariants(word) {
		if len(variants) == maxPalochkaVariants {
			break
		}
		variants = append(variants, v)
	}
	return variants
}
