DROP TABLE IF EXISTS imports;
DROP TABLE IF EXISTS accounts;

CREATE TABLE accounts(
 id TEXT PRIMARY KEY,
 last_updated TIMESTAMP
);

CREATE TABLE imports(
    account_id TEXT constraint fk_accounts references accounts on delete cascade,
    status TEXT,
    primary key (account_id)
);

INSERT INTO
    accounts(id, last_updated)
VALUES
    ('one', NOW() - INTERVAL '1 HOUR'),
    ('two', NOW() - INTERVAL '1 HOUR')
;

INSERT INTO
    imports(account_id, status)
VALUES
    ('one', 'completed'),
    ('two', 'failed')
;
