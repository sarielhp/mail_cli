//go:build !unix

package dirlock

func syncDir(_ string) error {
	return nil
}
