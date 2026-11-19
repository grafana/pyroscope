# Rails + Puma clustered mode (`preload_app!`) Example

This example profiles a Rails app served by Puma in clustered mode with
`preload_app!` enabled, which is the configuration where the Ruby SDK is most
often set up incorrectly.

The short version: **call `Pyroscope.configure` in the process that serves
requests, after the fork.** Never before it.

To run the example run the following commands:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker compose up --build
```

Navigate to [Grafana](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=ruby-puma-cluster&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds)
to explore profiles. Group by the `role` tag to see one series per Puma worker.

The stack runs two copies of the same app:

- `cluster`: `WEB_CONCURRENCY=4`, so `workers 4` and `preload_app!`. Reports as
  `role=worker-0` through `role=worker-3`.
- `single`: `WEB_CONCURRENCY=0`, so Puma single mode and no fork at all.
  Reports as `role=single`.

## Why the fork matters

The Ruby SDK samples the process by walking its own memory from a background
thread, and it ships profiles from a second background thread. `fork(2)` only
copies the calling thread, so **threads that existed before the fork do not
exist in the child**.

With `preload_app!`, Puma boots your whole app (Rails initializers included) in
the master process and then forks one worker per `workers` slot. So a
`Pyroscope.configure` call in `config/initializers/pyroscope.rb` runs in the
master:

- the master gets a sampler, and keeps reporting its own (nearly idle) stacks,
- no worker has a sampler, so **none of the code that actually serves your
  traffic is ever profiled**.

It looks like profiling works, because a series with your service name does
show up. It is just the wrong process.

Configuring in *both* places is worse than configuring in the wrong one. The
native extension is in the memory the worker inherited, so its global logger is
already installed, and installing it again panics inside a function that cannot
unwind. The worker dies with `SIGABRT` while booting, Puma forks a replacement,
and the replacement dies the same way. The result is a crash loop that serves
no requests at all.

## The fix

See [`config/puma.rb`](./config/puma.rb). Configure from a Puma hook that runs
inside the worker:

```ruby
before_worker_boot do |index|
  Pyroscope.configure do |config|
    config.application_name = "ruby-puma-cluster"
    config.server_address = "http://pyroscope:4040"
    config.report_pid = true
    config.tags = { "role" => "worker-#{index}" }
  end

  Pyroscope.initialize_rails_hooks
end
```

Notes on the snippet:

- `before_worker_boot` is the Puma 7 name. On Puma 6 and older use
  `on_worker_boot`, which is the same hook (in Puma 7 it is kept as a
  deprecated alias). Either way the block runs in the worker, after the fork.
- Do not leave a `Pyroscope.configure` call in an initializer as well. Remove it.
- `report_pid = true` adds a `pid` label, which is enough to tell the workers
  apart. A per-worker tag such as `role` on top of it gives you a stable,
  readable value to group by, since pids change on every restart.
- `Pyroscope.initialize_rails_hooks` is needed here specifically because of
  `preload_app!`. The gem installs its controller tagging from a railtie
  `after_initialize` hook, which runs during preload in the master, where there
  is no configuration yet, so it skips itself. Calling it in the worker restores
  the `action` tag (for example `action=work/fast`). Skip this line if you set
  `config.autoinstrument_rails = false`.

## Single process mode still works

`before_worker_boot` is cluster-only: Puma never calls it when `workers` is `0`.
This example keeps both paths working by branching on the worker count:

```ruby
if workers_count.zero?
  after_booted { start_pyroscope.call("single") }
else
  before_worker_boot { |index| start_pyroscope.call("worker-#{index}") }
end
```

`after_booted` (`on_booted` on older Puma) runs in the single serving process in
single mode, so there is no fork to worry about. In clustered mode it would run
in the master, which is why the branch matters.

If you only ever run single mode, a plain `config/initializers/pyroscope.rb` is
fine. The `single` service in this example is here to show that the clustered
configuration above does not break the no-worker case.

## Other forking setups

The rule generalises to anything that forks: **configure in the child, and only
in the child.** The parent must not have configured first.

Sidekiq, when `concurrency` is served by a single process, does not fork, so an
initializer is fine. If you run Sidekiq Enterprise / swarm or any supervisor
that forks workers, configure from the server hook that runs in the worker
process:

```ruby
Sidekiq.configure_server do |config|
  config.on(:startup) do
    Pyroscope.configure do |pyroscope|
      pyroscope.application_name = "my-sidekiq"
      pyroscope.server_address = "http://pyroscope:4040"
      pyroscope.report_pid = true
      pyroscope.tags = { "component" => "sidekiq" }
    end
  end
end
```

Note that `Sidekiq.configure_server` does not run in a Puma process at all, so a
web app and its workers can each configure their own agent safely.

For hand-rolled forking, such as a prefork supervisor or a Resque-style
`fork`-per-job loop, the same shape applies:

```ruby
pid = fork do
  Pyroscope.configure do |config|
    config.application_name = "my-worker"
    config.server_address = "http://pyroscope:4040"
    config.report_pid = true
  end

  do_work
end
```

Things to avoid in any forking setup:

- configuring in the parent and relying on the child to inherit it. It will not;
  the sampler thread is gone.
- configuring in the parent *and* the child. The child aborts.
- calling `Pyroscope.configure` more than once in one process. Same abort, no
  fork needed.

There is no supported way to profile a parent and its children from one
`require`. Profile the children, which is where the work happens, and leave the
parent unprofiled.
