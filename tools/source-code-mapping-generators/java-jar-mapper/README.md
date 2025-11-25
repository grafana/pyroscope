# Java Jar Mapper

A Go tool that updates a `.pyroscope.yaml` file with source code
mappings for 3rd party libraries found in a given JAR.

Note this tool relies on multiple 3rd party APIs. Downtime
in these APIs can result in non deterministic output from this tool.

This tool relies on `jar-mappings.json` to resolve common
3rd party libraries that are not properly resolved by the heuristics
in the tool.

## Dependencies

- JDK [for `jar` executable]

## Usage

```bash
go run . [flags]
```

### Flags

- `-jar string`: Path to the Java JAR file to analyze (required)
- `-jdk-version string`: JDK version for JDK function mappings (e.g., '8', '11', '17', '21'). If not specified, JDK mappings will not be generated.
- `-config string`: `.pyroscope.yaml` to modify with new source code mappings. If not specified, a valid `.pyroscope.yaml` will be printed to stdout.
- `-help`: Show help information

### Example

```bash
# Generate .pyroscope.yaml to stdout with source code mappings
go run . -jar /path/to/jar/foo.jar

# Update .pyroscope.yaml with source code mappings and JDK mappings
go run . -jar /path/to/jar/foo.jar -jdk-version 19 -config .pyroscope.yaml
```

## Build

```bash
go build -o java-jar-mapper .
```