CREATE TABLE IF NOT EXISTS poem_audio (
  poem_id VARCHAR(64) PRIMARY KEY,
  rank_position INT UNSIGNED NOT NULL,
  object_key VARCHAR(512) NOT NULL,
  voice VARCHAR(128) NOT NULL,
  mime_type VARCHAR(64) NOT NULL DEFAULT 'audio/mpeg',
  byte_size BIGINT UNSIGNED NOT NULL,
  duration_ms INT UNSIGNED NOT NULL,
  sha256 CHAR(64) NOT NULL,
  ranking_source VARCHAR(255) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_poem_audio_object (object_key),
  KEY idx_poem_audio_rank (rank_position),
  CONSTRAINT fk_poem_audio_poem FOREIGN KEY (poem_id) REFERENCES poems(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
