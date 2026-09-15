BEGIN;

CREATE TABLE api_keys (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(20) NOT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, id)
);

ALTER TABLE oauth_infos
    ADD CONSTRAINT oauth_infos_user_id_id_unique UNIQUE (user_id, id);

CREATE TABLE api_key_accounts (
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    oauth_info_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (api_key_id, oauth_info_id),
    FOREIGN KEY (user_id, api_key_id)
        REFERENCES api_keys(user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, oauth_info_id)
        REFERENCES oauth_infos(user_id, id) ON DELETE CASCADE
);

CREATE INDEX api_key_accounts_account_idx ON api_key_accounts(user_id, oauth_info_id);

COMMIT;
