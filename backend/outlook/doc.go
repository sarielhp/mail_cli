// Package outlook implements the mail client interfaces against the Microsoft
// Graph API.
//
// Graph paginates with @odata.nextLink and caps a page at $top; a bulk
// operation therefore loops until the folder is empty rather than acting on the
// first page and reporting success.
package outlook
