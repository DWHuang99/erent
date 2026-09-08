BEGIN;

-- Existing credentials need an explicitly assigned owner before this migration.
-- Never infer ownership from an upstream email or assign it to the administrator.
ALTER TABLE oauth_infos ADD COLUMN IF NOT EXISTS user_id BIGINT;
ALTER TABLE oauth_infos ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE oauth_infos ADD CONSTRAINT oauth_infos_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
CREATE UNIQUE INDEX oauth_infos_owner_account_idx ON oauth_infos (user_id, type, account_id);

COMMIT;
