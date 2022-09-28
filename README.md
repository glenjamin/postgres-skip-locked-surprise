# postgres-skip-locked-surprise
A reproduction of some surprising and possibly buggy behaviour in postgres' skip locked

## Explanation

This example is extracted from some real code which demonstrated some surprising behaviour
when using `SKIP LOCKED` which appears to show multiple transactions matching the same data
when they should not. It is a fairly minimal scenario, but as the bug only manifests
intermittently when running concurrent transactions I've used Go to run the queries as part
of a test suite.

The schema itself isn't really optimal for the task at hand, but is inherited from before the
code was changed to use `SKIP LOCKED` to make it act as a queue, and as far as I can tell it
should still work as expected.

We have a table of `accounts` which have associated `imports`. Each account always has an
`initial` import, and will eventually have an `incremental` import. The relevant code implements
queue-like behaviour, with multiple processes looking for accounts with stale data, excluding any
accounts which have imports which do not have a status of `completed`. When a worker identifies
accounts which are overdue for an update, the `incremental` import is set to a `pending` status
(creating a new row if one didn't previously exist) and an RPC call is made to begin processing
that import.

My understanding of the combination of `FOR UPDATE` and `SKIP LOCKED` should mean that worker
processes all select disjoint subsets of rows, and therefore we should never get duplicate messages.
The surprising and possibly buggy behaviour is that we do occasionally get duplicate messages
produced when running a bunch of concurrent workers. I'm unsure whether there is actually a postgres
bug here, or whether I've overlooked something in the way these concurrent transactions interact.

The closest thing I've found to an answer for what's going on is that if I remove the requirement
that some accounts do not yet have records for `incremental` imports, change the query to use
`INNER` joins for both cases, and include the `incr` join in the lock - then I can no longer
reproduce the problem. However, I'm unable to follow why this makes a difference.

## Running

Start a postgres instance
```
docker compose up -d
```

Run the general tests which should pass
```
go test .
```

Run a soak test to (sometimes) trigger the unexpected behaviour
```
go test . -run '/race' -count=100
```
