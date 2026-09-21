BEGIN;

ALTER TABLE external_api_keys
    ADD CONSTRAINT external_api_keys_user_id_id_unique UNIQUE (user_id, id);

CREATE TABLE api_key_external_keys (
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    external_api_key_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (api_key_id, external_api_key_id),
    FOREIGN KEY (user_id, api_key_id)
        REFERENCES api_keys(user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, external_api_key_id)
        REFERENCES external_api_keys(user_id, id) ON DELETE CASCADE
);

CREATE INDEX api_key_external_keys_external_idx ON api_key_external_keys(user_id, external_api_key_id);

COMMIT;
