CREATE TABLE audit_log (id bigserial PRIMARY KEY, at timestamptz NOT NULL DEFAULT now(), msg text);
