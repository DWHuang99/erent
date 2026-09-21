BEGIN;
CREATE TABLE external_api_keys (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash VARCHAR(64) NOT NULL,
    ciphertext TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    suffix_chat_completions TEXT NOT NULL DEFAULT '',
    suffix_responses TEXT NOT NULL DEFAULT '',
    suffix_messages TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX external_key_lookup ON external_api_keys(user_id, key_hash);
COMMIT;
