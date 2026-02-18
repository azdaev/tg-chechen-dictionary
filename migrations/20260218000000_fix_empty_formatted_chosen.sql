-- +goose Up
UPDATE dictionary_pairs SET formatted_chosen = NULL WHERE formatted_chosen = '';

-- +goose Down
