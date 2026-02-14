REVOKE ALL ON message_queue, users, devices, one_time_prekeys FROM messenger_federation;
REVOKE USAGE ON SCHEMA public FROM messenger_federation;
REVOKE CONNECT ON DATABASE messenger FROM messenger_federation;
DROP USER IF EXISTS messenger_federation;
