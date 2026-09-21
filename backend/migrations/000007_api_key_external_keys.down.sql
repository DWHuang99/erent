BEGIN;
DROP TABLE api_key_external_keys;
ALTER TABLE external_api_keys DROP CONSTRAINT external_api_keys_user_id_id_unique;
COMMIT;
