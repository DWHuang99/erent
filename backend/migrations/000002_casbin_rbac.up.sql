CREATE TABLE IF NOT EXISTS casbin_rule (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL DEFAULT '',
    v0 VARCHAR(100) NOT NULL DEFAULT '',
    v1 VARCHAR(100) NOT NULL DEFAULT '',
    v2 VARCHAR(100) NOT NULL DEFAULT '',
    v3 VARCHAR(100) NOT NULL DEFAULT '',
    v4 VARCHAR(100) NOT NULL DEFAULT '',
    v5 VARCHAR(100) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_casbin_rule
    ON casbin_rule (ptype, v0, v1, v2, v3, v4, v5);

INSERT INTO casbin_rule (ptype, v0, v1)
VALUES
    ('p', 'role:user', 'dashboard:view'),
    ('p', 'role:admin', 'dashboard:view'),
    ('p', 'role:test', 'dashboard:view')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;

INSERT INTO casbin_rule (ptype, v0, v1)
SELECT 'g', 'user:' || id::TEXT, 'role:' || role_code
FROM users
WHERE role_code IN ('user', 'admin', 'test')
ON CONFLICT (ptype, v0, v1, v2, v3, v4, v5) DO NOTHING;
