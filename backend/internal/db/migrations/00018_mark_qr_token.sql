-- +goose Up
ALTER TABLE marks ADD COLUMN qr_token_id UUID REFERENCES qr_tokens (id) ON DELETE SET NULL;
CREATE INDEX idx_marks_qr_token_id ON marks (qr_token_id);

-- +goose Down
DROP INDEX IF EXISTS idx_marks_qr_token_id;
ALTER TABLE marks DROP COLUMN IF EXISTS qr_token_id;
