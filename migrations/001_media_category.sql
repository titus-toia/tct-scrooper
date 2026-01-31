-- Add category and location columns to media table for S3 path generation
-- Run this migration against your database

ALTER TABLE media
ADD COLUMN IF NOT EXISTS category TEXT DEFAULT 'listing',
ADD COLUMN IF NOT EXISTS province TEXT DEFAULT '',
ADD COLUMN IF NOT EXISTS city TEXT DEFAULT '';

-- Add index for filtering by category
CREATE INDEX IF NOT EXISTS idx_media_category ON media(category);

-- Update existing records to have category based on context
-- (existing media without category is assumed to be listing photos)
UPDATE media SET category = 'listing' WHERE category IS NULL;

COMMENT ON COLUMN media.category IS 'Media category: listing, agent, brokerage, property';
COMMENT ON COLUMN media.province IS 'Province/state for S3 path (listings/properties only)';
COMMENT ON COLUMN media.city IS 'City for S3 path (listings/properties only)';
