CREATE TABLE IF NOT EXISTS article_operations (
    operation_id uuid PRIMARY KEY,
    article_id bigint REFERENCES articles(id),
    succeeded_at timestamptz,
    expires_at timestamptz,
    CONSTRAINT article_operations_completion CHECK (
        (article_id IS NULL AND succeeded_at IS NULL AND expires_at IS NULL)
        OR (article_id IS NOT NULL AND succeeded_at IS NOT NULL AND expires_at IS NOT NULL AND expires_at > succeeded_at)
    )
);
CREATE INDEX IF NOT EXISTS article_operations_expiry_idx ON article_operations(expires_at, operation_id) WHERE expires_at IS NOT NULL;
