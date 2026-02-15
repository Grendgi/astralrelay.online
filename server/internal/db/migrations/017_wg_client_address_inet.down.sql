DROP INDEX IF EXISTS idx_vpn_peers_client_address_net;

ALTER TABLE vpn_peers ALTER COLUMN client_address TYPE text USING (
  CASE WHEN client_address IS NULL THEN NULL
       ELSE client_address::text
  END
);
