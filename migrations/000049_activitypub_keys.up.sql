-- ActivityPub actor key material. One RSA-2048 keypair per participant.
-- We generate lazily on first access and store both keys in the row;
-- the public key is served on the actor document so remote instances
-- can verify signed requests.
--
-- Handle: we prefer an explicit handle (lowercased, URL-safe) but will
-- fall back to a slugified display_name. Unique across the instance.

ALTER TABLE participants ADD COLUMN ap_handle VARCHAR(64) UNIQUE;
ALTER TABLE participants ADD COLUMN ap_public_key TEXT;
ALTER TABLE participants ADD COLUMN ap_private_key TEXT;

CREATE INDEX idx_participants_ap_handle ON participants(ap_handle) WHERE ap_handle IS NOT NULL;
