CREATE TABLE users (
  id varchar(32) PRIMARY KEY
    CONSTRAINT users_id_check CHECK (id ~ '^[0-9a-f]{32}$'),
  username varchar(64) NOT NULL,
  password_hash varchar(255) NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NULL,
  CONSTRAINT users_username_check CHECK (length(username) BETWEEN 1 AND 64),
  CONSTRAINT users_password_hash_check CHECK (length(password_hash) > 0)
);

CREATE UNIQUE INDEX users_username_idx ON users (username);

INSERT INTO users (id, username, password_hash, is_active, created_at)
VALUES (
  '00000000000000000000000000000001',
  'admin',
  '$2a$10$RLOUsCJG.Ntj2lqIfL9JbuXrJjrifIl45KecRH8fQZ3pHDOgFYIva',
  true,
  CURRENT_TIMESTAMP
)
ON CONFLICT (username) DO NOTHING;
