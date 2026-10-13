// Package folderindex maintains the local record of which message IDs are in
// which folder, so a listing can be drawn without asking the server.
//
// It was called `label`, which collided with the top-level `label` package that
// owns the shape of a label tree. Two packages of the same name meant every file
// needing both had to alias one, and a mechanical rewrite during the modularity
// work resolved `label.BuildLabelTree` against the wrong one. The names say what
// each is for now: `label` is what a folder *is*, `folderindex` is what is *in*
// one.
//
// No file paths are exposed; callers pass a download directory and a folder name.
package folderindex
