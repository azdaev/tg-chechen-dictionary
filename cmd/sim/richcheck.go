package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Everything Bot API "Rich HTML style" lists, restricted to what a dictionary
// card could ever emit. A tag outside this set is dropped or rejected by
// Telegram, and either way the reader loses the structure it was carrying.
var richTags = map[string]bool{
	"a": true, "b": true, "strong": true, "i": true, "em": true,
	"u": true, "ins": true, "s": true, "strike": true, "del": true,
	"code": true, "mark": true, "sub": true, "sup": true, "tg-spoiler": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"p": true, "pre": true, "footer": true, "hr": true, "br": true,
	"ul": true, "ol": true, "li": true, "blockquote": true, "aside": true,
	"cite": true, "table": true, "caption": true, "tr": true, "th": true,
	"td": true, "details": true, "summary": true,
}

// void tags carry no closing partner.
var richVoid = map[string]bool{"hr": true, "br": true}

var tagRe = regexp.MustCompile(`</?([a-zA-Z0-9-]+)[^>]*>`)

// checkRich reports every way a card would fail Telegram's rich parser that can
// be told without asking it: an unknown tag, an unclosed one, a cell outside a
// row. It does not prove a card renders well — only that it is well formed.
func checkRich(html string) []string {
	var problems []string
	var stack []string

	for _, m := range tagRe.FindAllStringSubmatch(html, -1) {
		whole, name := m[0], strings.ToLower(m[1])
		closing := strings.HasPrefix(whole, "</")

		if !richTags[name] {
			problems = append(problems, "неизвестный тег <"+name+">")
			continue
		}
		if richVoid[name] {
			continue
		}
		if !closing {
			// Telegram builds RichBlockTable out of rows and cells; a cell with
			// no row around it has nowhere to go. Checked against the open stack,
			// not against the text, because we emit «<table compact>».
			if parent, needs := richParent[name]; needs && !slices.Contains(stack, parent) {
				problems = append(problems, "<"+name+"> вне <"+parent+">")
			}
			if name == "li" && !slices.Contains(stack, "ul") && !slices.Contains(stack, "ol") {
				problems = append(problems, "<li> вне списка")
			}
			stack = append(stack, name)
			continue
		}
		if len(stack) == 0 {
			problems = append(problems, "закрывающий </"+name+"> без открывающего")
			continue
		}
		if top := stack[len(stack)-1]; top != name {
			problems = append(problems, fmt.Sprintf("</%s> закрывает открытый <%s>", name, top))
		}
		stack = stack[:len(stack)-1]
	}
	for _, open := range stack {
		problems = append(problems, "незакрытый <"+open+">")
	}

	return problems
}

// richParent names the tag each structural tag has to sit inside.
var richParent = map[string]string{
	"tr": "table", "td": "tr", "th": "tr", "summary": "details", "caption": "table",
}
