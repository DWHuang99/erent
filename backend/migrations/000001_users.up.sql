CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role_code VARCHAR(50) NOT NULL DEFAULT 'user',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_username_length CHECK (CHAR_LENGTH(username) BETWEEN 3 AND 50),
    CONSTRAINT users_role_code_check CHECK (role_code IN ('user', 'admin', 'test'))
);

CREATE INDEX IF NOT EXISTS users_role_code_idx ON users (role_code);
