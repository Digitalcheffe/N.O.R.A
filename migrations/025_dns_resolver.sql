-- Add optional custom DNS resolver server per DNS check.
-- When set (e.g. "8.8.8.8" or "10.96.96.22"), RunDNS will query that server
-- instead of the container's default resolver.
ALTER TABLE monitor_checks ADD COLUMN dns_resolver TEXT;
