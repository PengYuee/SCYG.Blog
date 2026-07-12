CREATE TABLE article_images (
  id VARCHAR(32) PRIMARY KEY CONSTRAINT article_images_id_check CHECK (id ~ '^[0-9a-f]{32}$'),
  owner_id VARCHAR(32) NOT NULL CONSTRAINT article_images_owner_id_check CHECK (owner_id ~ '^[0-9a-f]{32}$'),
  storage_key VARCHAR(36) NOT NULL UNIQUE CONSTRAINT article_images_storage_key_check CHECK (storage_key ~ '^[0-9a-f]{32}\.(jpg|png)$'),
  media_type VARCHAR(10) NOT NULL CONSTRAINT article_images_media_type_check CHECK (media_type IN ('image/jpeg', 'image/png')),
  byte_size BIGINT NOT NULL CONSTRAINT article_images_byte_size_check CHECK (byte_size BETWEEN 1 AND 5242880),
  width INTEGER NOT NULL CONSTRAINT article_images_width_check CHECK (width BETWEEN 1 AND 8192),
  height INTEGER NOT NULL CONSTRAINT article_images_height_check CHECK (height BETWEEN 1 AND 8192),
  CONSTRAINT article_images_pixels_check CHECK (width::bigint * height::bigint <= 25000000),
  sha256 VARCHAR(64) NOT NULL CONSTRAINT article_images_sha256_check CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  status VARCHAR(10) NOT NULL CONSTRAINT article_images_status_check CHECK (status IN ('pending', 'committed', 'orphaned')),
  created_at TIMESTAMPTZ NOT NULL,
  committed_at TIMESTAMPTZ NULL,
  orphaned_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT article_images_timestamps_check CHECK (
    expires_at > created_at AND
    ((status = 'pending' AND committed_at IS NULL AND orphaned_at IS NULL) OR
     (status = 'committed' AND committed_at IS NOT NULL AND orphaned_at IS NULL AND committed_at >= created_at) OR
     (status = 'orphaned' AND committed_at IS NOT NULL AND orphaned_at IS NOT NULL AND committed_at >= created_at AND orphaned_at >= committed_at))
  )
);
CREATE INDEX article_images_status_expires_at_idx ON article_images (status, expires_at) WHERE status = 'pending';
CREATE INDEX article_images_status_orphaned_at_idx ON article_images (status, orphaned_at) WHERE status = 'orphaned';

CREATE TABLE article_image_references (
  article_id BIGINT NOT NULL,
  image_id VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT article_image_references_pkey PRIMARY KEY (article_id, image_id),
  CONSTRAINT article_image_references_article_id_fkey FOREIGN KEY (article_id) REFERENCES "Article" ("Id") ON DELETE CASCADE,
  CONSTRAINT article_image_references_image_id_fkey FOREIGN KEY (image_id) REFERENCES article_images (id) ON DELETE CASCADE
);
CREATE INDEX article_image_references_image_id_idx ON article_image_references (image_id);
