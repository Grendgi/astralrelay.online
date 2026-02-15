-- Add optional display name for devices (user-editable label)
ALTER TABLE devices ADD COLUMN IF NOT EXISTS name TEXT;
