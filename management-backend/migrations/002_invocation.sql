BEGIN;

ALTER TABLE loomspan_cloud.applications
    ADD COLUMN show_execution_details boolean NOT NULL DEFAULT false;

CREATE TABLE loomspan_cloud.application_invoke_users (
    application_id text NOT NULL REFERENCES loomspan_cloud.applications (id),
    subject text COLLATE "C" NOT NULL CHECK (length(btrim(subject)) > 0),
    roles text[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (application_id, subject)
);

CREATE TABLE loomspan_cloud.application_invoke_backends (
    application_id text NOT NULL REFERENCES loomspan_cloud.applications (id),
    backend_id text COLLATE "C" NOT NULL CHECK (length(btrim(backend_id)) > 0),
    PRIMARY KEY (application_id, backend_id)
);

COMMIT;
