# Banking Transactions — Observer & Visitor (Topic 11)

Go 1.21+. Run: `go mod init banking && go test -race ./... && go run .`

## Design decisions

| Decision | Choice | Why |
|---|---|---|
| Push vs pull | Push | Fraud and limit checks must react the moment a transaction arrives; pull would add polling latency and useless load. |
| Traversal owner | Visitor (`visitAll`) | Element classes hold data only; walking logic lives in one helper, so every visitor traverses identically and elements stay trivial. |
| `Visitor<R>` in Go | Void `Visit*` methods + typed `Result()` per visitor | Go interfaces cannot parameterise `Accept` with a type variable; a typed `Result()` is the direct Go equivalent of Java's `R`. |
| Thread safety | `RWMutex` + subscriber snapshot | Publish holds only a read lock to copy the slice, then runs handlers unlocked: no whole-subject lock, and handlers may (un)subscribe safely, even themselves. |
| ExecutorService equivalent | Goroutines + `sync.WaitGroup` | `producerCount` is the configurable thread count; `wg.Wait()` is the graceful shutdown: `Publish` is synchronous, so no event can be lost and no goroutine outlives main. |
| Construction | Single composition root in `main`, constructor injection only | Clean Code ch. 11: no lazy init, no service locators; every dependency is passed via a constructor. |

