-- Drop the orphaned outbox_events table. Billing is synchronous
-- (Reserve -> Settle -> Release) and no code writes outbox rows; the table
-- existed only for historical compatibility.
DROP TABLE IF EXISTS outbox_events;
