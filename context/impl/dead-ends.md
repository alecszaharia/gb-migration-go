# Dead Ends

Build site: context/plans/build-site.md

- REPEATABLE READ batch transactions with per-block SAVEPOINT rollback + concurrent range workers: rolling back an insert at the end of compiled_data's PK leaves an inherited X gap lock on supremum; two workers deadlock on insert-intention (1213), whole tx rolled back, ROLLBACK TO SAVEPOINT then fails 1305. Use READ COMMITTED + batch retry on 1213/1205 (commit 183b5df).
