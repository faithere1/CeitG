// mkicon writes a macOS .iconset folder (used by scripts/build.sh).
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"ceitg/internal/icon"
)

func main() {
	dir := os.Args[1]
	os.MkdirAll(dir, 0o755)
	for _, s := range []int{16, 32, 128, 256, 512} {
		for scale := 1; scale <= 2; scale++ {
			name := fmt.Sprintf("icon_%dx%d.png", s, s)
			if scale == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", s, s)
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				panic(err)
			}
			png.Encode(f, icon.Draw(s*scale))
			f.Close()
		}
	}
}
