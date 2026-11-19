package main

// Replay dump file format versions.
//
// Version history:
//
//	1 - Initial format. Header fields: version, source_query, tenants,
//	    from_unix_milli, to_unix_milli, created_at_unix_milli.
//	    Records are timestamp-ordered in practice but the header carries
//	    no explicit indication of this.
//
//	2 - Records are guaranteed to be written in ascending timestamp order
//	    (implicit in the version number; no extra header field). Consumers
//	    can rely on this for bounded-memory streaming replay without
//	    buffering the whole file first.
const replayFormatVersion = 2

// replayMinSupportedVersion is the oldest version this binary can read.
// Bumping this drops support for files produced by older releases.
const replayMinSupportedVersion = 1
