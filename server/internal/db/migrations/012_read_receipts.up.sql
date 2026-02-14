-- Read receipts: when recipient views a message, mark it as read
CREATE TABLE IF NOT EXISTS read_receipts (
    event_id    TEXT NOT NULL REFERENCES message_queue(event_id) ON DELETE CASCADE,
    reader_addr TEXT NOT NULL,
    read_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, reader_addr)
);

CREATE INDEX idx_read_receipts_reader ON read_receipts(reader_addr);
