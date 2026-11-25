BEGIN;

CREATE SCHEMA loomspan_cloud;
REVOKE ALL ON SCHEMA loomspan_cloud FROM PUBLIC;

CREATE TABLE loomspan_cloud.applications (
    id text PRIMARY KEY CHECK (id <> ''),
    namespace text NOT NULL UNIQUE CHECK (namespace ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' AND length(namespace) <= 64),
    trace_persistence text NOT NULL DEFAULT 'ONERROR' CHECK (trace_persistence IN ('NEVER', 'ONERROR', 'ALWAYS')),
    current_revision_id text,
    current_publication_version text
);

CREATE TABLE loomspan_cloud.stage_revisions (
    application_id text NOT NULL REFERENCES loomspan_cloud.applications (id),
    revision_id text NOT NULL CHECK (revision_id <> ''),
    pack_digest text NOT NULL CHECK (pack_digest ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (application_id, revision_id),
    UNIQUE (application_id, revision_id, pack_digest)
);

CREATE TABLE loomspan_cloud.app_variables (
    application_id text NOT NULL REFERENCES loomspan_cloud.applications (id),
    name text COLLATE "C" NOT NULL CHECK (name <> ''),
    value text NOT NULL,
    PRIMARY KEY (application_id, name)
);

CREATE TABLE loomspan_cloud.publications (
    application_id text NOT NULL,
    version text NOT NULL UNIQUE CHECK (version <> ''),
    revision_id text NOT NULL,
    pack_digest text NOT NULL,
    status text NOT NULL CHECK (status IN ('started', 'acknowledged', 'not-sent', 'rejected', 'unconfirmed')),
    PRIMARY KEY (application_id, version),
    FOREIGN KEY (application_id, revision_id, pack_digest)
        REFERENCES loomspan_cloud.stage_revisions (application_id, revision_id, pack_digest)
);

ALTER TABLE loomspan_cloud.applications
    ADD FOREIGN KEY (id, current_revision_id) REFERENCES loomspan_cloud.stage_revisions (application_id, revision_id),
    ADD FOREIGN KEY (id, current_publication_version) REFERENCES loomspan_cloud.publications (application_id, version);

CREATE TABLE loomspan_cloud.install_attempts (
    id text PRIMARY KEY CHECK (id <> ''),
    application_id text NOT NULL,
    version text NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('publish', 'restore')),
    settings jsonb NOT NULL CHECK (jsonb_typeof(settings) = 'object'),
    pod_uid text NOT NULL,
    instance_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('started', 'acknowledged', 'not-sent', 'rejected', 'unconfirmed')),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    FOREIGN KEY (application_id, version) REFERENCES loomspan_cloud.publications (application_id, version),
    CHECK ((status = 'started') = (finished_at IS NULL))
);

COMMIT;
