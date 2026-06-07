//go:build !linux

package desktop

func Install(iconPNG []byte) error {
	return nil
}
