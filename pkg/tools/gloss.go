// Parsing the text of a dictionary gloss: sense markers, the «~» standing in
// for the headword, grammar labels, abbreviations.
package tools

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	// grammarRe strips leading single-letter markers, possibly chained: gender
	// ("м цӏа"), and the palochka numbering homonyms ("ӏ ж балда" in «Губа»).
	// endingsRe strips adjective ending lists ("-ая, -ое къорден") — the comma
	// plus dash continuation keeps real words safe.
	grammarRe = regexp.MustCompile(`^([а-яёӏ]\s+)+`)
	endingsRe = regexp.MustCompile(`^-?[а-яё]{1,3}(,\s*-[а-яё]{1,3})+\s*`)
	// verbLabelRe strips the grammar labels heading a gloss ("сов., кому 1) …",
	// "несов. дала", "тк. мн. собир. …") — metadata, not translation, and
	// otherwise the first one becomes the card's header. Applied in a loop, so
	// a chain of them peels off one at a time.
	// Part-of-speech labels join them, period-terminated only — a bare "союз" is
	// also a real Russian word. The trailing group accepts a colon because
	// «Заострить» stores "сов.: заострить карандаш - къолам ирбан".
	verbLabelRe = regexp.MustCompile(`^((не)?сов\.|однокр\.|многокр\.|перех\.|неперех\.|безл\.|нескл\.|нареч\.|числ\.|мест\.|межд\.|предл\.|част\.|прил\.|сущ\.|гл\.|вводн\. сл\.|вводн\.|частица|междометие|союз|предлог|в разн\. знач\.|разн\. знач\.|т\.к\.|тк\.|мн\.|ед\.|собир\.|кратк\. ф\.|в знач\. сказ\.|в знач\. сущ\.|кому-чему|кого-что|о ком|о чём|кому|чему|кого|кем|чем|ком|что)(?:[,;:]?\s+|:)`)
	// crossRefRe strips a reference to another entry along with the entry it
	// points at: «Камешек» opens "м уменьш. от камень тӏулг", and cutting only
	// the abbreviation left «от камень тӏулг» standing in as the Chechen word.
	crossRefRe = regexp.MustCompile(`^(уменьш\.|увелич\.|ласк\.|унич\.|см\.)(\s+от)?\s+(\p{Cyrillic}+\s+)?`)
	// Sense markers come as "1)" but also as "ӏ. " (palochka standing in for
	// the digit) and "2. " in live dosham glosses.
	meaningRe = regexp.MustCompile(`(\d+\)|(?:^|\s)[ӏ\d]\.\s)`)
	// senseNumRe peels a sense number off the front of a gloss. The card
	// numbers its own senses, so "2 м лев" arrives claiming to be sense 2 of a
	// list the reader cannot see.
	senseNumRe = regexp.MustCompile(`^\d+[).]?\s+`)
	tildeRe    = regexp.MustCompile(`~([а-яё]+)`)
)

// findMainSemicolon returns the index of the semicolon that separates the main
// translation from its examples: the first one followed by a dash (the example
// marker), falling back to the first semicolon, or -1 if there is none.
func findMainSemicolon(text string) int {
	semicolons := []int{}
	for i, r := range text {
		if r == ';' {
			semicolons = append(semicolons, i)
		}
	}

	for _, pos := range semicolons {
		if strings.ContainsAny(text[pos+1:], "-–—") {
			return pos
		}
	}

	if len(semicolons) > 0 {
		return semicolons[0]
	}

	return -1
}

func cleanTranslation(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)
	return text
}

// parseExamples splits a semicolon-separated example list into rendered lines,
// capped at 5. Each example reads "headword phrase - gloss translation", so
// which side is Chechen follows from the entry, not from the text: chechenLeads
// says the headword side is the Chechen one and the source order already holds.
func parseExamples(text string, chechenLeads bool) []string {
	var examples []string

	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if left, right, ok := splitExample(part); ok {
			if left != "" && right != "" {
				if chechenLeads {
					examples = append(examples, FormatExample(left, right))
				} else {
					examples = append(examples, FormatExample(right, left))
				}
			}
		} else {
			// Not a two-sided example — a whole-sentence illustration. Still an
			// example, so it still gets the card's italic.
			examples = append(examples, "<i>"+part+"</i>")
		}
	}

	if len(examples) > 5 {
		examples = examples[:5]
	}

	return examples
}

// splitExample splits one example at the dash separating its two sides. The
// source data mixes hyphens with en/em dashes, so all three count, and the
// separator needs a space on at least one side: live glosses glue it to the
// preceding tilde ("деревянный ~- дечиган цӏа") but never write the
// case-government shorthand ("кого-л.", "что-л.") with any space at all, and
// splitting on that one turns «проводить кого-л. до дома» into «л. до дома →
// проводить кого». ok is false when no dash qualifies.
func splitExample(part string) (left, right string, ok bool) {
	for i, r := range part {
		if r != '-' && r != '–' && r != '—' {
			continue
		}
		width := utf8.RuneLen(r)
		if i == 0 || i+width >= len(part) {
			continue // nothing to put on one of the sides
		}
		if part[i-1] != ' ' && part[i+width] != ' ' {
			continue // inside a word
		}
		left = strings.Trim(strings.TrimSpace(part[:i]), `"«»""`)
		right = strings.Trim(strings.TrimSpace(part[i+width:]), `"«»""`)
		return left, right, true
	}
	return "", "", false
}

