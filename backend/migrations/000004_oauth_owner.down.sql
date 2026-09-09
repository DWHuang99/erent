BEGIN;
DROP INDEX oauth_infos_owner_account_idx;
ALTER TABLE oauth_infos DROP COLUMN user_id;
COMMIT;
