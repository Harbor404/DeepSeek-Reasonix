// Package decisionstore persists immutable decision request snapshots in a
// SQLite file. A snapshot is an opaque byte payload bound to an optional parent
// version; the package knows nothing about its shape. Entries expire after
// TTL, at most Capacity are kept, and every read verifies the stored digest and
// record checksum. Failures are typed sentinels the caller classifies with
// errors.Is.
package decisionstore
