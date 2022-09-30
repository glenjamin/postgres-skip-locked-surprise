BEGIN;

\set id 'nothing'

SELECT
    id
FROM
    accounts a
        LEFT JOIN imports i ON a.id = i.account_id
WHERE
        last_updated < NOW() - INTERVAL '10 minutes'
  AND i.status = 'completed'
ORDER BY last_updated ASC
LIMIT 1
    FOR UPDATE OF a
        SKIP LOCKED \gset

UPDATE imports SET status = 'pending' WHERE account_id = :'id';

\echo 'Matched' :id

COMMIT;
