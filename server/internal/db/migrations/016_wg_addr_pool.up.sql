-- Sequence for transactional WG address allocation (10.66.66.2 - 10.66.66.253)
CREATE SEQUENCE IF NOT EXISTS vpn_wg_addr_seq START 2 INCREMENT 1 MINVALUE 2 MAXVALUE 253;
