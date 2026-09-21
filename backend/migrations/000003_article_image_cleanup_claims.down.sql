DROP INDEX article_images_cleanup_claim_idx;
ALTER TABLE article_images
  DROP CONSTRAINT article_images_cleanup_claim_check,
  DROP COLUMN cleanup_claim_token,
  DROP COLUMN cleanup_claim_expires_at;
