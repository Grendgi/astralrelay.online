-- Store client_address as inet/cidr for efficient network queries
-- (overlap, containment, subnet lookups without text parsing)
ALTER TABLE vpn_peers ALTER COLUMN client_address TYPE inet USING (
  CASE WHEN client_address IS NULL OR TRIM(client_address) = '' THEN NULL
       ELSE client_address::inet
  END
);

-- GIST index for network operators: && (overlap), >> (contains), etc.
CREATE INDEX idx_vpn_peers_client_address_net ON vpn_peers USING gist (client_address inet_ops)
WHERE client_address IS NOT NULL;
