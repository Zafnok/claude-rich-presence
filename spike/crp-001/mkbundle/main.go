// mkbundle <srcdir> <out.mcpb>: zip a directory, marking everything under server/ executable.
package main

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	src, out := os.Args[1], os.Args[2]
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		info, _ := d.Info()
		h, _ := zip.FileInfoHeader(info)
		h.Name = rel
		h.Method = zip.Deflate
		if strings.HasPrefix(rel, "server/") {
			h.SetMode(0o755)
		} else {
			h.SetMode(0o644)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err != nil {
		panic(err)
	}
}
