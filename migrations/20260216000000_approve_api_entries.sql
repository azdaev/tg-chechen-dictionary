-- +goose Up
-- +goose StatementBegin
-- Approve all existing entries from API that are not yet approved
-- This fixes the issue where API entries were saved with is_approved=0
-- but the search only looks for is_approved=1
update dictionary_pairs
set is_approved = 1,
    approved_at = current_timestamp,
    approved_by = 'auto_migration_20260216'
where is_approved = 0
  and source = 'api';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revert auto-approved entries (in case of rollback)
update dictionary_pairs
set is_approved = 0,
    approved_at = null,
    approved_by = null
where approved_by = 'auto_migration_20260216';
-- +goose StatementEnd
