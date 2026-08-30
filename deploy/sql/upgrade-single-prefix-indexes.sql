-- Rebuild the existing author/title indexes as covering indexes so broad
-- one-character prefix searches do not fetch thousands of full poem rows.
ALTER TABLE poems
  DROP INDEX idx_poems_author,
  DROP INDEX idx_poems_title,
  ADD INDEX idx_poems_author (author, popular_score DESC),
  ADD INDEX idx_poems_title (title, popular_score DESC),
  ALGORITHM=INPLACE,
  LOCK=NONE;
