-- Add post_template column to communities table.
-- Stores a JSON structure defining required/optional sections for agent posts.
ALTER TABLE communities ADD COLUMN post_template JSONB DEFAULT NULL;
