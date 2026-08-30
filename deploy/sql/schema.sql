CREATE TABLE IF NOT EXISTS poems (
  id VARCHAR(64) PRIMARY KEY,
  title VARCHAR(512) NOT NULL,
  author VARCHAR(255) NOT NULL,
  dynasty VARCHAR(32) NOT NULL,
  kind VARCHAR(24) NOT NULL,
  form VARCHAR(64) NOT NULL,
  cipai VARCHAR(255) NOT NULL DEFAULT '',
  content_text MEDIUMTEXT NOT NULL,
  lines_json JSON NOT NULL,
  pinyin_json JSON NOT NULL,
  translation TEXT NOT NULL,
  annotations_json JSON NOT NULL,
  appreciation MEDIUMTEXT NOT NULL,
  themes_json JSON NOT NULL,
  collections_json JSON NOT NULL,
  age_min TINYINT UNSIGNED NOT NULL,
  age_max TINYINT UNSIGNED NOT NULL,
  popular_score INT NOT NULL DEFAULT 0,
  content_hash CHAR(64) NOT NULL,
  source_name VARCHAR(64) NOT NULL,
  source_url VARCHAR(255) NOT NULL,
  source_commit VARCHAR(64) NOT NULL,
  source_id VARCHAR(128) NOT NULL,
  license_note VARCHAR(512) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_poems_content_hash (content_hash),
  KEY idx_poems_dynasty (dynasty),
  KEY idx_poems_author (author),
  KEY idx_poems_title (title),
  KEY idx_poems_form (form),
  KEY idx_poems_cipai (cipai),
  KEY idx_poems_popular (popular_score DESC, id ASC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS poem_tags (
  poem_id VARCHAR(64) NOT NULL,
  dimension VARCHAR(32) NOT NULL,
  value VARCHAR(128) NOT NULL,
  PRIMARY KEY (poem_id, dimension, value),
  KEY idx_poem_tags_lookup (dimension, value, poem_id),
  CONSTRAINT fk_poem_tags_poem FOREIGN KEY (poem_id) REFERENCES poems(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS dataset_imports (
  version VARCHAR(64) PRIMARY KEY,
  object_key VARCHAR(255) NOT NULL,
  record_count INT NOT NULL,
  sha256 CHAR(64) NOT NULL,
  imported_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
