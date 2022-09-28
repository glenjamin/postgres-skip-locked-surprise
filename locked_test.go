package main

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestLocked(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres@localhost:5432/postgres"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	assert.NilError(t, err)
	t.Cleanup(func() { pool.Close() })

	setupSchema(ctx, t, pool)

	t.Run("two runs find matches, then queue is empty", func(t *testing.T) {
		resetData(ctx, t, pool)

		accounts, commit, err := findOverdue(ctx, t, pool)
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(accounts, []string{"one"}))
		assert.NilError(t, commit())

		accounts, commit, err = findOverdue(ctx, t, pool)
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(accounts, []string{"two"}))
		assert.NilError(t, commit())

		accounts, commit, err = findOverdue(ctx, t, pool)
		assert.NilError(t, err)
		assert.Check(t, cmp.Len(accounts, 0))
		assert.NilError(t, commit())
	})

	t.Run("interleaved runs don't compete", func(t *testing.T) {
		resetData(ctx, t, pool)

		accounts1, commit1, err := findOverdue(ctx, t, pool)
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(accounts1, []string{"one"}))

		accounts2, commit2, err := findOverdue(ctx, t, pool)
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(accounts2, []string{"two"}))

		assert.NilError(t, commit1())
		assert.NilError(t, commit2())
	})

	t.Run("races dont duplicate matches", func(t *testing.T) {
		resetData(ctx, t, pool)

		mu := sync.Mutex{}
		wg := &sync.WaitGroup{}
		wg.Add(20)
		var allAccounts []string
		for i := 0; i < 20; i++ {
			go func() {
				defer wg.Done()
				accounts, commit, err := findOverdue(ctx, t, pool)
				mu.Lock()
				defer mu.Unlock()
				assert.Check(t, err)
				allAccounts = append(allAccounts, accounts...)
				if commit != nil {
					_ = commit()
				}
			}()
		}
		wg.Wait()

		assert.Assert(t, cmp.DeepEqual(allAccounts, []string{"one", "two"}))
	})
}

func findOverdue(ctx context.Context, t *testing.T, pool *pgxpool.Pool) ([]string, func() error, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	rows, err := tx.Query(ctx, `SELECT
		id,
		incr.status IS NOT NULL AS has_incr
	FROM
		accounts a
		INNER JOIN imports init ON a.id = init.account_id AND init.kind = 'initial'
		LEFT JOIN imports incr ON a.id = incr.account_id AND incr.kind = 'incremental'
	WHERE
	    last_updated < NOW() - INTERVAL '10 minutes'
	AND init.status = 'completed'
	AND (incr.status = 'completed' OR incr.status IS NULL)
	ORDER BY last_updated ASC
	LIMIT 1
	FOR UPDATE OF a, init
	SKIP LOCKED`)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}

	var toUpdate []string
	var toInsert []string
	for rows.Next() {
		var hasIncr bool
		var account string
		if err := rows.Scan(&account, &hasIncr); err != nil {
			return nil, nil, err
		}
		if hasIncr {
			toUpdate = append(toUpdate, account)
		} else {
			toInsert = append(toInsert, account)
		}
	}

	accounts := append(toUpdate, toInsert...)

	_, err = tx.Exec(ctx, `UPDATE imports SET status = 'pending' WHERE kind = 'incremental' AND account_id = any($1)`, toUpdate)
	if err != nil {
		_ = tx.Rollback(ctx)
		return accounts, nil, err
	}

	for _, account := range toInsert {
		_, err = tx.Exec(ctx, `INSERT INTO imports(account_id, kind, status) VALUES ($1, 'incremental', 'pending')`, account)
		if err != nil {
			_ = tx.Rollback(ctx)
			return accounts, nil, err
		}
	}

	return accounts, func() error { return tx.Commit(ctx) }, nil
}

func setupSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `DROP TABLE IF EXISTS imports`)
	assert.NilError(t, err)
	_, err = pool.Exec(ctx, `DROP TABLE IF EXISTS accounts`)
	assert.NilError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE accounts(
    	id TEXT PRIMARY KEY,
    	last_updated TIMESTAMP
    )`)
	assert.NilError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE imports(
    	account_id TEXT constraint fk_accounts references accounts on delete cascade,
    	kind TEXT,
    	status TEXT,
    	primary key (account_id, kind)
    )`)
	assert.NilError(t, err)
}

func resetData(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `DELETE FROM accounts`)
	assert.NilError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO
		accounts(id, last_updated)
	VALUES
	    ('one', NOW() - INTERVAL '2 HOUR'),
	    ('two', NOW() - INTERVAL '1 HOUR'),
	    ('three', NOW() - INTERVAL '1 HOUR')`,
	)
	assert.NilError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO
		imports(account_id, kind, status)
	VALUES
	    ('one', 'initial', 'completed'),
	    ('one', 'incremental', 'completed'),
	    ('two', 'initial', 'completed'),
	    -- uncomment the following line to note the problem still occurs without INSERTs
	    ('two', 'incremental', 'completed'),
	    ('three', 'initial', 'failed')
	    `,
	)
	assert.NilError(t, err)
}
