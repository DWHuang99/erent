BEGIN;
DROP TABLE api_key_accounts;
DROP TABLE api_keys;
ALTER TABLE oauth_infos DROP CONSTRAINT oauth_infos_user_id_id_unique;
COMMIT;
