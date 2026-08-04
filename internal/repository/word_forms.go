// The spelling-insensitive index: the folded columns and the word-form table
// that carries an inflected query to its lemma.
package repository

import (
	"chetoru/pkg/tools"
	"context"
	"strings"
)

// BackfillFolded fills the folded columns for rows stored before they existed,
// in batches so no long write transaction forms. It is idempotent: rows are
// selected on `original_folded is null`, and new rows arrive with the columns
// already filled, so a concurrent INSERT can never be missed.
//
// On error it returns the number of rows already updated along with the error —
// a partially filled column is correct for the rows it does hold, so the caller
// logs and keeps serving; the folded lookup simply covers less.
func (r *Repository) BackfillFolded(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = 500
	}
	total := 0
	for {
		rows, err := r.db.QueryContext(
			ctx,
			`select id, original_clean, translation_clean from dictionary_pairs
			 where original_folded is null limit ?;`,
			batch,
		)
		if err != nil {
			return total, err
		}
		type row struct {
			id                    int64
			original, translation string
		}
		var pending []row
		for rows.Next() {
			var v row
			if err := rows.Scan(&v.id, &v.original, &v.translation); err != nil {
				rows.Close()
				return total, err
			}
			pending = append(pending, v)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return total, err
		}
		rows.Close()
		if len(pending) == 0 {
			return total, nil
		}

		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return total, err
		}
		stmt, err := tx.PrepareContext(ctx,
			`update dictionary_pairs set original_folded = ?, translation_folded = ? where id = ?;`)
		if err != nil {
			tx.Rollback()
			return total, err
		}
		for _, v := range pending {
			if _, err := stmt.ExecContext(ctx, tools.FoldSearch(v.original), tools.FoldSearch(v.translation), v.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return total, err
			}
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += len(pending)
	}
}

// SaveWordForms records a headword's paradigm, keyed by the folded form so a
// query that dropped the long-vowel tilde («лоьмаш» for «ло̃ьмаш») still lands.
// The headword itself is skipped — the folded columns already reach it.
func (r *Repository) SaveWordForms(ctx context.Context, headword string, forms []string) error {
	headword = strings.TrimSpace(headword)
	if headword == "" || len(forms) == 0 {
		return nil
	}
	headFolded := tools.FoldSearch(headword)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`insert or ignore into word_forms (form_folded, headword) values (?, ?);`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, f := range forms {
		folded := tools.FoldSearch(f)
		if folded == "" || folded == headFolded {
			continue
		}
		if _, err := stmt.ExecContext(ctx, folded, headword); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// FindHeadwordsByForm returns the headwords whose paradigm contains folded.
// A form can belong to more than one word, so the caller gets every match.
func (r *Repository) FindHeadwordsByForm(ctx context.Context, folded string, limit int) ([]string, error) {
	if folded == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.QueryContext(ctx,
		`select headword from word_forms where form_folded = ? limit ?;`, folded, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
