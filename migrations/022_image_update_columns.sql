-- 022_image_update_columns.sql
-- Adds image update detection columns to discovered_containers for DD-9.
-- image_digest:          manifest digest of the locally running image (from Docker socket)
-- registry_digest:       latest manifest digest from the container registry for the same tag
-- image_update_available: 1 if registry_digest differs from image_digest, 0 if same or unknown
-- image_last_checked_at: when the registry was last polled for this container

ALTER TABLE discovered_containers ADD COLUMN image_digest TEXT;
ALTER TABLE discovered_containers ADD COLUMN registry_digest TEXT;
ALTER TABLE discovered_containers ADD COLUMN image_update_available INTEGER NOT NULL DEFAULT 0;
ALTER TABLE discovered_containers ADD COLUMN image_last_checked_at DATETIME;
