-- +goose Up
-- Tracks words users searched for but the dictionary had no translation.
-- This surfaces vocabulary gaps so maintainers know exactly which Chechen
-- words to add next — directly serving the mission of growing the dictionary.
CREATE TABLE IF NOT EXISTS missing_words (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    clean_word TEXT NOT NULL UNIQUE,
    raw_word TEXT NOT NULL,
    search_count INTEGER NOT NULL DEFAULT 1,
    first_searched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_searched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_missing_words_search_count ON missing_words(search_count DESC);
CREATE INDEX idx_missing_words_last_searched ON missing_words(last_searched_at);

-- +goose Down
DROP TABLE IF EXISTS missing_words;
