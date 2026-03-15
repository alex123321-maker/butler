-- Add summary column to sessions table for LLM-generated session summaries.
-- The memory pipeline worker writes this after each run extraction.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
