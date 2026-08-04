package cache

import (
	"reflect"
	"strings"
	"testing"

	"chetoru/internal/models"
)

// pairShape is what a cached answer was serialized from when
// translationKeyVersion was last bumped.
const pairShape = "Original:string Translate:string OriginalLang:string TranslateLang:string " +
	"FormattedAI:string FormattedChosen:string Rate:int Packed:bool EntryType:string " +
	"Subtype:int EntryIndex:int Notes:string Structured:string"

// A cached answer is a []models.TranslationPairs, held for a month. Change that
// struct without bumping translationKeyVersion and every answer stored by the
// previous release decodes into something this one no longer means — a missing
// field arrives as its zero value and nothing complains.
//
// That is not hypothetical: «Packed» was added without a bump. It says the
// Chechen side is a whole article rather than a gloss, so every article cached
// before that release would have come back false and been rendered as its own
// raw text — «ж жӏаьла ; он на этом ~у съел - …» offered as the meaning of
// «собака», for thirty days, on nothing but a deploy.
func TestTranslationKeyVersion_TracksThePairShape(t *testing.T) {
	rt := reflect.TypeFor[models.TranslationPairs]()
	fields := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		fields = append(fields, rt.Field(i).Name+":"+rt.Field(i).Type.String())
	}
	if got := strings.Join(fields, " "); got != pairShape {
		t.Fatalf("models.TranslationPairs changed shape.\n got: %s\nwant: %s\n"+
			"Bump translationKeyVersion (now %d) so a month of answers stored in the old shape "+
			"is dropped rather than misread, then update pairShape.", got, pairShape, translationKeyVersion)
	}
}
