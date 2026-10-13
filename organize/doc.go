// Package organize archives mail: moving everything under a label prefix into
// the account's archive folder.
//
// It fans out over a subtree rather than naming one folder, which is why it
// asks ResolveAll for every folder under the prefix instead of asking which one
// folder a name means.
package organize