// replaceTildeWithWord expands the dictionary's tilde shorthand for word. exact
// is false when some tilde needed a stem wordStem refuses to guess; callers that
// can drop the line should, and the rest get the headword uninflected — wrong
// case, but a word of the language rather than an invented one.
func replaceTildeWithWord(text, word string) (string, bool) {
	if word == "" {
		return text, true
	}

	lowerWord := strings.ToLower(word)
	stem, regular := wordStem(lowerWord)
	exact := true
	result := tildeRe.ReplaceAllStringFunc(text, func(match string) string {
		ending := match[1:]
		// Русские грамматические окончания не длиннее 3 букв. Более длинный
		// «хвост» — это отдельное слово, склеенное в источнике с заглавным
		// (например «~культуры» для слова «дом» значит «дом культуры»).
		// Подставляем слово с пробелом, а не лепим несуществующее «домкультуры».
		if len([]rune(ending)) >= 4 {
			return lowerWord + " " + ending
		}
		if !regular {
			exact = false
			return lowerWord
		}
		return stem + ending
	})

	// Замена одиночной тильды (~) на само слово в именительном падеже.
	// Одиночная тильда обозначает заглавное слово без изменений, поэтому
	// подставляем полную форму, а не основу.
	return strings.ReplaceAll(result, "~", lowerWord), exact
}

// wordStem cuts a headword back to the part a short grammatical ending attaches
// to, and reports whether the cut is one Russian spelling actually determines.
//
// Two are. An adjective in -ый/-ий/-ой drops those two letters for every form
// it has. A word ending in a vowel drops it — «слеза» + «~ами» is «слезами»,
// «домашний» + «~ие» is «домашние».
//
// The rest are morphology the spelling does not carry, and the old heuristic
// invented words there: a consonant-final headword may hide a fleeting vowel
// («силок» + «~ки» is «силки», not «силокки») or a suffix the ending replaces
// («разбойник» + «~ца» is «разбойница»), and a verb's infinitive is not its
// present stem («визжать» + «~ит» is «визжит», not «визжаит»). Measured on
// 1800 live entries, refusing these two classes drops every wrong expansion in
// the sample and about a dozen right ones.
func wordStem(word string) (string, bool) {
	runes := []rune(word)
	if len(runes) < 3 {
		return word, false
	}
	switch string(runes[len(runes)-2:]) {
	case "ый", "ий", "ой":
		return string(runes[:len(runes)-2]), true
	case "ся":
		// A reflexive verb ends in a vowel but is not a vowel stem: «застояться»
		// + «~лся» is «застоялся», and cutting the last letter writes
		// «застоятьслся».
		return word, false
	}
	if strings.ContainsRune("аеёиоуыэюя", runes[len(runes)-1]) {
		return string(runes[:len(runes)-1]), true
	}
	return word, false
}

// expandAbbreviations заменяет словарные сокращения на полные формы
func expandAbbreviations(text string) string {
	return abbreviationReplacer.Replace(text)
}

// abbreviationReplacer expands dictionary abbreviations in one pass. Pairs are
// ordered longest-first (ties alphabetical, so the order is deterministic)
// because short abbreviations can be substrings of longer ones ("им." inside
// "хим."). Expansions contain no periods while every abbreviation ends with
// one, so a single pass cannot create new matches.
var abbreviationReplacer = newAbbreviationReplacer()

func newAbbreviationReplacer() *strings.Replacer {
	abbreviations := map[string]string{
		"тж.": "также",
		// Both spellings occur live: "тк. мн." in «Нечистоты», "т.к. кратк. ф."
		// in «Длинный». In a dictionary gloss it is "только", not "так как".
		"тк.":  "только",
		"т.к.": "только",
		// "кратк." earns an entry even though the card rarely shows it: without
		// one the replacer finds "тк." inside it and writes "кратолько".
		// Longest-first ordering then lets the whole phrase win.
		"кратк. ф.":  "(краткая форма)",
		"кратк.":     "(краткая форма)",
		"вводн. сл.": "(вводное слово)",
		"разг.":      "(разговорное)",
		"прост.":     "(просторечие)",
		"перен.":     "(переносное)",
		"устар.":     "(устаревшее)",
		"книжн.":     "(книжное)",
		"офиц.":      "(официальное)",
		"спец.":      "(специальное)",
		"мед.":       "(медицинское)",
		"воен.":      "(военное)",
		"юр.":        "(юридическое)",
		"тех.":       "(техническое)",
		"муз.":       "(музыкальное)",
		"мат.":       "(математическое)",
		"физ.":       "(физическое)",
		"хим.":       "(химическое)",
		"биол.":      "(биологическое)",
		"геол.":      "(геологическое)",
		"бот.":       "(ботаническое)",
		"зоол.":      "(зоологическое)",
		"геогр.":     "(географическое)",
		"ист.":       "(историческое)",
		"эк.":        "(экономическое)",
		"полит.":     "(политическое)",
		"рел.":       "(религиозное)",
		"филос.":     "(философское)",
		"лит.":       "(литературное)",
		"поэт.":      "(поэтическое)",
		"ирон.":      "(ироничное)",
		"шутл.":      "(шутливое)",
		"пренебр.":   "(пренебрежительное)",
		"ласк.":      "(ласкательное)",
		"уменьш.":    "уменьшительное",
		"увелич.":    "(увеличительное)",
		"собир.":     "(собирательное)",
		"множ.":      "(множественное)",
		"ед.":        "(единственное)",
		"мн.":        "(множественное)",
		"им.":        "(именительный)",
		"род.":       "(родительный)",
		"дат.":       "(дательный)",
		"вин.":       "(винительный)",
		"тв.":        "(творительный)",
		"пр.":        "(предложный)",
	}

	keys := make([]string, 0, len(abbreviations))
	for abbrev := range abbreviations {
		keys = append(keys, abbrev)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})

	pairs := make([]string, 0, len(keys)*2)
	for _, abbrev := range keys {
		pairs = append(pairs, abbrev, abbreviations[abbrev])
	}
	return strings.NewReplacer(pairs...)
}
