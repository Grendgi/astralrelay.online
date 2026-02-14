-- Optional: federation DB user with minimal grants (isolation)
-- Requires superuser/CREATEROLE for CREATE USER; run manually if migration fails
-- DATABASE_FEDERATION_URL=postgres://messenger_federation:pass@.../messenger
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'messenger_federation') THEN
    CREATE USER messenger_federation WITH PASSWORD 'changeme_federation';
  END IF;
EXCEPTION
  WHEN insufficient_privilege THEN
    RAISE NOTICE 'Run as superuser: CREATE USER messenger_federation WITH PASSWORD ''...'';';
  WHEN duplicate_object THEN
    NULL;
END
$$;
DO $$
DECLARE dbn text := current_database();
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'messenger_federation') THEN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO messenger_federation', dbn);
    GRANT USAGE ON SCHEMA public TO messenger_federation;
    GRANT SELECT ON users, devices, one_time_prekeys TO messenger_federation;
    GRANT SELECT, INSERT ON message_queue TO messenger_federation;
  END IF;
END
$$;
