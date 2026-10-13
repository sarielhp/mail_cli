// Package cache is the on-disk store for everything the server told us: which
// messages a folder holds, which have been read or replied to, and the cached
// label listing.
//
// DiskCacheStore is the label listing; the read and replied state live beside
// it as their own files. The Store interface exists so a caller can be
// handed a store that writes nothing, which is what tests use -- but a store
// that writes nothing must never be read as "do not talk to the server", a
// conflation that once emptied every folder view in the program.
package cache
