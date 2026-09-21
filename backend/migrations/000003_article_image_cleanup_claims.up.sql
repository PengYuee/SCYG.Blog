ALTER TABLE article_images
  ADD COLUMN cleanup_claim_token VARCHAR(32) NULL,
  ADD COLUMN cleanup_claim_expires_at TIMESTAMPTZ NULL,
  ADD CONSTRAINT article_images_cleanup_claim_check CHECK (
    (cleanup_claim_token IS NULL AND cleanup_claim_expires_at IS NULL) OR
    (cleanup_claim_token IS NOT NULL AND cleanup_claim_expires_at IS NOT NULL)
  );

CREATE INDEX article_images_cleanup_claim_idx
  ON article_images (status, expires_at, cleanup_claim_expires_at, id)
  WHERE status IN ('pending', 'orphaned');
