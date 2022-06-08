<p align="center">
  <img width="500" src="https://user-images.githubusercontent.com/4546722/166553238-ae7b1ffb-a709-4196-b207-16055c3e1bc3.png">
</p>

---

[![Go Reference](https://pkg.go.dev/badge/github.com/polarsignals/arcticdb.svg)](https://pkg.go.dev/github.com/polarsignals/arcticdb)
[![Go Report Card](https://goreportcard.com/badge/github.com/polarsignals/arcticdb)](https://goreportcard.com/report/github.com/polarsignals/arcticdb)
![Build](https://github.com/polarsignals/arcticdb/actions/workflows/go.yml/badge.svg)
![Discord](https://img.shields.io/discord/813669360513056790?label=Discord)

> This project is still in its infancy, consider it not production-ready, probably has various consistency and correctness problems and all API will change!

ArcticDB is an embeddable columnar database written in Go. It features semi-structured schemas (could also be described as typed wide-columns), and uses [Apache Parquet](https://parquet.apache.org/) for storage, and [Apache Arrow](https://arrow.apache.org/) at query time. Building on top of Apache Arrow, ArcticDB provides a query builder and various optimizers (it reminds of DataFrame-like APIs).

ArcticDB is optimized for use cases where the majority of interactions are writes, and when data is queried, a lot of data is queried at once (our use case at Polar Signals can be broadly described as Observability and specifically for [Parca](https://parca.dev/)). It could also be described as a wide-column columnar database.

Read the annoucement blog post to learn about what made us create it: https://www.polarsignals.com/blog/posts/2022/05/04/introducing-arcticdb/

## Why you should use ArcticDB

Columnar data stores have become incredibly popular for analytics data. Structuring data in columns instead of rows leverages the architecture of modern hardware, allowing for efficient processing of data.
A columnar data store might be right for you if you have workloads where you write a lot of data and need to perform analytics on that data.

ArcticDB is similar to many other in-memory columnar databases such as [DuckDB](https://duckdb.org/) or [InfluxDB IOx](https://github.com/influxdata/influxdb_iox). 

ArcticDB may be a better fit for you if:
- Are developing a Go program
- Want to embed a columnar database in your program instead of running a separate server
- Have immutable datasets that don't require updating or deleting
- Your data contains dynamic columns, where a column may expand during runtime

ArcticDB is likely not suitable for your needs if:
- You aren't developing in Go
- You require a standalone database server
- You need to modify or delete your data
- You query by rows instead of columns

## Getting Started

You can explore the [examples](https://github.com/polarsignals/arcticdb/tree/main/examples) directory for sample code using ArcticDB. Below is a snippet from the simple database example. It creates a database with a dynamic column schema, inserts some data, and queries it back out.

```go
// Create a new column store
columnstore := arcticdb.New(
    prometheus.NewRegistry(),
    8192,
    10*1024*1024, // 10MiB
)

// Open up a database in the column store
database, _ := columnstore.DB("simple_db")

// Define our simple schema of labels and values
schema := simpleSchema()

// Create a table named simple in our database
table, _ := database.Table(
    "simple_table",
    arcticdb.NewTableConfig(schema),
    log.NewNopLogger(),
)

// Create values to insert into the database these first rows havel dynamic label names of 'firstname' and 'surname'
buf, _ := schema.NewBuffer(map[string][]string{
    "names": {"firstname", "surname"},
})

// firstname:Frederic surname:Brancz 100
buf.WriteRow([]parquet.Value{
    parquet.ValueOf("Frederic").Level(0, 1, 0),
    parquet.ValueOf("Brancz").Level(0, 1, 1),
    parquet.ValueOf(100).Level(0, 0, 2),
})

// firstname:Thor surname:Hansen 10
buf.WriteRow([]parquet.Value{
    parquet.ValueOf("Thor").Level(0, 1, 0),
    parquet.ValueOf("Hansen").Level(0, 1, 1),
    parquet.ValueOf(10).Level(0, 0, 2),
})
table.InsertBuffer(buf)

// Now we can insert rows that have middle names into our dynamic column
buf, _ = schema.NewBuffer(map[string][]string{
    "names": {"firstname", "middlename", "surname"},
})
// firstname:Matthias middlename:Oliver surname:Loibl 1
buf.WriteRow([]parquet.Value{
    parquet.ValueOf("Matthias").Level(0, 1, 0),
    parquet.ValueOf("Oliver").Level(0, 1, 1),
    parquet.ValueOf("Loibl").Level(0, 1, 2),
    parquet.ValueOf(1).Level(0, 0, 3),
})
table.InsertBuffer(buf)

// Create a new query engine to retrieve data and print the results
engine := query.NewEngine(memory.DefaultAllocator, database.TableProvider())
engine.ScanTable("simple_table").
    Filter(
        logicalplan.Col("names.firstname").Eq(logicalplan.Literal("Frederic")),
    ).Execute(context.Background(), func(r arrow.Record) error {
    fmt.Println(r)
    return nil
})
```

## Design choices

ArcticDB was specifically built for Observability workloads. This resulted in several characteristics that make it unique in its combination.

Table Of Contents:

* [Columnar Layout](#columnar-layout)
* [Dynamic Columns](#dynamic-columns)
* [Immutable & Sorted](#immutable--sorted)
* [Snapshot isolation](#snapshot-isolation)

### Columnar layout

Observability data is most useful when highly dimensional and those dimensions can be searched and aggregated by efficiently. Contrary to many relational databases like (MySQL, PostgreSQL, CockroachDB, TiDB, etc.) that store data all data belonging to a single row together, in a columnar layout all data of the same column in a table is available in one contiguous chunk of data, making it very efficient to scan and more importantly, only the data truly necessary for a query is loaded in the first place. ArcticDB uses [Apache Parquet](https://parquet.apache.org/) for storage, and [Apache Arrow](https://arrow.apache.org/) at query time. Apache Parquet is used for storage to make use of its efficient encodings to save on memory and disk space. Apache Arrow is used at query time as a foundation to vectorize the query execution.

### Dynamic Columns

While columnar databases already exist, most require a static schema, however, Observability workloads differ in that their schemas are not static, meaning not all columns are pre-defined. On the other hand, wide column databases also already exist, but typically are not strictly typed, and most wide-column databases are row-based databases, not columnar databases.

Take a [Prometheus](https://prometheus.io/) time-series for example. Prometheus time-series are uniquely identified by the combination of their label-sets:

```
http_requests_total{path="/api/v1/users", code="200"} 12
```

This model does not map well into a static schema, as label-names cannot be known upfront. The most suitable data-type some columnar databases have to offer is a map, however, maps have the same problems as row-based databases, where all values of a map in a row are stored together, unable to exploit the advantages of a columnar layout. An ArcticDB schema can define a column to be dynamic, causing a column to be created on the fly when a new label-name is seen.

An ArcticDB schema for Prometheus could look like this:

```go
package arcticprometheus

import (
	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/segmentio/parquet-go"
)

func Schema() *dynparquet.Schema {
	return dynparquet.NewSchema(
		"prometheus",
		[]dynparquet.ColumnDefinition{{
			Name:          "labels",
			StorageLayout: parquet.Encoded(parquet.Optional(parquet.String()), &parquet.RLEDictionary),
			Dynamic:       true,
		}, {
			Name:          "timestamp",
			StorageLayout: parquet.Int(64),
			Dynamic:       false,
		}, {
			Name:          "value",
			StorageLayout: parquet.Leaf(parquet.DoubleType),
			Dynamic:       false,
		}},
		[]dynparquet.SortingColumn{
			dynparquet.NullsFirst(dynparquet.Ascending("labels")),
			dynparquet.Ascending("timestamp"),
		},
	)
}
```

> Note: We are aware that Prometheus uses double-delta encoding for timestamps and XOR encoding for values. This schema is purely an example to highlight the dynamic columns feature.

With this schema, all rows are expected to have a `timestamp` and a `value` but can vary in their columns prefixed with `labels.`. In this schema all dynamically created columns are still Dictionary and run-length encoded and must be of type `string`.

### Immutable & Sorted

There are only writes and reads. All data is immutable and sorted. Having all data sorted allows ArcticDB to avoid maintaining an index per column, and still serve queries with low latency.

To maintain global sorting ArcticDB requires all inserts to be sorted if they contain multiple rows. Combined with immutability, global sorting of all data can be maintained at a reasonable cost. To optimize throughput, it is preferable to perform inserts in as large batches as possible. ArcticDB maintains inserted data in batches of a configurable amount of rows (by default 8192), called a _Granule_. To directly jump to data needed for a query, ArcticDB maintains a sparse index of Granules. The sparse index is small enough to fully reside in memory, it is currently implemented as a [b-tree](https://github.com/google/btree) of Granules.

![Sparse index of Granules](https://docs.google.com/drawings/d/1DbGqLKsloKAEG7ydJ5n5-Vr03j4jQMqdipJyEu0goIE/export/svg)

At insert time, ArcticDB splits the inserted rows into the appropriate Granule according to their lower and upper bound, to maintain global sorting. Once a Granule exceeds the configured amount, the Granule is split into `N` new Granules depending.

![Split of Granule](https://docs.google.com/drawings/d/1c38HQfpTPVtzatGenQaqF7oA_7NiEDbfeudxiUV5lSg/export/svg)

Under the hood, Granules are a list of sorted Parts, and only if a query requires it are all parts merged into a sorted stream using a [direct k-way merge](https://en.wikipedia.org/wiki/K-way_merge_algorithm#Direct_k-way_merge) using a [min-heap](https://en.wikipedia.org/wiki/Binary_heap). An example of an operation that requires the whole Granule to be read as a single sorted stream are the aforementioned Granule splits.

![A Granule is organized in Parts](https://docs.google.com/drawings/d/1Ex4hKLwoQ_IgYARj0aEjoFEjQRt6-B0fO8K9E7syyHc/export/svg)

### Snapshot isolation

ArcticDB has snapshot isolation, however, it comes with a few caveats that should be well understood. It does not have read-after-write consistency as the intended use is for users reading data that are not the same as the entity writing data to it. To see new data the user re-runs a query. Choosing to trade-off read-after-write consistency allows for mechanisms to increase throughput significantly. ArcticDB releases write transactions in batches. It essentially only ensures write atomicity and that writes are not torn when reading. Since data is immutable, those characteristics together result in snapshot isolation. 

More concretely, arcticDB maintains a watermark indicating that all transactions equal and lower to the watermark are safe to be read. Only write transactions obtain a _new_ transaction ID, while reads use the transaction ID of the watermark to identify data that is safe to be read. The watermark is only increased when strictly monotonic, consecutive transactions have finished. This means that a low write transaction can block higher write transactions to become available to be read. To ensure progress is made, write transactions have a timeout.

This mechanism inspired by a mix of [Google Spanner](https://research.google/pubs/pub39966/), [Google Percolator](https://research.google/pubs/pub36726/) and [Highly Available Transactions](https://www.vldb.org/pvldb/vol7/p181-bailis.pdf).

![Transactions are released in batches indicated by the watermark](https://docs.google.com/drawings/d/1qmcMg9sXnDZix9eWSvOtWJD06yHsLpgho8M-DGF84bU/export/svg)

## Roadmap

* Persistence: ArcticDB is currently fully in-memory.

## Acknowledgments

ArcticDB stands on the shoulders of giants. Shout out to Segment for creating the incredible [`parquet-go`](https://github.com/segmentio/parquet-go) library as well as InfluxData for starting and various contributors after them working on [Go support for Apache Arrow](https://pkg.go.dev/github.com/apache/arrow/go/arrow).
