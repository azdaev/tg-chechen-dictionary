-- +goose Up
CREATE TABLE IF NOT EXISTS spellcheck_feedback (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    original_text TEXT NOT NULL,
    corrected_text TEXT NOT NULL,
    feedback TEXT NOT NULL CHECK (feedback IN ('like', 'dislike')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_spellcheck_feedback_created_at ON spellcheck_feedback(created_at);

-- +goose Down
DROP TABLE IF EXISTS spellcheck_feedback;
