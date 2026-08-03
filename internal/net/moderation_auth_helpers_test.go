package net

import (
	"context"
	"strconv"
)

func strconvParseInt(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

// authProbeRepo counts writes to the dictionary so a rejected callback can be
// told apart from one that merely failed later.
type authProbeRepo struct {
	Repository
	calls int
}

func (r *authProbeRepo) SetTranslationPairFormattingChoice(context.Context, int64, string) error {
	r.calls++
	return nil
}
