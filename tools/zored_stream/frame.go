package main

// frame renders only when the key changes and hands back the cached image otherwise, so resends (A4) still set it.
type frame struct {
	key, image string
}

func (f *frame) render(key string, draw func() string) string {
	if key != f.key || f.image == "" {
		f.key, f.image = key, draw()
	}
	return f.image
}
