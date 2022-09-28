# postgres-skip-locked-surprise
A reproduction of some surprising and possibly buggy behaviour in postgres' skip locked

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
