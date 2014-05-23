package main

import (
	"fmt"
	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/render"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
)

func main() {
	m := martini.Classic()
	m.Use(render.Renderer(render.Options{
		Layout:     "layout",
		Delims:     render.Delims{"{[{", "}]}"},
		Extensions: []string{".html"}}))

	m.Get("/", func(r render.Render) {
		r.HTML(200, "index", nil)
	})

	m.Post("/upload", chunkedReader)

	m.Run()
}

type ByChunk []os.FileInfo

func (a ByChunk) Len() int      { return len(a) }
func (a ByChunk) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByChunk) Less(i, j int) bool {
	ai, _ := strconv.Atoi(a[i].Name())
	aj, _ := strconv.Atoi(a[j].Name())
	return ai < aj
}

func chunkedReader(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(25)

	chunkDirPath := "./incomplete/" + r.FormValue("flowFilename")
	_, err := os.Stat(chunkDirPath)
	if err != nil {
		err := os.MkdirAll(chunkDirPath, 02750)
		if err != nil {
			w.Write([]byte("Error creating tempdir"))
			w.WriteHeader(500)
			return
		}
	}

	for _, fileHeader := range r.MultipartForm.File["file"] {
		src, err := fileHeader.Open()
		if err != nil {
			w.Write([]byte("Error opening file"))
			w.WriteHeader(500)
			return
		}
		defer src.Close()

		if r.FormValue("flowChunkNumber") == r.FormValue("flowTotalChunks") {
			// final chunk
			fileInfos, err := ioutil.ReadDir(chunkDirPath)
			if err != nil {
				w.Write([]byte("Error reading chunk directory"))
				w.WriteHeader(500)
				return
			}

			// create final file to write to
			dst, err := os.Create(r.FormValue("flowFilename"))
			if err != nil {
				w.Write([]byte("Error final file"))
				w.WriteHeader(500)
				return
			}
			defer dst.Close()

			sort.Sort(ByChunk(fileInfos))
			for _, fs := range fileInfos {
				fmt.Println(fs.Name())
				f, err := os.Open(chunkDirPath + "/" + fs.Name())
				if err != nil {
					w.Write([]byte("Error blob file"))
					w.WriteHeader(500)
					return
				}
				defer f.Close()
				io.Copy(dst, f)
			}
			io.Copy(dst, src)
			os.RemoveAll(chunkDirPath)
		} else {
			dst, err := os.Create(chunkDirPath + "/" + r.FormValue("flowChunkNumber"))
			if err != nil {
				w.Write([]byte("Error creating file"))
				w.WriteHeader(500)
				return
			}
			defer dst.Close()
			io.Copy(dst, src)
		}
	}
	w.Write([]byte("success"))
	w.WriteHeader(200)
}
